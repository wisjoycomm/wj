# Token tăng vọt và cách giảm context

Khi số token trên `wj top` hoặc overlay tăng bất thường, xem phần này để tìm nguyên nhân và cách giảm mà không mất
thông tin quan trọng.

## Vì sao tốn

Mỗi lượt gọi, model đọc lại **toàn bộ** context từ cache. Vì vậy:

> token của một phiên ≈ số lượt gọi × context trung bình

Một agent chạy 185 lượt, context tăng từ 26k lên 300k, sẽ đọc khoảng 185 × 180k ≈ 34M token. Chạy 5 agent như vậy song song thì một giờ có thể lên ~100M.

Gần như tất cả là **đọc cache** (98% trong ví dụ dưới). Trên API trả tiền, đọc cache có giá bằng 10% giá input.
Chưa rõ nó tính vào hạn mức gói subscription theo tỷ lệ nào.

## Trước tiên: loại trừ lỗi gateway

Nếu file phiên có một trong hai dòng sau thì không phải do cách làm việc, mà do gateway làm hỏng tool search
(+~280k mỗi lượt). Xem [gateway-adnx.md](gateway-adnx.md).
- `Tool references removed - tool search not enabled`
- `ToolSearch is no longer available`

```bash
cd ~/.claude/projects/<thư-mục-project>
rg -l 'Tool references removed - tool search not enabled|ToolSearch is no longer available' $(fd -e jsonl --changed-within 1d .)
```

Dấu hiệu khác: context nhảy vọt vài trăm k chỉ trong một lượt. Context tăng dần đều là do làm việc thật.

## Lệnh phân tích

**Cách nhanh nhất: `wj usage today` (hoặc `week`, `lastweek`, `month`...).** Báo cáo liệt kê từng session của kỳ,
session tốn nhiều nhất lên đầu, kèm phần do worker, context lớn nhất và model. Xem
[token.md](token.md#theo-ngày-tuần-tháng). Các lệnh `jq`/`awk` dưới đây chỉ cần khi muốn xem sâu hơn, ví dụ tách
theo giờ hoặc theo từng subagent.

Thư mục project trong `~/.claude/projects/` có tên là đường dẫn project, thay `:\` và `\` bằng `-`
(ví dụ `D--UnityProjects-endlesswinter`). Subagent nằm trong `<session>/subagents/agent-*.jsonl`.
Một lượt gọi có thể bị ghi nhiều dòng, nên phải lọc trùng theo `message.id`.

```bash
cd ~/.claude/projects/D--UnityProjects-endlesswinter
# Gom usage của mọi file phiên + subagent sửa trong 1 ngày: id, file, giờ, token
for f in $(fd -t f -e jsonl --changed-within 1d .); do
  jq -r --arg f "$f" 'select(.type=="assistant" and .message.usage) | [.message.id, $f, .timestamp,
    (.message.usage | (.input_tokens//0)+(.cache_read_input_tokens//0)+(.cache_creation_input_tokens//0)+(.output_tokens//0))] | @tsv' "$f"
done > .tmp/u.tsv

# Token theo phiên / agent
awk -F'\t' '!s[$1]++ {t[$2]+=$4; n[$2]++} END {for (k in t) printf "%7.1fM %4d calls  %s\n", t[k]/1e6, n[k], k}' .tmp/u.tsv | sort -rn | head

# Token theo giờ (giờ VN = UTC+7)
awk -F'\t' '!s[$1]++ {h=(substr($3,12,2)+7)%24; t[h]+=$4} END {for (h in t) printf "%02d:00 %6.1fM\n", h, t[h]/1e6}' .tmp/u.tsv | sort

# Context của một phiên lớn lên thế nào (cứ 50 lượt in một lần)
jq -r 'select(.type=="assistant" and .message.usage) | [.message.id, (.message.usage | (.input_tokens//0)+(.cache_read_input_tokens//0)+(.cache_creation_input_tokens//0))] | @tsv' <file>.jsonl |
  awk -F'\t' '!s[$1]++ {n++; if (n==1||n%50==0) printf "call %d: %dk\n", n, $2/1000}'
```

Log của proxy wj (`~/.wj/proxy/usage.jsonl`, có trường `project`, `host`, `status`) cho biết request đi thẳng
`api.anthropic.com` hay qua gateway.

## Ví dụ: endlesswinter, 2026-11-09

**Cả ngày:** 1,785 lượt, **253.9M token**, trong đó 248.1M là đọc cache. Không có fallback tool search, và traffic
đi thẳng `api.anthropic.com`, nên không phải lỗi gateway.

| Giờ VN | Token |
|---|---|
| 08:00 | 17.8M |
| **09:00** | **93.8M** |
| 10:00 | 24.1M |
| 11:00 | 20.8M |
| 12:00 | 35.3M |
| 13:00 | 54.7M |

**Vụ nhảy vọt:** chat mở lúc 08:51 ("tiếp tục các task trong plan") chạy song song 5 worker, mỗi worker làm một
dòng PLAN. Trong 70 phút tốn 111.6M:

| Worker | Lượt | Token | Context cuối |
|---|---|---|---|
| T5.10 event shop | 185 | 34.0M | 302k |
| T4.3/T4.5 | 173 | 33.1M | 322k |
| (3 worker khác) | 70–179 | 5.9–15.9M | |

**Context tăng đều, không nhảy vọt:** 26k → 49k (lượt 20) → 133k (lượt 50) → 207k (lượt 100) → 302k. Mỗi worker
có khoảng 180 lệnh Bash, đổ khoảng 100k token output (build, tìm code Unity và backend, file config).

**Chat chính `6a9b5fe6`** (từ 10:48): 292 lượt, context lên 401k, tốn 64.3M.

## Cách giảm mà không mất thông tin quan trọng

Nguyên tắc: **cái quan trọng phải nằm trong file, không chỉ nằm trong chat.** Khi đó `/clear` hay `/compact` không làm mất gì.

| Thông tin | Để ở đâu |
|---|---|
| Trạng thái task, việc tiếp theo | `PLAN.md` |
| Code đã viết | git commit |
| Rule của project | `AGENTS.md` / `CLAUDE.md` (tự nạp mỗi phiên) |
| Việc đang dở (file đang sửa, test đang lỗi, bước tiếp theo) | `.tmp/handover.md`, phải ghi ra trước khi compact hoặc clear |

Các cách, xếp theo hiệu quả:
1. **Task nhỏ:** mỗi worker khoảng 50–80 lượt. Tách dòng PLAN lớn (backend + client) thành hai.
2. **Ít worker song song:** 2–3 thay vì 5. Tổng token cho cùng khối việc gần như không đổi, nhưng tránh được cú nhảy và dừng sớm được nếu hỏng.
3. **Lưu trạng thái trước khi compact hoặc clear:**
   - Bảo model "cập nhật PLAN.md và ghi trạng thái vào `.tmp/handover.md`".
   - Rồi `/compact keep: task ID, files changed, failing test, next step`. Phần ghi sau `/compact` được giữ trong bản tóm tắt.
   - Hoặc `/clear`, rồi cho chat mới đọc `PLAN.md` và `.tmp/handover.md`.
4. **Compact sớm hơn**, kèm hook giữ chất lượng (xem hai mục dưới).
5. **Giới hạn output Bash:** `BASH_MAX_OUTPUT_LENGTH`, kèm rule dùng cờ quiet và `| tail`.
6. **Chat chính dài:** qua khoảng 150–200k thì `/compact` hoặc mở chat mới.

## Cách nào ảnh hưởng chất lượng

| Cách | Rủi ro chất lượng | Lý do |
|---|---|---|
| Giới hạn output Bash | Gần như không | Output dài thành file + 2.000 ký tự đầu. Model tự đọc hoặc tìm trong file khi cần, không mất gì |
| Task nhỏ, ít worker song song | Không, có thể tốt hơn | Cùng khối việc, context mỗi worker ngắn hơn |
| Compact sớm | **Có** | Bản tóm tắt có thể làm rơi chi tiết nhỏ nhưng quan trọng. Cần hook và ghi chú trên đĩa |

Nhưng context quá dài cũng làm giảm chất lượng. Anthropic
([Effective context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents))
ghi nhận khả năng nhớ chính xác thông tin trong context giảm dần khi số token tăng, ở mọi model. Bài này khuyên
compact vừa phải, kèm ghi chú có cấu trúc lưu ngoài context.

Sau khi compact, Claude Code nạp lại CLAUDE.md, AGENTS.md và memory từ đĩa, và đọc lại tối đa 5 file sửa gần nhất
(file trên 5.000 token chỉ còn đường dẫn). Những thứ khác chỉ còn lại phần bản tóm tắt giữ.

## Thông số đã kiểm chứng (Claude Code 2.1.319, 2026-11-09)

Đã thử thật, không chỉ đọc tài liệu.

**`BASH_MAX_OUTPUT_LENGTH` (mặc định 30.000 ký tự):**
- Đặt 15000, chạy lệnh in ra 20.000 ký tự: model nhận đường dẫn file + 2.000 ký tự đầu, thay vì cả 20.000 ký tự inline.
- Đặt trong shell hay trong khối `env` của settings đều có tác dụng.
- Setting `bashOutputMaxChars: 15000` cho kết quả giống hệt.

**Cửa sổ compact:**
- Model cửa sổ 1M: debug log ghi `effectiveWindow=980000`, mặc định tự compact ở khoảng 967k.
- Công thức trong mã Claude Code: `ngưỡng = min(effectiveWindow × PCT/100, effectiveWindow − 13000)`.

| Thông số | Compact ở |
|---|---|
| Mặc định (model 1M) | ~967k |
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=40` / `30` / `20` | ~392k / ~294k / ~196k |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000` | ~267k (`/context` hiện `/ 300k`) |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000` | ~167k (`effectiveWindow=180000`) |

- **Nên dùng `CLAUDE_CODE_AUTO_COMPACT_WINDOW` thay cho PCT:**
  - Nó không phụ thuộc model: được chặn ở cửa sổ của model.
  - PCT thì tính trên cửa sổ, nên PCT=20 là ~196k ở model 1M nhưng chỉ ~36k ở model 200k.
  - Đặt trong shell, trong `env` của settings, hay setting `autoCompactWindow` đều có tác dụng.
  - Subagent cũng dùng cửa sổ này (theo tài liệu).
- **Không đặt cả hai:** PCT sẽ nhân thêm (cửa sổ 200k với PCT=30 ra ~54k).

**Hook giữ chất lượng khi compact:**
- **`PreCompact`:** stdout của hook được dùng làm hướng dẫn cho bản tóm tắt.
- **`SessionStart` matcher `compact`:** stdout được đưa vào context ngay sau compact.
- **Đã thử:** cả hai hook chạy khi compact chat chính (`/compact`), và khi **subagent tự compact**.
  - Lần thử subagent: compact 4 lần, hai hook chạy cả 4 lần, chuỗi đánh dấu trong hook có mặt trong file phiên của subagent.
  - Subagent vẫn làm xong và trả đúng kết quả.

## Đã áp dụng cho endlesswinter (2026-11-09, phương án B: thận trọng)

`D:\UnityProjects\endlesswinter\.claude\settings.json`:

```json
"env": {
  "BASH_MAX_OUTPUT_LENGTH": "15000",
  "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "300000"
},
"hooks": {
  "PreCompact": [ { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/precompact.sh\"" } ] } ],
  "SessionStart": [ { "matcher": "compact", "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR/.claude/hooks/after-compact.sh\"" } ] } ]
}
```

- **`.claude/hooks/precompact.sh`:**
  - Bảo bản tóm tắt giữ task ID và spec, file đã sửa, lỗi kiểm tra kèm dòng lỗi chính xác, quyết định và hướng đã loại, bước tiếp theo.
  - Chèn nguyên văn `.tmp/handover.md` nếu có. Tìm trong thư mục đang làm (worktree của worker) trước, rồi đến project.
- **`.claude/hooks/after-compact.sh`:** sau compact, nhắc đọc lại dòng task trong PLAN.md và `git status`, rồi chèn `.tmp/handover.md` nếu có.

Kết quả: compact ở ~267k, nên chỉ worker dài nhất mới compact, khoảng một lần gần cuối.

Ba phương án đã cân nhắc:

| Phương án | Cửa sổ | Compact ở | Tiết kiệm | Rủi ro chất lượng |
|---|---|---|---|---|
| A | 200000 + hook | ~167k | Nhiều hơn | Thấp |
| **B (đang dùng)** | 300000 + hook | ~267k | Ít | Rất thấp |
| C | PCT=30 | ~294k | Gần như không | Rất thấp |

Kế hoạch: dùng B vài ngày. Nếu chất lượng không giảm thì chuyển A, chỉ cần đổi `300000` thành `200000`.

Cách đo chất lượng, so với số liệu 2026-11-09:
- `fast_verify` có pass ngay lần đầu không.
- Số commit `fix(...)` phải làm thêm sau mỗi task.
- Số vấn đề review tìm ra.
- Token mỗi task.

`AGENTS.md` có thêm mục **Keep output small**:
- **Build và test:** chỉ in lỗi và phần cuối (`dotnet build -v q -clp:ErrorsOnly`, `dotnet test --nologo -v q`, `| tail -n 40`).
- **Unity batch mode:** ghi log ra `.tmp/unity.log` rồi `rg` lỗi.
- **Tìm kiếm:** dùng `rg -l` / `rg -c` trước khi đọc.
- **JSON lớn:** truy vấn bằng `jq` / `rg`, không đổ ra cả file.
- **Task dài:** ghi `PLAN.md` và `.tmp/handover.md` trước khi hết context.

`wj init` không làm mất các thay đổi này:
- AGENTS.md của endlesswinter không có dấu `wj:begin` nên không bị đụng tới.
- Với settings.json, `wj init` chỉ thêm quyền từ kit vào file có sẵn.

Chưa đưa vào kit của `wj init`. Muốn mọi project mới đều có thì thêm vào `kit/base/` (`AGENTS.md`,
`.claude/settings.json`) sau khi thấy hiệu quả ở endlesswinter.

Lưu ý: `.claude/settings.json` của endlesswinter được git theo dõi. Worker chạy thành phiên Claude riêng trong
worktree chỉ có cấu hình này sau khi nó được commit vào nhánh mà worktree tạo ra từ đó. Subagent trong cùng phiên
thì dùng luôn cấu hình của project chính.
