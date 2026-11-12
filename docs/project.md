# Project

## Setup ban đầu

```text
cd <project>
wj init                    tự nhận job, hoặc hỏi
wj init -type unity,web    chọn job
wj init -dry-run           xem trước
wj init -update            project cài bằng kit cũ: lấy skill Orca mới
wj init -lite              chỉ phần không cần wj (xem "Hai mức" bên dưới)
```

Job có sẵn: `unity`, `web`, `go`, `python`.

### Hai mức: đủ và lite

Kit có hai lớp, cùng một nguồn (`kit/base` và `kit/orca`):

| Mức | Có gì | Cần `wj` trên máy? |
|---|---|---|
| Lite (`wj init -lite`) | `AGENTS.md`, `CLAUDE.md`, rule theo job, `.gitignore`, và `.claude/settings.json` với file cấm đọc, giới hạn output Bash, quyền git | Không |
| Đủ (`wj init`) | Lite + hook `wj guard`, skill `/orca-orchestrator` và `/orca-dream`, subagent `orca-worker`, `PLAN.md` | Có |

- Lite vẫn tiết kiệm token nhờ thứ Claude Code tự cưỡng chế (rule deny, giới hạn output). Nó không có: chặn đọc cả
  file dài, chặn đường vòng qua shell (`cat .env`), và chạy task song song.
- Người không cài `wj`: mỗi bản release có `wj-kit-lite-<job>.zip` (`general`, `unity`, `web`, `go`, `python`), do
  chính `wj init -lite` sinh ra. Giải nén vào gốc project **mới**. Project đã có `AGENTS.md`, `.gitignore` hay
  `.claude/settings.json` thì đừng giải nén đè: gộp tay, vì zip không biết gộp như `wj init`.
- Nâng từ lite lên đủ: cài `wj` rồi chạy `wj init`. Tool chỉ thêm phần còn thiếu.
- Repo cài mức đủ, nhưng một người trong nhóm không có `wj`: hook tự bỏ qua (không báo lỗi, không chặn gì), rule
  và file cấm đọc vẫn có tác dụng. Skill Orca thì không chạy được, và model được dặn báo "thiếu wj" rồi dừng.

Kit nhắm vào Claude Code. Tool thêm rule chung, rule theo job, `PLAN.md`, `PLAN-ARCHIVE.md` (bảng trống),
`.claude/settings.json`, `.gitignore`, hai skill (`/orca-orchestrator`, `/orca-dream`) và subagent `orca-worker`.
Chạy lại nhiều lần được: file đã có không bị ghi đè, còn rule trong `AGENTS.md` nằm giữa hai dấu
`<!-- wj:begin -->` nên phần tự viết được giữ nguyên.

`.claude/settings.json` có năm phần, và tool chỉ thêm, không xoá:

- `allow`: việc chạy không cần hỏi (`wj plan`, `wj verify`, `wj digest`, git chỉ đọc, `git add`, `git commit`,
  subagent `orca-worker`, lệnh test của job). `git push` vẫn hỏi.
- `deny`: file Claude Code không được đọc. Luật "đừng đọc" trong `AGENTS.md` chỉ là lời nhắc; ở đây harness
  chặn hẳn: `PLAN-ARCHIVE.md`, lockfile, `node_modules/`, `.venv/`, `.env`, và `Library/` `Temp/` `Logs/` của Unity
  (tuỳ job), `go.sum` và `vendor/` của Go. Rule viết dạng `Read(**/node_modules/**)` nên đúng cả với thư mục
  nằm sâu trong monorepo. Muốn mở lại một mục thì xoá dòng đó khỏi `deny`.
- `hooks`: `PreToolUse` gọi `wj guard` trước mỗi lượt Read, Bash, PowerShell (xem mục dưới). Project đã có hook
  riêng thì tool thêm hook này bên cạnh, không đụng hook cũ.
- `env`: `CLAUDE_CODE_GLOB_NO_IGNORE=false` để Glob bỏ qua file gitignore như Grep;
  `BASH_MAX_OUTPUT_LENGTH=15000` để output lệnh dài thành file + 2.000 ký tự đầu (xem [context.md](context.md)).
  Biến bạn đã đặt thì giữ nguyên giá trị.
- `worktree.baseRef: "head"`: worktree của worker bắt đầu từ commit hiện tại của bạn, không phải từ nhánh mặc định
  trên remote. Thiếu dòng này worker không thấy `PLAN.md` vừa commit.

Skill, worker và hook gọi `wj plan`, `wj verify`, `wj digest`, `wj guard`, nên máy cần có `wj` (không cần `pwsh`). Project cài bằng
kit cũ: `wj init -update` thay skill bằng bản mới (bản cũ sao lưu `.bak-<giờ>`), xoá script `.ps1` và skill
`orca-stateless-worker` cũ. `CLAUDE.md` đã có thì tool không sửa; muốn lấy bản mới thì xoá file rồi chạy lại `wj init`.

Xong thì commit các file này, vì worktree của worker chỉ thấy file đã commit, rồi mở lại Claude Code để nó nhận
subagent và settings mới.

## Chặn lượt đọc tốn token: wj guard

Luật "đọc ít" trong `AGENTS.md` chỉ là lời nhắc, model có thể bỏ qua. `wj guard` là hook `PreToolUse`: Claude Code
gọi nó trước mỗi lượt Read, Bash, PowerShell; nếu lượt đó tốn token vô ích thì nó chặn và trả lý do kèm cách làm
đúng, model tự sửa ở lượt sau. Áp dụng cho cả chat chính, subagent và worker.

| Bị chặn | Model được bảo làm gì |
|---|---|
| Read một file dài hơn 500 dòng mà không có `limit`, hoặc `limit` quá 500 | Grep tìm đoạn cần, rồi Read với `offset` + `limit` |
| `cat` / `type` / `Get-Content` in cả một file dài hơn 500 dòng | như trên |
| `cat`, `grep`, `rg`, `head`, `tail`, `sed`, `awk`... vào đường dẫn có rule deny `Read(...)` | không đọc, kể cả qua shell |

Không bị chặn: đọc một khoảng (`offset` + `limit`), `cat file | grep ...`, `cat file > file-khác`, `head -n 20`,
`Get-Content -TotalCount 40`, ảnh, PDF, notebook.

- Rule deny lấy từ `permissions.deny` trong `.claude/settings.json` và `settings.local.json` của project. Claude Code
  tự chặn tool Read theo các rule này; guard chặn thêm đường vòng qua shell.
- Đổi ngưỡng: thêm `"WJ_GUARD_MAX_LINES": "800"` vào khối `env` của settings. `0` là bỏ kiểm tra số dòng (rule deny
  vẫn giữ).
- Guard không bao giờ làm hỏng phiên: JSON lạ, file không đọc được, lệnh shell phức tạp (here-document, `cd` rồi mới
  đọc) đều được cho qua. Nó chặn trường hợp rõ ràng, không cố đoán.
- Lệnh của hook là `command -v wj >/dev/null 2>&1 && wj guard || true`: máy chưa có `wj`, hoặc `wj` bản cũ chưa
  có lệnh `guard`, thì hook im lặng cho qua. Muốn có guard thì `wj update`.
- Đã thử thật (Claude Code 2.1.322 trên Windows, 2026-11-12): Read cả file 900 dòng, `cat` file đó và `cat .env`
  đều bị chặn, model nhận đúng lý do; khi lệnh trong hook không tồn tại thì Read chạy bình thường, không có lỗi hook.

## Chạy task song song: orchestrator và worker

Worker là subagent của Claude Code, không phải phiên riêng. Bạn chỉ nói chuyện với một phiên (mở ở đâu cũng được,
kể cả trong Orca):

```text
/orca-orchestrator <mục tiêu>     chia thành task trong PLAN.md, đưa bạn xem, rồi dừng
/orca-orchestrator go             chạy các task sẵn sàng (tối đa 4 worker một lượt)
/orca-orchestrator merge T-01     gộp nhánh của task vào nhánh hiện tại, xoá worktree, archive
```

Mỗi worker (`.claude/agents/orca-worker.md`) nhận đúng một dòng của `PLAN.md`, làm trong worktree riêng dưới
`.claude/worktrees/`, chạy `wj verify`, commit lên nhánh của worktree đó, rồi trả về một dòng
`TASK: ... | STATUS: PASS|PARTIAL|BLOCKED | ...`. Orchestrator ghi kết quả vào `PLAN.md` và in lệnh
`git diff HEAD...<nhánh>` để bạn xem. Nó không tự merge: phải có lệnh `merge` của bạn.

Orchestrator chọn model cho worker theo task, ghi ở cột Agent:

| Task | Agent |
|---|---|
| SP 1-2, đã rõ từng bước (đổi tên, sửa nhỏ, thêm test, docs) | `claude:haiku` |
| SP 3-5, việc thường | `claude:sonnet` |
| Rủi ro hoặc chưa rõ (đồng thời, bảo mật, migrate dữ liệu, đổi nhiều module) | `claude:opus` |

Task bị BLOCKED sau khi đã thử thật thì chạy lại ở bậc cao hơn. Muốn dùng model khác thì nêu tên cho
orchestrator; `haiku` `sonnet` `opus` là bậc, gateway của bạn quyết định mỗi bậc là model nào (xem [Model](model.md)).

## Memory và dreaming

Memory có ba chỗ. Chỗ nháp ở local, bản đã duyệt vào git:

| Ở đâu | Vào git? | Ai ghi | Ai đọc |
|---|---|---|---|
| `.claude/memory/` (ở checkout chính, mọi worktree dùng chung) | Không | Claude Code tự ghi trong phiên chính. Đây là **hộp nháp**: chưa ai duyệt | Phiên chính (file chỉ mục `MEMORY.md` được nạp ở mọi phiên) |
| `AGENTS.md` mục `## Lessons` | Có | `/orca-dream`, sau khi bạn duyệt | Mọi phiên, mọi worker, cả nhóm, và mọi CLI đọc `AGENTS.md` |
| `CLAUDE.local.md` mục `## Lessons` | Không | `/orca-dream`, sau khi bạn duyệt | Mọi phiên và worker trên máy này. Chỉ cho thứ riêng của máy (chỗ cài tool, cổng local) |

- Bài học đã duyệt nằm trong git để có lịch sử và rollback, và để người khác trong nhóm (kể cả người dùng bản
  lite) cùng có. Viết nó ngoài các khối `wj:begin` / `wj:end`, vì `wj init` ghi lại phần trong khối.
- `wj init` ghi `autoMemoryDirectory` vào `.claude/settings.local.json` để Claude Code thôi lưu memory dưới
  `~/.claude/projects/`. File này không vào git nên mỗi worktree bạn mở phiên chính cần chạy `wj init` một lần.
- Worker không tự ghi memory: nhiều worker ghi cùng một file sẽ đè nhau, và một worker hiểu sai sẽ làm hỏng cả
  đợt sau.
- Đã thử (Claude Code 2.1.322, 2026-11-12): subagent chạy trong worktree riêng vẫn nhận được nội dung
  `CLAUDE.local.md` của phiên chính, dù file không có trong worktree đó.

Một phiên chỉ thấy chính nó: nó không biết năm phiên khác cũng vấp đúng lỗi đó, và không quay lại xoá ghi chú đã
sai. `/orca-dream` làm hai việc đó, ngoài phiên làm việc:

```text
wj digest                  7 ngày của project: lỗi tool lặp lại, bị từ chối, handover không PASS,
                           rồi danh sách ghi chú memory và độ dài các mục Lessons
wj digest -days 14
wj digest -session 1acf    mọi lỗi của một phiên
/orca-dream                đọc digest, đề xuất tối đa 5 sửa lỗi + 5 thay đổi memory, kèm chứng cứ
```

**Lỗi lặp lại.** `wj digest` đọc transcript thay cho model (mỗi file nặng nhiều MB) và tính cả subagent lẫn mọi
worktree của repo. Lệnh chỉ thấy lệnh thoát với mã lỗi: lỗi bị che sau `| tail` hay `|| true` không được đếm.
`/orca-dream` ưu tiên cách sửa mà harness cưỡng chế được (quyền trong `settings.json`, lệnh Verify, bước trong
`orca-worker.md`) rồi mới tới một dòng bài học.

**Lọc memory.** Mục MEMORY của digest liệt kê mỗi ghi chú một dòng (file, loại, tuổi, số dòng, mô tả), cũ nhất
trước. `MISSING:` nghĩa là ghi chú nhắc tới một đường dẫn không còn trong project; đây là gợi ý để kiểm tra, không
phải kết luận. Digest cũng báo dòng chỉ mục trỏ tới file đã mất và ghi chú không có trong chỉ mục. `/orca-dream`
mở tối đa 10 ghi chú mỗi lần (có `MISSING` trước, rồi cũ nhất) và cho mỗi cái một kết luận:

| Kết luận | Khi nào | Làm gì |
|---|---|---|
| SHARE | Quy ước hay cạm bẫy của project, còn đúng, phiên khác và worker cần | Thêm một dòng vào `## Lessons` của `AGENTS.md`, xoá ghi chú |
| KEEP | Về bạn (vai trò, sở thích), về máy này, hoặc trỏ tới thứ ngoài repo | Không đổi |
| MERGE | Hai ghi chú nói cùng một điều | Giữ cái tốt hơn, xoá cái kia |
| DELETE | Nhắc tới thứ đã mất; repo đã ghi sẵn (code, git log, `AGENTS.md`, docs); trạng thái task đã xong | Xoá ghi chú và dòng của nó trong `MEMORY.md` |

Ghi chú loại `user` không bao giờ bị chia sẻ hay xoá khi bạn chưa nói. Mỗi mục `## Lessons` giữ dưới 40 dòng, vì
dòng nào ở đó cũng tốn token ở mọi phiên. `/orca-dream` không ghi gì trước khi bạn duyệt và không tự commit.

## Worker kiểm tra task: wj verify

```text
wj verify T-01     kiểm tra task T-01 trong worktree đang đứng
wj verify          không có task: chỉ chạy kiểm tra của project
```

Lệnh liệt kê file đã đổi, cảnh báo file ngoài Scope (`WARN out-of-scope:`), rồi chạy lệnh Verify của task.
Verify là `-` thì tự đoán: Unity EditMode test, script test trong `package.json` (hoặc `tsc --noEmit`), pytest,
`go test`. Dòng cuối luôn là một trong ba:

| Dòng cuối | Mã thoát | Nghĩa |
|---|---|---|
| `VERIFY: PASS` | 0 | kiểm tra qua |
| `VERIFY: FAIL (...)` | 1 | thay đổi làm hỏng gì đó, tính một lần hỏng |
| `VERIFY: NOT_VERIFIED (...)` | 2 | chưa kiểm tra được gì (thiếu `node_modules`, không có thay đổi...) |

Task không có Scope mà sửa `AGENTS.md`, `CLAUDE.md`, `PLAN-ARCHIVE.md` hay `.claude/` thì có dòng
`WARN shared file:`. Biến tuỳ chọn: `UNITY_EXE`, `UNITY_TEST_PLATFORM`, `VERIFY_TAIL` (xem `wj help verify`).

## PLAN.md: một bảng duy nhất

`PLAN.md` chỉ có một bảng Markdown, mỗi task một dòng, không có gì khác (không tiêu đề, không
ghi chú, không danh sách). Nhờ vậy `wj` lấy một task chỉ cần đọc đúng một dòng.

```text
| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-01 | Thêm kiểm tra JWT | P1 | 3 | DONE | claude:sonnet | - | `src/auth.py` | `pytest -q` | Thêm kiểm tra hạn token | worktree-agent-a1b2c3 |
| T-02 | Sửa timeout pool DB | P0 | 2 | BLOCKED | claude:haiku | T-01 | `src/db/` | - | timeout sau 3 lần retry | - |
```

| Cột | Giá trị |
|---|---|
| Pri | `P0` gấp, `P1` cao, `P2` thường, `P3` để sau (thay cho Backlog) |
| SP | Story point kiểu agile: `1` `2` `3` `5` `8` `13`, hoặc `?` chưa ước lượng. Từ 8 nên tách nhỏ |
| Status | `TODO` `DOING` `DONE` (chưa merge) `BLOCKED` `PARTIAL` |
| Agent | `claude` (model của phiên đang chạy), hoặc `claude:<model>` để worker chạy bằng model đó. `codex` `agy` vẫn hợp lệ nhưng kit hiện không cài gì cho chúng |
| Depends | ID cách nhau dấu phẩy, hoặc `-` |
| Scope | Đường dẫn trong backtick, hoặc `-` |
| Verify | Một lệnh trong backtick, hoặc `-` để tự đoán |
| Notes | Tối đa 100 ký tự |
| Session | Nhánh worker đã commit, hoặc `-` |

Không ô nào chứa ký tự `|` hay xuống dòng. Chỉ orchestrator sửa `PLAN.md`, và sửa đúng một dòng
mỗi lần; task mới thêm ở cuối, ID tăng dần, không đánh lại số. Task đã merge chuyển sang
`PLAN-ARCHIVE.md`.

```text
wj plan            task sẵn sàng chạy (ưu tiên cao trước), đang chạy, bị chặn
wj plan check      kiểm tra format, in lỗi kèm số dòng
wj plan log        20 task đã archive gần nhất (-last 50; -find T-12: một ID hoặc chữ trong Task / Notes)
wj plan convert    đổi PLAN.md kiểu cũ (checkbox + Backlog) sang bảng, cập nhật skill Orca
                   của project; bản cũ được sao lưu .bak-<giờ>
wj plan archive T-01 T-02                 chuyển task đã merge sang PLAN-ARCHIVE.md
wj plan archive T-01 -commit 9fb158c      ghi rõ commit merge
wj plan archive T-01=9fb158c T-02=3f38ef1 mỗi task một commit
```

`PLAN-ARCHIVE.md` cũng là bảng: đủ các cột của `PLAN.md`, thêm `Commit` (commit merge) và
`Archived` (ngày). Không ghi `-commit` thì tool lấy commit mới nhất trong `git log` có nhắc ID
task; không tìm thấy thì ô Commit là `-`. Chữ cũ trong `PLAN-ARCHIVE.md` được giữ nguyên, bảng nối
vào cuối. Chỉ task `DONE` hoặc `PARTIAL` mới archive được.

Khi đổi kiểu cũ, tiêu đề dài bị cắt còn 120 ký tự (bản đầy đủ ở file .bak), Pri mặc định `P1`
cho task, `P2` cho dòng lẻ trong Queue, `P3` cho Backlog, và SP là `?`: chỉnh lại sau.

## Tự kiểm tra máy: wj doctor

```text
wj doctor              kiểm tra, rồi hỏi sửa từng mục (Enter = sửa)
wj doctor -yes         sửa hết không hỏi
wj doctor -dry-run     chỉ báo
wj doctor -ai          nhờ AI qua gateway của bạn gợi ý thêm
```

Lệnh tìm:

- Phiên Claude bị tắt tool search giữa chừng (đọc cache tăng vọt, xem [Model](model.md)).
  Sửa: cho project đi qua proxy wj, bật proxy chạy nền.
- Proxy đang chạy addon cũ. Sửa: ghi addon mới, khởi động lại proxy nền.
- Project bật đo token mà proxy không chạy (Claude ở đó không kết nối được). Sửa: bật proxy nền.
- MCP cài cho mọi project nhưng chỉ vài project dùng trong 30 ngày. Sửa: chuyển về đúng các
  project đó bằng chính lệnh `claude mcp`, không gọi được thì bỏ khỏi phạm vi chung.
- `PLAN.md` kiểu cũ hoặc sai format.

`-ai` chỉ gửi các dòng báo cáo (đường dẫn, số đếm) lên gateway trong `~/.claude/settings.json`,
không gửi key hay code. Không có gateway hoặc mất mạng thì vẫn chạy với các luật sửa cứng.

Proxy nền: `wj proxy bg` bật, `wj proxy stop` tắt, `wj proxy restart` nạp addon mới.

## So cấu hình giữa các agent

```text
wj check
```

Với Claude, Codex, Antigravity, Cursor và OpenCode, lệnh cho biết đã cài chưa, đang kết
nối kiểu gì, có MCP server và skill nào, và agent nào đang thiếu so với agent khác.

Lệnh chỉ đọc, không cần mạng, và chỉ in tên, không in key.
