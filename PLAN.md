| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-21 | Test: wj plan help says the orchestrator runs claude, codex and agy rows as workers | P2 | 1 | BLOCKED | agy | - | `plan_help_test.go` | `go test -run TestPlanHelp ./...` | Test passes, staged; git commit denied by user's Antigravity deny rule (Orca mode) | wisjoycomm/t-21 |
| T-31 | wj usage shows no Codex tokens: find why (paths, line format, model/session match) and fix | P0 | 3 | DONE | claude:sonnet | - | `usage.go`, `usage_test.go` | `go test ./...` | No bug: Codex counted; last Codex session 2026-11-21 so today shows 0 | - |
| T-32 | Overlay shows no Codex: find why (open Codex sessions, project match, app label) and fix | P0 | 3 | DONE | claude:sonnet | - | `usage.go`, `overlay_windows.go`, `usage_test.go` | `go test ./...` | Codex had no open marker, dropped after 10m; now kept 1h after last call | wisjoycomm/t-32 |
