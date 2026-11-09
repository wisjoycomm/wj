# Gateway ai.adnx.vn: lỗi `tool_reference` (điều tra 2026-11-09)

Tóm tắt phần [Gateway làm hỏng tool search](model.md#gateway-làm-hỏng-tool-search-cache-read-tăng-vọt)
trong model.md, ở đây là bản đầy đủ: bằng chứng, các lỗi phụ, và cách kiểm tra lại.

## Tình trạng

- **2026-11-09: CHƯA SỬA.** `tool_reference` dùng tên gốc vẫn bị `400 Tool reference 'X' not found in available tools`.
- **Cách vá của proxy wj vẫn chạy:** thêm `_ide` vào tên trong `tool_reference`.
- **Đã gửi bên gateway:** báo cáo, log và script trong `~/Downloads/gateway-tool-reference-report-2026-11-09.zip`.

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
- **Gateway đã sửa:** T2, T4, T5 trả 200 và model gọi đúng tên gốc. Khi đó có thể bỏ bản vá `tool_ref_suffix` trong `proxy_addon.py`.
- **Bản vá proxy hỏng:** T3 trả 400, nghĩa là gateway đổi quy tắc đặt tên. Sửa `tool_ref_suffix` trong `proxy_addon.py`.

Phải gọi **thẳng** gateway. Proxy wj sửa request nên sẽ che mất lỗi; các script đã tự bỏ `HTTPS_PROXY`.
Header `User-Agent: wj-gateway-client/1` và `x-app: wj` dùng để tránh một lỗi khác của gateway
([lỗi `reasoning_effort`](model.md#gateway-từ-chối-claude-code-lỗi-400-reasoning_effort)).
