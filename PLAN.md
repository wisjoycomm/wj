| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-20 | Test: kit and repo orchestrator skill are equal and name codex exec and agy -p workers | P2 | 2 | DONE | codex:cx/gpt-6.2-sol | - | `orchestrator_skill_test.go` | `go test -run TestOrchestratorSkill ./...` | Added skill parity and worker command checks | wisjoycomm/t-20 |
| T-21 | Test: wj plan help says the orchestrator runs claude, codex and agy rows as workers | P2 | 1 | BLOCKED | agy | - | `plan_help_test.go` | `go test -run TestPlanHelp ./...` | Test passes, staged; git commit denied by user's Antigravity deny rule (Orca mode) | wisjoycomm/t-21 |
