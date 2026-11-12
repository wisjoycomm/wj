# Project

## Setup ban đầu

```text
cd <project>
wj init                    tự nhận job, hoặc hỏi
wj init -type unity,web    chọn job
wj init -dry-run           xem trước
wj init -update            project cài bằng kit cũ: lấy skill Orca mới
wj init -lite              chỉ phần không cần wj (xem "Hai mức" bên dưới)
wj init -remove            gỡ kit: chỉ phần của wj (khối wj trong AGENTS.md, file giống bản kit,
                           hook wj guard, quyền Orca); file bạn đã sửa giữ nguyên, file gỡ vào
                           thùng rác. Thêm -dry-run để chỉ xem danh sách
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
- Chọn model ở bản lite: không có orchestrator nên không có bảng `.claude/models.md`. Bạn tự chọn model của phiên
  bằng `/model`; `CLAUDE.md` chỉ dặn một điều: subagent chỉ tìm và đọc thì chạy `haiku`, và đừng mở subagent cho việc
  một lượt Grep hay Read làm được.
- Bài học ở bản lite: tự viết vào mục `## Lessons` của `AGENTS.md` (ngoài các khối `wj:begin`). Không có
  `/orca-dream` lọc giúp, nên giữ mục này ngắn.
- Người không cài `wj`: mỗi bản release có `wj-kit-lite-<job>.zip` (`general`, `unity`, `web`, `go`, `python`), do
  chính `wj init -lite` sinh ra. Giải nén vào gốc project **mới**. Project đã có `AGENTS.md`, `.gitignore` hay
  `.claude/settings.json` thì đừng giải nén đè: gộp tay, vì zip không biết gộp như `wj init`.
- Nâng từ lite lên đủ: cài `wj` rồi chạy `wj init`. Tool chỉ thêm phần còn thiếu.
- Repo cài mức đủ, nhưng một người trong nhóm không có `wj`: hook tự bỏ qua (không báo lỗi, không chặn gì), rule
  và file cấm đọc vẫn có tác dụng. Skill orchestrator và worker chuyển sang làm tay các bước `wj plan`, `wj verify` (tốn token hơn); riêng
  `/orca-dream` cần `wj digest` nên báo thiếu rồi dừng.

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

`wj` là phần phụ, không bắt buộc. Skill và worker gọi `wj plan`, `wj verify`; máy không có `wj` (hoặc `wj` cũ hơn số
ghi trong `.claude/wj-kit`) thì chúng làm tay các bước đó, tốn token hơn. Hook `wj guard` thì tự bỏ qua. Riêng
`/orca-dream` cần `wj digest`. `wj init` ghi phiên bản `wj` đã cài kit vào `.claude/wj-kit` (commit file này; bản
lite không có file này); `wj init -update` ghi lại số mới. Project cài bằng
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
- Tắt hẳn cho một project: thêm `"WJ_GUARD": "off"` vào khối `env` đó. Hook cho qua mọi thứ, và bạn xoá được khối
  `hooks` mà `wj init` hay `wj doctor` không thêm lại. Các phần khác của kit (file cấm đọc, giới hạn output) vẫn giữ.
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

### Model nào làm việc nào

Orchestrator chọn model cho từng task lúc lập kế hoạch và ghi vào cột Agent. Bảng chọn nằm trong
`.claude/models.md`: file này của bạn, `wj init` không ghi đè, và nó không được nạp vào phiên nên không tốn token
cho tới khi lập kế hoạch. Bản mặc định:

| Loại việc (dòng khớp đầu tiên thắng) | Agent |
|---|---|
| Bảo mật: đăng nhập, quyền, mã hoá, secret, kiểm tra input | `claude:opus` |
| Toán và thuật toán: code số, hình học, thống kê, tối ưu, parser | `claude:opus` |
| UI: bố cục, style, component, chỉnh giao diện | `claude:sonnet` |

| Việc còn lại, theo cỡ và rủi ro | Agent |
|---|---|
| SP 1-2, đã rõ từng bước (đổi tên, sửa nhỏ, thêm test, docs) | `claude:haiku` |
| SP 3-5, việc thường | `claude:sonnet` |
| Rủi ro hoặc chưa rõ (đồng thời, migrate dữ liệu, đổi nhiều module) | `claude:opus` |

Task bị BLOCKED sau khi đã thử thật thì chạy lại ở bậc cao hơn. `haiku` `sonnet` `opus` là bậc, gateway của bạn
quyết định mỗi bậc là model nào (xem [Model](model.md)).

**Model hãng khác (GPT, Gemini, DeepSeek...).** Worker chạy được bằng bất kỳ model nào gateway phục vụ cho Claude
Code. Tool Agent của Claude Code chỉ nhận `sonnet`, `opus`, `haiku`, `fable` làm model của subagent, nhưng một file
subagent thì ghi được model id bất kỳ. Vì vậy mỗi model hãng khác có một bản worker riêng:

```text
wj models test                  chỉ dùng id có OK ở cột Claude Code
wj plan worker cx/gpt-6.1-sol   tạo .claude/agents/orca-worker-cx-gpt-6-1-sol.md (model: cx/gpt-6.1-sol)
                                và thêm quyền Agent(...) vào settings
wj plan worker                  liệt kê worker đã tạo
```

Rồi sửa `.claude/models.md`, ví dụ đổi dòng UI thành `claude:cx/gpt-6.1-sol`, commit, và mở lại Claude Code
(subagent mới chỉ được nạp khi mở phiên). `wj plan` in mục WORKERS cho orchestrator biết task nào chạy bằng
subagent nào, và báo `MISSING` kèm lệnh cần chạy nếu chưa tạo. Chạy lại `wj plan worker <id>` sau khi
`orca-worker.md` đổi để bản riêng theo kịp.

Model hãng khác qua Claude Code không được hãng hỗ trợ chính thức: tool hoặc phiên dài có thể lỗi dù bài thử ngắn
qua. Giao task nhỏ trước. Đã thử (Claude Code 2.1.322, 2026-11-12): subagent có `model: cx/gpt-6.1-sol` gửi đúng id
đó lên API; chưa thử chạy một task thật bằng model hãng khác.

**Agent khác (Codex, Antigravity).** Cột Agent cũng nhận `codex`, `agy`, hoặc kèm model: `codex:<model>`,
`agy:<model>`, và dùng được trong `.claude/models.md` như mọi giá trị khác. Orchestrator của Claude Code chỉ tự chạy
dòng `claude`; task `codex` / `agy` nó để `TODO` và liệt kê ra để bạn chạy trong app đó. Dòng task, Scope và Verify
giống nhau cho mọi agent.

### Unity: chung checkout

Mỗi worktree là một bản checkout đủ `Assets/` và không có `Library/`, nên với Unity mỗi task sẽ phải import lại cả
project. Vì vậy project có job `unity` không dùng worktree: mọi worker làm ngay trong checkout chính, và chỉ có
một Unity Editor (bản bạn đang mở) cho mọi việc cần Editor. Project không có job `unity` giữ nguyên worktree.

**Cài.** `wj init` với job `unity` (kể cả `-type unity,web`) cài `.claude/agents/orca-worker.md` từ
`kit/stacks/unity/dot-claude/agents/orca-worker.md` thay cho bản chung của Orca. Bản Unity không có dòng
`isolation: worktree` và không có dòng `tools:`, để worker dùng được MCP server đang kết nối. `-update`, `-remove`,
`-dry-run` và mục kit của `wj doctor` so với đúng bản đã cài cho job đó. Bản lite không cài worker.

**Worker Unity.** Nhiều worker cùng sửa một checkout, mỗi worker một Scope không trùng file:

- Chỉ đụng file trong Scope (và `.meta` đi kèm). File khác đang đổi là của worker khác: không sửa, không hoàn tác.
- Commit đúng file của mình: `git add -- <file trong Scope>` rồi `git commit -m "<ID>: ..." -- <các file đó>`,
  không dùng `git add -A`. Gặp lỗi `index.lock` thì chờ vài giây và thử lại, tối đa 3 lần. Không stash, reset,
  checkout hay đổi nhánh.
- Task script (chỉ `.cs`, `.asmdef`...): sửa file, không mở Unity, không chạy batchmode, rồi chạy `wj verify <ID>`.
- Task cần Editor (scene, prefab, uGUI, asset, chạy test): chỉ làm qua tool của Unity MCP server đang kết nối,
  không sửa YAML bằng tay. Không có server nào kết nối thì báo BLOCKED `Unity MCP not connected`. Trước khi sửa:
  refresh asset và đọc console; có lỗi biên dịch thì báo BLOCKED kèm tên file và dòng lỗi, không tự sửa file ngoài
  Scope. Sau khi sửa: lưu scene/prefab qua MCP, đọc lại console, chụp Game view nếu server có tool chụp.
- Dòng handover giữ nguyên dạng `TASK: ... | STATUS: ... | FILES: ... | SUMMARY: ...`.

**`wj verify` ngoài worktree** (thư mục `.git` của checkout trùng với thư mục git chung):

- Chỉ xét file thay đổi nằm trong Scope của task. File đổi ngoài Scope và `PLAN.md` đang đổi không còn là lỗi
  hay `WARN`, vì chúng thuộc worker khác và orchestrator. Không có file nào trong Scope thay đổi thì vẫn là
  `NOT_VERIFIED` như cũ. Task không có Scope: giữ cách kiểm tra cũ.
- Project Unity mà Editor đang mở (`Temp/UnityLockfile` tồn tại): không bật Unity batchmode. Verify là `-` thì
  kết thúc bằng `VERIFY: PASS (scope only: Editor open, compile and tests not run)`; lệnh Verify ghi rõ thì vẫn chạy.
- Project Unity mà Editor không mở: chạy batchmode như cũ, nhưng mỗi lúc một lần: giữ file khoá
  `.tmp/unity-verify.lock` trong lúc chạy, lần khác chờ đến khi khoá nhả (khoá cũ hơn 30 phút coi như bỏ).
- Trong worktree: không đổi gì.

**Orchestrator.** Nhận ra chế độ này khi `.claude/agents/orca-worker.md` không có dòng `isolation: worktree`:

- Task script chạy song song như thường. Task cần Editor (Scope có `*.unity`, `*.prefab`, `*.asset`, hoặc task là
  chạy test / chỉnh UI) chạy mỗi lúc một task, và chỉ khi không còn task script nào `DOING`.
- Ghi kết quả: ô Session là hash commit của task (`git log --format=%h -1 --grep "<ID>:"`), lệnh xem lại là
  `git --no-pager show <hash>`.
- Task cần Editor báo lỗi biên dịch ở một file: thêm một task `P0` sửa file đó (Scope là file đó), rồi chạy lại
  task bị chặn sau khi task sửa xong.
- `merge` không còn gì để gộp: chỉ archive (`wj plan archive T-01=<hash>`).

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
                           chỗ lãng phí context, rồi ghi chú memory và độ dài các mục Lessons
wj digest -days 14
wj digest -session 1acf    mọi lỗi của một phiên
/orca-dream                đọc digest, đề xuất tối đa 5 sửa lỗi + 5 thay đổi memory, kèm chứng cứ
```

**Lỗi lặp lại.** `wj digest` đọc transcript thay cho model (mỗi file nặng nhiều MB) và tính cả subagent lẫn mọi
worktree của repo. Lệnh chỉ thấy lệnh thoát với mã lỗi: lỗi bị che sau `| tail` hay `|| true` không được đếm.
`/orca-dream` ưu tiên cách sửa mà harness cưỡng chế được (quyền trong `settings.json`, lệnh Verify, bước trong
`orca-worker.md`) rồi mới tới một dòng bài học.

**Lãng phí context.** Mục WASTE của digest có hai phần. BIG RESULTS: mỗi kết quả tool từ 8.000 token trở lên,
gộp theo lệnh gây ra nó (số lần, tổng token, phiên nào). READ AGAIN: cùng một file bị Read cả file từ 3 lần trong
một phiên; đọc theo khoảng (`offset` + `limit`) không tính. Một kết quả tool nằm lại trong hội thoại và bị gửi lại ở
mọi lượt sau, nên nó tốn hơn nhiều so với kích thước của chính nó. `/orca-dream` xử lý dòng nào lặp từ 2 lần hoặc
lớn từ 20.000 token: thêm rule deny, thêm cờ quiet vào lệnh Verify, hoặc một dòng rule. Token ở đây là ước lượng
(số ký tự / 4), không phải số đo từ gateway.

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
- Bản sao lưu chương trình `.old-*` còn sót sau các lần `wj update`. Sửa: chuyển vào thùng rác.
- Project đã cài kit mà thiếu `autoMemoryDirectory` (memory không nằm trong project). Sửa: ghi lại
  vào `.claude/settings.local.json`.
- Kit của project đang đứng thiếu hoặc cũ hơn `wj` này. Lệnh chạy thử `wj init` (không ghi gì) và liệt kê thứ
  init sẽ thêm: hook `wj guard`, file cấm đọc, giới hạn output Bash, khối rule. Sửa: `wj init`. Project cài bản
  lite thì chỉ so với bản lite. Project có `CLAUDE.md` hay `AGENTS.md` mà chưa cài kit thì chỉ được nhắc.
- Context cố định quá lớn. Đây là phần nạp ở **mọi lượt của mọi phiên và mọi worker**:
  - rule và memory: `CLAUDE.md` và các file nó import bằng `@` (như `AGENTS.md`), `CLAUDE.local.md`,
    `~/.claude/CLAUDE.md`, chỉ mục `MEMORY.md`. Ngưỡng báo: 3.000 token.
  - danh sách skill và agent: tên + mô tả của từng cái, ở project và ở `~/.claude`. Skill có
    `disable-model-invocation: true` không tính vì model không thấy nó. Ngưỡng báo: 2.000 token.

  Mục này không tự sửa: chạy `/orca-dream` để xoá rule và bài học cũ, hoặc rút ngắn mô tả skill.

- File tạm cũ hơn 7 ngày trong `.tmp/` và `.temp/`, ở project đang đứng (cả checkout chính nếu đang ở worktree) và
  các project đã bật đo token. Lệnh báo số file, dung lượng, file cũ nhất và vài file lớn nhất. Sửa: chuyển đúng các
  file cũ đó **vào thùng rác** (không xoá vĩnh viễn, khôi phục được) và bỏ thư mục con bị rỗng; file mới hơn 7 ngày (như `.tmp/handover.md` đang dùng) được giữ. Rule trong
  `AGENTS.md` dặn agent để file tạm theo `.tmp/<task>/` và xoá thư mục đó trước khi báo xong, nhưng đó chỉ là
  lời nhắc; mục này của doctor là lưới đỡ khi agent quên.

Dòng đầu của `wj doctor` luôn in context cố định của project đang đứng, kể cả khi chưa vượt ngưỡng:

```text
Context cố định của project này: ~1.1k token mỗi phiên (rule và memory ~1.0k, danh sách skill/agent ~136).
```

Con số là ước lượng (byte / 4) và chưa gồm system prompt, tool có sẵn và tool MCP, vì chúng không nằm trong file.

`-ai` chỉ gửi các dòng báo cáo (đường dẫn, số đếm) lên gateway trong `~/.claude/settings.json`,
không gửi key hay code. Không có gateway hoặc mất mạng thì vẫn chạy với các luật sửa cứng.

Proxy nền: `wj proxy bg` bật, `wj proxy stop` tắt, `wj proxy restart` nạp addon mới.

## Xoá vào thùng rác: wj trash

```text
wj trash <file hoặc thư mục>...
```

Lệnh chuyển file vào thùng rác của hệ điều hành thay vì xoá hẳn: Recycle Bin trên Windows, `~/.Trash` trên macOS,
thùng rác XDG (`~/.local/share/Trash`, có file `.trashinfo` để khôi phục) trên Linux. Một đường dẫn không tồn tại
thì lệnh dừng và chưa chuyển gì. Không chuyển được (khác ổ đĩa, hết quyền) thì lệnh báo lỗi và để nguyên file: nó
không bao giờ xoá vĩnh viễn.

- Kit bản đủ dặn agent xoá bằng `wj trash`, không dùng `rm`, và cho phép sẵn `Bash(wj trash *)`.
- `/orca-dream` xoá ghi chú memory đã duyệt bằng lệnh này; `wj doctor` dọn file tạm cũ cũng bằng nó.
- Đã thử trên Windows (2026-11-12): file và thư mục vào Recycle Bin kèm vị trí gốc. Bản macOS và Linux mới được
  kiểm bằng test tự động, chưa thử tay.

## So cấu hình giữa các agent

```text
wj check
```

Với Claude, Codex, Antigravity, Cursor và OpenCode, lệnh cho biết đã cài chưa, đang kết
nối kiểu gì, có MCP server và skill nào, và agent nào đang thiếu so với agent khác.

Lệnh chỉ đọc, không cần mạng, và chỉ in tên, không in key.
