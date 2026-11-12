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
- Temp and scratch files (logs, test output, one-off scripts, downloads) go in `.tmp/` at the repo root, which is gitignored. Never leave them in the repo root or next to source files. Delete them when done.
- Don't write summary or report files (`SUMMARY.md`, `CHANGES.md`, `NOTES.md`...) after a task: report in chat instead.
- Write docs only when asked, in `docs/`. When you change behavior that a file in `docs/` describes, update that file in the same change.

## Commands
- Non-interactive only: no `git rebase -i`, no `git commit` without `-m`, no pagers, editors or pickers (`less`, `vim`, `fzf`, `lazygit`), and no `gh` command that would prompt (pass all flags).
- Tests run once, never in watch mode. Prefer quiet output.
- Never run `git push`, `git reset --hard` or `git clean` unless the user asks.

## Stop after 3 failures
If the same build or test check fails 3 times in a row, stop and report the exact error. Don't keep looping.
