| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-18 | Overlay right-click menu: time window 1h/24h/7d, opacity and text size; saved in overlay.json with the position | P2 | 5 | DOING | claude:sonnet | T-17 | `overlay_windows.go` | `go vet ./... && go test ./... && go build ./...` | overlay.json holds only X,Y now; old files must still load. Flags -since, -warn beat saved values | - |
| T-19 | Docs and help for the new overlay look, warn bar and menu settings | P2 | 1 | TODO | claude:haiku | T-18 | `docs/token.md`, `help.go` | `go vet ./... && go test ./...` | Describe only what T-15 to T-18 shipped: read their commits first | - |
