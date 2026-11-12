| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-01 | Machine-level defaults in `wj onboard`: global instructions and MCP servers for each agent | P3 | ? | TODO | claude | - | - | - | - | - |
| T-02 | `wj init -update` to refresh Orca skills in projects made with an older kit | P3 | ? | TODO | claude | - | - | - | - | - |
| T-03 | Make the Orca skill scripts work without `pwsh` on macOS and Linux | P3 | ? | TODO | claude | - | - | - | - | - |
| T-04 | Generate the `.claude` and `.agents` Orca skill copies from one source | P3 | ? | TODO | claude | - | - | - | - | - |
| T-05 | Confirm where Antigravity reads user-level skills and MCP servers on each OS | P3 | ? | TODO | claude | - | - | - | - | - |
| T-06 | Confirm Codex works end to end through the gateway, including `codex resume` | P3 | ? | TODO | claude | - | - | - | - | - |
| T-07 | Token overlay for macOS and Linux | P3 | ? | TODO | claude | - | - | - | - | - |
| T-08 | wj doctor: report old `.old-*` program backups (fix: moveToTrash) and a kit project with no autoMemoryDirectory (fix: set it) | P1 | 3 | DOING | claude:sonnet | - | `doctor_local.go`, `doctor_local_test.go`, `doctor.go` | `go vet ./... && go test ./...` | See removeOldExecutables, planLocalMemory; tests use fakeTrash; leave docs/ to the orchestrator | - |
| T-09 | wj digest: print the memory folder as a full path when it is outside the checkout the command runs in | P2 | 1 | DOING | claude:haiku | - | `digest_memory.go`, `digest_memory_test.go`, `digest.go` | `go vet ./... && go test ./...` | printMemory gets the main repo root, so in a worktree `.claude/memory` misleads; leave docs/ alone | - |
| T-10 | wj init -remove: take the kit out of a project, only what wj owns; with -dry-run it only lists | P2 | 5 | DOING | claude:opus | - | `init.go`, `init_remove.go`, `init_remove_test.go`, `help.go` | `go vet ./... && go test ./...` | Remove wj blocks, kit-identical files, guard hook; keep user rules; delete via moveToTrash; no docs/ | - |
