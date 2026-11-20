# Áp dụng ngay từ agentdriven.dev

Nguồn so sánh: `.docs/agentdriven-article-analysis.md` (nhánh `wisjoycomm/t-22`) và `.docs/agentdriven-settings-analysis.md` (nhánh `wisjoycomm/t-23`), đối chiếu với `AGENTS.md`, `CLAUDE.md`, `.claude/settings.json` của repo này. Chưa áp dụng gì; đây là danh sách để người quyết định.

Hiện trạng repo (đã kiểm): `.claude/settings.json` chỉ deny `git reset --hard`, `git clean`, `Read(/PLAN-ARCHIVE.md)`, `go.sum`, `vendor`. Không có luật chặn `.env`, `~/.ssh`, `~/.aws`; không có chặn lệnh `gh` ghi ra ngoài. `AGENTS.md` không có mục bảo mật.

## Làm ngay (nhỏ, rủi ro thấp)

| # | Việc | File | Lý do |
|---|---|---|---|
| 1 | Thêm `deny` cho secret: `Read(**/.env)`, `Read(**/.env.*)`, `Read(~/.ssh/**)`, `Read(~/.aws/**)`, `Read(~/.gnupg/**)`, `Read(~/.kube/**)`, `Read(~/.docker/**)`, `Read(~/.netrc)`, `Read(~/.npmrc)` | `.claude/settings.json` | Repo chưa chặn gì. Dạng `Read(...)` chạy được trên Windows. |
| 2 | Thêm `deny` cho `gh` ghi ra ngoài: `gh pr create/comment/review/edit/merge/close`, `gh issue create/comment/edit/close`, `gh release create/edit/delete`, `gh gist create` | `.claude/settings.json` | Worker không nên nói thay người dùng. Kiểm trước: bước release (`.claude/skills/release`) có dùng `gh pr create` không; nếu có thì đưa vào `ask`, không `deny`. |
| 3 | Thêm một mục `## Security` ngắn: không đọc `.env`, `~/.ssh`, `~/.aws`; việc cần secret thì viết script cho người chạy; lệnh từ xa (deploy, SSH, cloud) để người hoặc coordinator, không để worker | `AGENTS.md` | Bài và settings đều nêu; AGENTS.md hiện chỉ cấm `git push`. Hợp với ghi nhớ "agents don't run wj". |
| 4 | Thêm "zero finding trên diff lớn cũng là một finding" vào bước review của orchestrator | `.claude/skills/orca-orchestrator/SKILL.md` | Một dòng, không mâu thuẫn Orca. |

## Cân nhắc (cần người quyết)

- **`deny` cho `Bash(cat .env*)`, `head`, `grep` trên `.env`**: chỉ là gợi ý, dễ vượt qua (analysis T-23, chưa kiểm thử). Chặn thật cần hook; repo đã có hook `wj guard` ở PreToolUse (`Read|Bash|PowerShell`), nên nên đưa luật secret vào `wj guard` thay vì thêm nhiều rule Bash.
- **`CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3`**: chưa kiểm biến này có trong tài liệu chính thức chưa. Không thêm cho đến khi kiểm.
- **Checklist lỗi thường gặp** cho bước review: hợp lý nhưng cần chỗ để lưu; làm sau khi có vài lỗi thật.

## Không áp dụng

- `cleanupPeriodDays: 36500`: giữ transcript vĩnh viễn, có thể chứa secret in ra từ tool.
- Cấm fan-out / không tự mở subagent: mâu thuẫn với Orca.
- Các deny kiểu macOS (`~/Library`, `~/Movies`, `brew`) và stack riêng (`sst`, `application.yml`, `pnpm:*`): không khớp repo Go trên Windows. `pnpm:*` còn cho chạy `pnpm exec`/`dlx`.
- Chọn model theo vai Scout/Thinker: repo đã chọn theo cột Agent trong PLAN.md và `.claude/models.md`.
- Cài bằng `curl -o ~/.claude/CLAUDE.md`: ghi đè file cá nhân, chạy nội dung ngoài.
- Mục Voice, Cost discipline, đoạn triết lý: không kiểm được bằng lệnh hoặc không hợp ngữ cảnh.

## Đã có sẵn (không cần làm)

Trả lời ngắn, không bịa và kiểm bằng lệnh (AGENTS.md "Replies"), dừng sau 3 lần fail, `git push` phải hỏi, deny `git reset --hard` và `git clean`.
