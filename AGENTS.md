<!-- wj:begin base -->
# AGENTS.md

Rules for every coding agent in this repo. Claude Code has extra notes in CLAUDE.md.

## Read less
- Find before you read: locate files and symbols first. A file you only look something up in: read the lines you need.
- `.claude/map.md`, when the repo has it, lists every folder with its scripts, its assets by kind and its main files. Read it to find where something lives before a wide search.
- A file you are going to change: read it whole when it is under 500 lines. For a longer one, search for the part you need and read that range; one read loads at most 500 lines.
- Before you change a function or a type, search for where it is used, and read the rest of its file when the change touches something other code shares. Don't edit from a fragment you have not understood: reading more costs less than a wrong fix.
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
- Write docs only when asked, in `.docs/`. When you change behavior that a file in `.docs/` describes, update that file in the same change.

## Commands
- Non-interactive only: no `git rebase -i`, no `git commit` without `-m`, no pagers, editors or pickers (`less`, `vim`, `fzf`, `lazygit`), and no `gh` command that would prompt (pass all flags).
- Tests run once, never in watch mode. Prefer quiet output.
- Never run `git push`, `git reset --hard` or `git clean` unless the user asks.

## Security
- Don't read `.env` files, `~/.ssh`, `~/.aws` or other secret files. If a task needs a secret, write a script for the user to run instead.
- Don't run remote commands (deploy, SSH, cloud CLIs). Leave them to the user or the coordinator.

## Replies
- Start with the answer or the result. No preamble, no apology, no closing pleasantries, no restating the question.
- Short, plain words and short sentences. Explain so a newcomer can follow: say what it is and why, not only the term.
- Never invent facts, paths, commands, flags, versions or numbers. If you did not check it, say "not checked" or "not sure". Say what is missing; don't guess.
- A claim from a file, a command output, a doc or a web page gets its source: `path:line`, the command you ran, or the URL. A claim from memory is marked as such.

## Reuse, don't repeat
- When a job comes up a second time, or will clearly come up again (a setup, a data check, a log query, a release step), even a one-step one, turn it into a script in `.tools/` instead of doing the steps by hand again. One script per job, non-interactive, quiet by default, with a `-h` that says what it does. Group scripts in one folder per broad area of work (`.tools/release-new-version/`, `.tools/check-game-balance-data/`), not one folder per script and not a flat pile; give folders and scripts long, plain names that say what they do.
- Then add a short skill `.claude/skills/<job>/SKILL.md`: when to use it, the exact command, and how to read its output. Before a job, check `.tools/` and the skills for one that already does it, and fix that script rather than writing a second one. `.tools/` and `.docs/` start with a dot so they sort first; some searches skip dot folders, so list them by path.
- Say in your report which script and skill you added or changed.

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
- Ask me before deleting anything you did not create in this task.
- Tasks live in `PLAN.md`, one table. Grep it for `^\| [A-Z]+-` with `-n`; don't read the whole file.
- Never read or search `PLAN-ARCHIVE.md`. For an old task Grep the ID in it with `head_limit` 5.
- Parallel workers each run in their own git worktree and follow `.claude/agents/orca-worker.md`: Orca sessions started with the `orca` CLI, or `orca-worker` subagents when Orca is not running.
- Claude Code skills: `/orca-orchestrator <goal>` plans tasks into PLAN.md, then `/orca-orchestrator go` runs them as workers. `/orca-dream` reviews past sessions and memory, and proposes changes.
<!-- wj:end orca -->
