# Kết nối

Key nhập riêng trên từng máy. Tool không in key ra màn hình.

## Claude Code

```text
wj configure claude                                  hỏi từng bước
wj auth -auth gateway -only claude -base-url ai.adnx.vn -profile adnx
wj auth -auth oauth -profile work                    token sk-ant-oat...
wj auth -auth api-key -profile api                   key từ console.anthropic.com
wj auth -auth login                                  đăng nhập trên web
```

Đổi kết nối đã lưu:

```text
wj -list-profiles
wj -use-profile adnx -run
```

Sau khi đổi, mở phiên Claude mới và gõ `/status` để kiểm tra.

### Toàn máy hay chỉ project

Chạy `wj configure claude` trong một project git thì tool hỏi thêm áp dụng cho đâu:

| Chọn | Ghi vào | Tác dụng |
|---|---|---|
| Toàn máy (mặc định) | `~/.claude/settings.json` | mọi project |
| Chỉ project này | `<project>/.claude/settings.local.json` | chỉ phiên Claude mở trong project đó |

Lệnh có sẵn tham số: thêm `-scope project` (và `-dir THƯ_MỤC` nếu không đứng trong project), vd
`wj -use-profile work -scope project`.

- File của project là file cá nhân: tool tự thêm nó vào `.gitignore`, nên key không vào commit.
- Claude Code gộp `env` của hai file. Để kết nối toàn máy không lọt vào project, tool ghi các
  biến kết nối toàn máy mà project không dùng thành chuỗi rỗng trong file project.
- Bỏ kết nối riêng của project: xoá các biến `ANTHROPIC_*` / `CLAUDE_CODE_OAUTH_TOKEN` trong
  `.claude/settings.local.json` (giữ biến của `wj proxy project` nếu đang đo token).

## Codex và OpenCode

```text
wj configure codex                                   hỏi từng bước
wj configure codex -auth gateway -base-url ai.adnx.vn
wj configure codex -from-profile adnx                dùng lại gateway đã lưu cho Claude
wj configure opencode
```

Gateway cho Codex phải hỗ trợ `/v1/responses`.

OpenCode cũng chọn được chỉ project: `wj configure opencode` trong project git sẽ hỏi, hoặc thêm
`-scope project`. Tool ghi `opencode.json` ở gốc project và thêm nó vào `.gitignore`; nếu file đó
đã nằm trong git thì tool từ chối để key không bị commit. Codex chỉ có toàn máy, vì Codex bỏ qua
provider trong config của project.

Cursor, Antigravity và Orca tự đăng nhập trong app; `wj configure` chỉ in hướng dẫn.

File cấu hình cũ được sao lưu thành `.bak-<thời gian>` trước khi ghi đè.

## Gọi thẳng gateway, bỏ proxy

Proxy đo token lỗi hoặc đang tắt thì Claude trong project đã bật đo sẽ không kết nối được.
Bỏ proxy, giữ gateway:

```text
wj direct -dry-run     xem sẽ đổi gì
wj direct
```

Lệnh tắt đo token ở mọi project (`wj proxy project off -all`). Gateway và key trong
`~/.claude/settings.json` giữ nguyên, nên Claude vẫn gọi thẳng lên gateway của bạn, không qua Anthropic.

Lệnh không tắt mitmproxy đang chạy và không đụng Codex. Mở phiên Claude mới sau khi chạy.
Bật đo lại bằng `wj proxy project on`.
