# Gateway ai.adnx.vn: lỗi `tool_reference` (điều tra 2026-11-09, đã sửa 2026-11-11)

Các phần "Lỗi chính" và "Bằng chứng" bên dưới mô tả lỗi **trước khi sửa** (2026-11-09). Kết quả kiểm tra lại ngày 2026-11-11 nằm ở mục "Kết quả kiểm tra lại".

Tóm tắt phần [Gateway làm hỏng tool search](model.md#gateway-làm-hỏng-tool-search-cache-read-tăng-vọt)
trong model.md, ở đây là bản đầy đủ: bằng chứng, các lỗi phụ, và cách kiểm tra lại.

## Tình trạng

- **2026-11-11: ĐÃ SỬA lỗi chính.** `tool_reference` dùng tên gốc giờ trả 200 và model gọi đúng tên đó. Đã thử 43/43 tên hợp lệ (tool Claude Code, tool MCP dài, tên đã có `_ide`, tên tự đặt), có `stream: true`.
- **Còn lại:** request không có trường `stream` vẫn lộ tên `X_ide`; gateway vẫn chèn 20 tool cố định (xem "Lỗi phụ").
- **Bản vá của proxy wj không còn cần,** nhưng vô hại: chỉ chạy sau khi gateway trả `400 Tool reference ... not found`, nên gateway đã sửa thì không bao giờ kích hoạt. Gateway nhận cả tên gốc lẫn tên có `_ide`.
- **Lịch sử (2026-11-09):** từng bị `400 Tool reference 'X' not found in available tools`. Báo cáo, log và script gửi bên gateway: `~/Downloads/gateway-tool-reference-report-2026-11-09.zip`.

## Lỗi chính

Claude Code nạp tool hoãn (`defer_loading: true`) bằng ToolSearch. ToolSearch trả `{"type":"tool_reference","tool_name":"Monitor"}`
trong `tool_result`, rồi Anthropic tìm tên đó trong `tools` của request. Tài liệu Anthropic yêu cầu tên phải khớp
([tool-search-tool](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool), mục
"Custom tool search implementation" và "Error handling").

Gateway đổi tên mọi tool ở `tools[].name` thành `X_ide` nhưng không đổi `tool_reference.tool_name`:

```
Claude Code gửi:                  Gateway chuyển lên Anthropic:
tools: [ {name:"Monitor"} ]  →    tools: [ {name:"Monitor_ide"} ]   ← có đổi
tool_reference: "Monitor"    →    tool_reference: "Monitor"         ← không đổi → 400
```

Lúc trả về, gateway đổi `Monitor_ide` lại thành `Monitor`, nên client không thấy đuôi `_ide`.

**Hậu quả:** Claude Code tắt tool search và gửi đủ định nghĩa mọi tool ở mỗi lượt. Đo trên phiên thật 2026-11-06:
**109,236 → 396,378 token mỗi lượt (+287k)**, giữ nguyên đến hết phiên.

Dấu hiệu trong file phiên:
- `[Tool references removed - tool search not enabled]`
- `ToolSearch is no longer available. Tools listed earlier as deferred ...`

## Bằng chứng (gọi thẳng gateway, đã tắt proxy)

| Test | Request | Kết quả | Chứng minh |
|---|---|---|---|
| T1 | Không có `tool_reference` | 200 | Key, kết nối ổn |
| T2 | ref = `Monitor` (tên gốc) | **400** not found | Lỗi chính |
| T3 | ref = `Monitor_ide` | **200** | Phía Anthropic có tool `Monitor_ide`, tức gateway đã đổi tên |
| T4 | ref = tên MCP gốc | 400 | Lỗi với mọi tool |
| T5 | Như T2, `stream: true` | 400 | Streaming cũng lỗi |
| T6 | ref = `Nope` (không tồn tại) | 400, cùng thông báo | Là lỗi "không có tool" chuẩn của Anthropic |
| T7 | Như T3, không có trường `stream` | 200 nhưng trả `Monitor_ide` | Lỗi phụ, xem dưới |
| T8 | Hỏi model thấy tool nào (gửi `get_weather`) | 200 | Model thấy `get_weather_ide` và 20 tool gateway chèn thêm |

Thêm:
- **Thêm `_ide` hai lần thì sai:** `Monitor_ide_ide` → 400. Chỉ đúng một hậu tố `_ide` mới chạy.
- **Request không có tools:** gateway không chèn gì, model trả lời "NONE".

## Lỗi phụ của gateway

- **Request không có trường `stream`:**
  - Response khai `Content-Type: text/event-stream` nhưng body là một khối JSON kèm `data: [DONE]`.
  - Tên tool không được đổi lại, nên `get_weather_ide` lộ ra client.
  - Có `"stream": true` hoặc `"stream": false` thì đúng. Đã thử mỗi kiểu 3 lần, kết quả như nhau.
- **Gateway chèn tool:** khi client gửi tools, gateway chèn một bộ 20 tool cố định:
  `Task, TaskOutput, TaskStop, TaskCreate, TaskGet, TaskUpdate, TaskList, Bash, Glob, Grep, Read, Edit, Write, NotebookEdit, WebFetch, WebSearch, AskUserQuestion, Skill, EnterPlanMode, ExitPlanMode`.

## Vì sao có `_ide` (suy luận, chưa xác nhận với bên gateway)

- **Tránh trùng tên:** bộ tool gateway chèn thêm trùng tên với tool của Claude Code (`Bash`, `Read`…), và Anthropic từ chối tên trùng. Vì vậy gateway thêm `_ide` vào **mọi** tool của client, kể cả tool không trùng.
- **Bộ tool cũ:** bộ chèn thêm giống Claude Code bản cũ (có `Task`, bản mới dùng `Agent`; không có `ToolSearch`).
- **Có vẻ chạy qua tài khoản subscription:** response 200 có header `anthropic-ratelimit-unified-5h-*` và `anthropic-ratelimit-unified-7d-*`, là kiểu hạn mức của gói subscription Claude. Kèm tiền tố model `cc/`, có vẻ gateway chạy qua tài khoản Claude Code và giả dạng Claude Code.
- **Không phải do Claude Code:** Claude Code 2.1.319 không có quy tắc đổi tên `X_ide`. Trong binary chỉ có `mcp__ide__getDiagnostics` và `mcp__ide__executeCode`, là tiền tố của tích hợp IDE.

## So với dùng gói Claude trực tiếp

| | Trực tiếp | Qua ai.adnx.vn |
|---|---|---|
| Tên tool lên Anthropic | Giữ nguyên | Thêm `_ide` |
| Tool chèn thêm | Không | 20 tool cố định |
| `tool_reference` tên gốc | Chạy | 400 |
| Tắt tool search, +~287k token mỗi lượt | Không | Có (nếu không qua proxy wj) |

## Số liệu từ proxy wj

- **Lần đầu phát hiện:** 2026-11-07 04:32 UTC. Proxy gặp 400, bật bản vá, gửi lại thì được 200.
- **Số lần vá:** từ đó đến 2026-11-09, proxy đã thêm `_ide` cho **467 request**.
- **Phiên đầu tiên có dấu hiệu fallback:** 2026-11-05 (endlesswinter).

## Kết quả kiểm tra lại (2026-11-11)

Gọi thẳng gateway, model `cc/claude-haiku-4-5-20251001`.

| Test | Trước (11-09) | Sau (11-11) |
|---|---|---|
| T2 ref = `Monitor` | 400 | **200**, model gọi `Monitor` |
| T3 ref = `Monitor_ide` | 200 | 200, model gọi `Monitor` |
| T4 ref = tên MCP gốc | 400 | **200** |
| T5 `stream: true` | 400 | **200** |
| T6 ref = `Nope` | 400 | 400 (đúng) |
| T7 không có `stream` | 200, lộ `Monitor_ide` | 200, **vẫn lộ `Monitor_ide`** |
| T8 liệt kê tool | `get_weather_ide` + 20 tool chèn | **không đổi** |

Thử thêm 45 tên, mỗi tên một request có `tool_reference` tên gốc và `defer_loading`: 43 tên đạt (gồm `Agent`, `Artifact`, `Workflow`, `Task*`, `Cron*`, `mcp__claude_ai_Figma__use_figma`, `mcp__claude_ai_Context7__query-docs`, `mcp__claude-in-chrome__computer`, `Foo_ide`, `my_tool`). Hai ca còn lại không phải lỗi gateway: `ToolSearch` bị 400 do script khai báo trùng tên, `mcp__claude-in-chrome__navigate` được 200 nhưng model chọn gọi `ToolSearch`.

Chưa kiểm tra bằng một phiên Claude Code thật qua gateway: kiểm tra xem dòng `[Tool references removed - tool search not enabled]` còn xuất hiện trong file phiên không.

## Kiểm tra lại

Script trong [gateway-adnx/](gateway-adnx/). Script đọc key từ `GATEWAY_KEY`, hoặc từ `~/.wj/profiles/adnx.json` nếu không có, và không in key ra. Mỗi lần chạy tốn vài nghìn token Haiku.

```powershell
powershell -ExecutionPolicy Bypass -File docs\gateway-adnx\repro.ps1
```
```bash
bash docs/gateway-adnx/repro.sh                 # bảng kết quả T1–T8
bash docs/gateway-adnx/capture-logs.sh .tmp/gw  # log HTTP đầy đủ từng test (đã che key)
```

Đọc kết quả:
- **Gateway đã sửa (tình trạng từ 2026-11-11):** T2, T4, T5 trả 200 và model gọi đúng tên gốc. Bản vá `tool_ref_suffix` trong `proxy_addon.py` không còn cần, và vô hại nên chưa gỡ.
- **Bản vá proxy hỏng:** T3 trả 400, nghĩa là gateway đổi quy tắc đặt tên. Sửa `tool_ref_suffix` trong `proxy_addon.py`.

Phải gọi **thẳng** gateway. Proxy wj sửa request nên sẽ che mất lỗi; các script đã tự bỏ `HTTPS_PROXY`.
Header `User-Agent: wj-gateway-client/1` và `x-app: wj` dùng để tránh một lỗi khác của gateway
([lỗi `reasoning_effort`](model.md#gateway-từ-chối-claude-code-lỗi-400-reasoning_effort)).
