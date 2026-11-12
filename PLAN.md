| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-01 | Machine-level defaults in `wj onboard`: global instructions and MCP servers for each agent | P3 | ? | TODO | claude | - | - | - | - | - |
| T-02 | `wj init -update` to refresh Orca skills in projects made with an older kit | P3 | ? | TODO | claude | - | - | - | - | - |
| T-03 | Make the Orca skill scripts work without `pwsh` on macOS and Linux | P3 | ? | TODO | claude | - | - | - | - | - |
| T-04 | Generate the `.claude` and `.agents` Orca skill copies from one source | P3 | ? | TODO | claude | - | - | - | - | - |
| T-05 | Confirm where Antigravity reads user-level skills and MCP servers on each OS | P3 | ? | TODO | claude | - | - | - | - | - |
| T-06 | Confirm Codex works end to end through the gateway, including `codex resume` | P3 | ? | TODO | claude | - | - | - | - | - |
| T-07 | Token overlay for macOS and Linux | P3 | ? | TODO | claude | - | - | - | - | - |
| T-08 | wj doctor: old `.old-*` program backups (fix: moveToTrash); kit project without autoMemoryDirectory (fix: set it) | P1 | 3 | DONE | claude:sonnet | - | `doctor_local.go`, `doctor_local_test.go`, `doctor.go` | `go vet ./... && go test ./...` | Added doctor checks for old .old-* backups (trash fix) and missing autoMemoryDirectory (set fix) | worktree-agent-aa5ac6df4f4c0e7af |
| T-09 | wj digest: print the memory folder as a full path when it is outside the checkout the command runs in | P2 | 1 | DONE | claude:haiku | - | `digest_memory.go`, `digest_memory_test.go`, `digest.go` | `go vet ./... && go test ./...` | Memory folder shows full path outside the running checkout; go vet and tests pass | worktree-agent-a0f0bf9e4de187b5c |
| T-10 | wj init -remove: take the kit out of a project, only what wj owns; with -dry-run it only lists | P2 | 5 | DONE | claude:opus | - | `init.go`, `init_remove.go`, `init_remove_test.go`, `help.go` | `go vet ./... && go test ./...` | Added wj init -remove: strips wj blocks, unchanged kit files, guard hook, Orca rules; -dry-run lists | worktree-agent-aa83036978abb131f |
