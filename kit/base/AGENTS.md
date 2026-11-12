# AGENTS.md

Rules for every coding agent in this repo. Claude Code has extra notes in CLAUDE.md.

## Read less
- Find before you read: locate files and symbols first. A file you only look something up in: read the lines you need.
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
- Write docs only when asked, in `docs/`. When you change behavior that a file in `docs/` describes, update that file in the same change.

## Commands
- Non-interactive only: no `git rebase -i`, no `git commit` without `-m`, no pagers, editors or pickers (`less`, `vim`, `fzf`, `lazygit`), and no `gh` command that would prompt (pass all flags).
- Tests run once, never in watch mode. Prefer quiet output.
- Never run `git push`, `git reset --hard` or `git clean` unless the user asks.

## Stop after 3 failures
If the same build or test check fails 3 times in a row, stop and report the exact error. Don't keep looping.
