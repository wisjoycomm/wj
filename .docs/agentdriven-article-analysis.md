# Phân tích bài agentdriven.dev

Nguồn: https://agentdriven.dev/ (tải bằng curl, ngày 2026-11-20). Trang là một file `CLAUDE.md` mẫu
kèm `settings.json`, cài bằng `curl -o ~/.claude/CLAUDE.md https://agentdriven.dev/index.md`.
Chưa kiểm tra file `settings.json` của họ.

## Ý chính

Bài là bộ nguyên tắc để làm việc cùng agent như một đồng đội, không phải máy bán hàng tự động.
Mục tiêu là hiểu chung và làm đúng, không phải "xong" nhanh. Thứ tự ưu tiên khi xung đột:
bảo mật, chất lượng code, người đọc hiểu, hiệu quả token/model, tốc độ.

## Các phần và ý đáng chú ý

| Phần | Nội dung | Ghi chú |
|---|---|---|
| Brevity | Trả lời ngắn, gạch đầu dòng, mỗi lần hỏi một câu | Giống mục "Replies" trong AGENTS.md của repo này |
| Cost discipline | Không gợi ý dịch vụ trả phí riêng | Hẹp, chỉ có giá trị với người dùng gói cố định |
| Agent & token discipline | Không tự mở subagent; đề xuất rồi chờ người đồng ý, mỗi lần một quyết định | Ngược với Orca (mở nhiều worker song song) |
| Pipeline 3 vai | Orchestrator (làm chính, trong phiên), Scout (đọc, rẻ), Thinker (lập kế hoạch, review, đắt) | Chọn model theo vai, không theo việc |
| Vòng lặp | Hỏi, nghĩ, người duyệt kế hoạch, build, review, lặp đến khi xanh | Bước duyệt kế hoạch là bắt buộc |
| Command authority | Scout chỉ đọc; lệnh local do Orchestrator; lệnh từ xa (deploy, SSH) chỉ Thinker | Phân quyền theo phạm vi ảnh hưởng |
| Review vs Red team | Review: chạy checklist trong phiên mới. Red team: phản biện thiết kế, quyết go/no-go | Zero finding trên diff lớn cũng là một finding |
| The Hard Way | Cấm hack tạm, cấm báo "xong" khi chưa xong, phải nói rõ khi bị chặn | Giống luật "Stop after 3 failures" |
| Never guess, verify | Ngày, đường dẫn, "đã deploy chưa" phải kiểm bằng lệnh | Giống mục "Never invent facts" |
| Security | Không đọc `.env`, `~/.aws`, `~/.ssh`; việc cần secret thì viết script cho người chạy | Rõ và đáng lấy |
| Voice | Văn bản công khai (PR, commit) mang giọng người, có dòng công khai dùng AI | Cấm em dash, liệt kê dài |
| Many Minds | Ba vai Architect, Scientist, Engineer, gọi theo tên | Hợp tác bằng hợp đồng giao nhận |
| CLAUDE.md dự án | Mẫu: tóm tắt, Start Here, Structure, Commands, Key Principles | Gần với cách repo này đã làm |

## So với repo này (AGENTS.md, CLAUDE.md, Orca)

Giống nhau:
- Trả lời ngắn, không bịa, kiểm chứng bằng lệnh.
- Dừng và báo lỗi khi bị chặn (họ: "say I'm blocked"; ta: dừng sau 3 lần fail).
- Giữ hành vi trong file hướng dẫn gốc của dự án.

Khác nhau:
- **Song song.** Họ cho tối đa một agent nền và cấm fan-out nếu chưa được duyệt từng lần.
  Orca chủ động chạy nhiều worker, mỗi worker một worktree. Hai cách nghĩ trái nhau.
- **Chọn model.** Họ chọn theo vai (Scout/Orchestrator/Thinker). Repo này chọn theo cột Agent
  trong PLAN.md và `models.md` của orca-orchestrator.
- **Người duyệt.** Họ bắt người duyệt kế hoạch trước khi build. Ở đây người duyệt khi gõ `go`.
- **Độ dài.** Họ viết nhiều văn xuôi và triết lý ("Godspeed"). AGENTS.md của ta là luật ngắn, kiểm được.

## Có thể học

Chưa áp dụng gì; đây là gợi ý để người quyết định.
1. **Phân quyền lệnh theo phạm vi ảnh hưởng.** Worker chỉ làm local; lệnh từ xa (deploy, push) để người hoặc coordinator.
   AGENTS.md hiện chỉ cấm `git push`, chưa nói về SSH hay cloud.
2. **Luật bảo mật rõ ràng.** Thêm một dòng cấm đọc `.env`, `~/.aws`, `~/.ssh`. AGENTS.md chưa có.
3. **"Zero finding là một finding."** Dùng cho bước review của orchestrator.
4. **Checklist lỗi thường gặp.** Gặp loại lỗi mới thì thêm vào, để cổng kiểm tra không phụ thuộc sự cẩn thận.

## Không nên lấy

- Cấm fan-out: mâu thuẫn với cách Orca làm việc.
- Đoạn văn triết lý ("Our Understanding"): tốn token, không kiểm được bằng lệnh.
- Mục Cost discipline: chỉ hợp với bối cảnh gói trả cố định.
- Cài thẳng bằng `curl -o ~/.claude/CLAUDE.md`: ghi đè file cá nhân và chạy nội dung từ ngoài. Nên đọc và chép tay phần cần.

## Giới hạn của phân tích

- Chỉ đọc trang chính, không đọc `settings.json` và `index.md` riêng.
- Không rõ tác giả hay mức độ kiểm chứng của các quy tắc; bài không đưa số liệu hiệu quả.
