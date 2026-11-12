# Model từ gateway

## Tự động lấy danh sách

`wj configure`, `wj auth` và `-use-profile` luôn lấy danh sách mới từ gateway. Mỗi gateway
đặt tên một kiểu, nên tool giữ nguyên từng tên trong danh sách, không sửa và không đòi tên
có `claude` hay `gpt`. Tên trong các ví dụ ở đây chỉ để minh họa.

| App | Model Claude | Model khác |
|---|---|---|
| Claude Code | Tự cập nhật mỗi lần mở app | Cập nhật khi chạy các lệnh ở đây |
| Codex | Cập nhật khi chạy các lệnh ở đây | Cập nhật khi chạy các lệnh ở đây |

Gateway vừa thêm model thì chạy:

```text
wj models
```

Lệnh dùng key đã lưu trong từng app, giữ nguyên model mặc định, và không hỏi lại key.

## Claude chạy được là ưu tiên

Khi ghi cấu hình Claude Code qua gateway (`wj configure claude`, `-use-profile`, `wj models`), tool
thử từng model bằng đúng kiểu request Claude Code gửi (`/v1/messages`, stream, có tool):

- Model Claude là phần cốt lõi, luôn được ghi. Model mặc định được thử trước; nếu gateway không
  trả lời thì tool đổi sang Sonnet/Opus/Haiku đầu tiên chạy được và báo rõ.
- Model khác hãng (GPT, DeepSeek, Grok...) chỉ được thêm vào `/model` khi đã thử chạy được. Model
  nào lỗi thì bị bỏ, kèm số lượng bỏ, để không làm Claude Code gặp lỗi khi bạn chọn nhầm.
- Không thử được model Claude nào (mất mạng, sai key): tool vẫn ghi cấu hình bạn đã chọn và in
  cảnh báo `CHƯA kiểm chứng`.
- `-dry-run` không gọi mạng nên không thử gì.

Mỗi lần thử tốn vài token. Bản thử chỉ gồm một câu ngắn có tool, không đảm bảo mọi tính năng.

## Gateway từ chối Claude Code (lỗi 400 `reasoning_effort`)

Có gateway trả `400 reasoning_effort: Extra inputs are not permitted` cho mọi model Claude khi
request mang danh tính Claude Code (`User-Agent: claude-cli/...`, `x-app: cli`), trong khi cùng
request từ client khác thì chạy. `wj models test` sẽ hiện `HTTP 400` ở cột Claude Code.

Việc cần làm là báo cho gateway sửa. Trong lúc chờ, có thể bật đường vòng:

```text
wj -use-profile adnx -neutral-headers          bật, lưu vào profile
wj -use-profile adnx -neutral-headers=false    tắt
```

Cờ này chỉ dùng được với kết nối Gateway và mặc định tắt. Khi bật, tool ghi
`ANTHROPIC_CUSTOM_HEADERS` vào cấu hình Claude: Claude Code gửi `User-Agent: wj-gateway-client/1`
và `x-app: wj` thay cho danh tính của nó, nên gateway không biết đó là Claude Code. Bộ thử model
cũng dùng header này, nên kết quả khớp với thực tế. Đường vòng này không sửa lỗi của gateway và
chỉ mới thử với một câu trả lời ngắn.

## Gateway làm hỏng tool search (cache read tăng vọt)

Có gateway đổi tên mọi tool trước khi gửi lên Anthropic (`Monitor` thành `Monitor_ide`) nhưng
quên đổi tên trong khối `tool_reference`. Khi Claude Code nạp một tool hoãn (ToolSearch),
Anthropic trả `400 Tool reference 'Monitor' not found in available tools`. Claude Code liền tắt
tool search và gửi đủ định nghĩa mọi tool ở mọi lượt sau. Với MCP lớn (kitwright, rea,
ldplayer) mỗi lượt thêm khoảng 280k token đọc cache.

Dấu hiệu trong file phiên: `[Tool references removed - tool search not enabled]`.

Ngày 2026-11-11 gateway ai.adnx.vn đã sửa lỗi này: `tool_reference` tên gốc chạy với mọi tool đã thử (43/43). Bằng chứng, lỗi phụ còn lại và script kiểm tra lại: [gateway-adnx.md](gateway-adnx.md).

Với gateway khác còn lỗi này, cần báo gateway đổi tên cả `tool_reference.tool_name`. Trong lúc chờ, chạy Claude
qua proxy đo token (`wj proxy run claude` hoặc `wj proxy project on` + `wj proxy start`): khi
gateway trả lỗi trên lần đầu, proxy thêm `_ide` vào tên trong `tool_reference` gửi tới gateway
đó, nên lần thử lại của Claude Code chạy được và tool search giữ nguyên. Gateway không trả lỗi
này thì proxy không đụng gì.

Đổi hậu tố bằng `--set tool_ref_suffix=...` khi tự chạy mitmproxy; để rỗng là tắt. Proxy
chỉ sửa đúng tên trong `tool_reference`, không ghi nội dung request vào log.

## Dùng chéo model

Claude Code chỉ gửi `/v1/messages`, Codex chỉ gửi `/v1/responses`. Cả hai app đều chọn
được model khác hãng, nhưng gateway phải tự chuyển đổi đường gửi đó.

Hãng không hỗ trợ chính thức việc này, nên có thể lỗi ở tool, suy luận hoặc nén ngữ cảnh.
Thử thật trước khi dùng:

```text
wj models test                 thử mọi model, tốn vài token, có hỏi trước
wj models test -models a,b     chỉ thử vài model
```

```text
MODEL               /v1/messages (Claude Code)  /v1/responses (Codex)
cc/claude-opus-5-5  OK                          HTTP 404
cx/gpt-6.1-sol      HTTP 429                    OK
```

`OK` ở cột nào thì app đó chọn model này được. Lệnh chỉ thử một câu ngắn có tool, không
đảm bảo mọi tính năng của app.
