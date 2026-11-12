<!-- wj:begin base -->
# AGENTS.md

Rules for every coding agent in this repo. Claude Code has extra notes in CLAUDE.md.

## Read less
- Find before you read: locate files and symbols first, then read only the lines you need. Don't read whole files over ~200 lines.
- One read loads at most 500 lines: for a longer file, search for the part you need and read that range.
- Never read lockfiles, generated files, build output or caches, with a file tool or through the shell (`cat`, `grep`, `head`...).

## Keep output small
- Builds and tests: use the quiet flags and show only the errors and the end of the output. Send a long log to a file in `.tmp/` and search it.
- Search: list or count the matches first, then read the few that matter.
- Large JSON, logs and data files: query them (`jq`, a search), never print the whole file.

## Edit, don't rewrite
- Change existing files with small targeted edits or patches, never by rewriting the whole file. Create whole files only when they are new.
- Keep each edited snippet as small as possible while still unique in the file.

## Files you create
- Temp and scratch files (logs, test output, one-off scripts, downloads) go in `.tmp/<task or topic>/` at the repo root, which is gitignored: one folder per task, so it can be found and removed as a whole. Never use another folder (`.temp/`, `tmp/`), the repo root or a place next to source files.
- Before you report a task done, delete its folder in `.tmp/`. Keep only what the next step needs, and name it in your report.
- Don't write summary or report files (`SUMMARY.md`, `CHANGES.md`, `NOTES.md`...) after a task: report in chat instead.
- Write docs only when asked, in `docs/`. When you change behavior that a file in `docs/` describes, update that file in the same change.

## Commands
- Non-interactive only: no `git rebase -i`, no `git commit` without `-m`, no pagers, editors or pickers (`less`, `vim`, `fzf`, `lazygit`), and no `gh` command that would prompt (pass all flags).
- Tests run once, never in watch mode. Prefer quiet output.
- Never run `git push`, `git reset --hard` or `git clean` unless the user asks.

## Stop after 3 failures
If the same build or test check fails 3 times in a row, stop and report the exact error. Don't keep looping.
<!-- wj:end base -->

<!-- wj:begin go -->
## Go
- Run `gofmt -l .`, `go vet ./...` and `go test ./...` before calling a change done.
- Never edit `go.sum` by hand. Run `go mod tidy` only when dependencies changed.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`) and handle them; don't discard them without a reason.
- Prefer the standard library. Don't add a dependency without saying why in your report.
- Use `t.TempDir()` and `t.Setenv` in tests, so they never touch the real machine.
<!-- wj:end go -->

<!-- wj:begin orca -->
## Orca: plan and parallel workers
`wj` is optional here: without it the skills and the worker do the same steps by hand, which costs more tokens.
- Delete with `wj trash <path>...`, never `rm`: it moves files to the recycle bin, so a wrong delete can be undone (no wj: ask me before deleting anything you did not create in this task).
- Tasks live in `PLAN.md`, one table. Get its state with `wj plan`; don't read the whole file.
- Never read or search `PLAN-ARCHIVE.md`. For an old task run `wj plan log -find T-12` (no wj: Grep the ID in it with `head_limit` 5).
- Parallel workers are `orca-worker` subagents, each in its own git worktree; they also follow `.claude/agents/orca-worker.md`.
- Claude Code skills: `/orca-orchestrator <goal>` plans tasks into PLAN.md, then `/orca-orchestrator go` runs them as `orca-worker` subagents. `/orca-dream` reviews past sessions and memory, and proposes changes.
<!-- wj:end orca -->
