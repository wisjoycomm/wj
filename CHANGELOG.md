# Changelog

What changed for your projects and for you, newest first, a few lines per version. `wj init` shows the entries
newer than the kit a project was set up with.

## 0.2.13
- Overlay: a small column chart under the total, like `wj usage` (days, weeks or months, the current one in
  colour), and a bar under each project for its share of the total.
- `wj init` on a project from an older kit that still has `scripts/` or `docs/`: Claude Code moves them to
  `.tools/` and `.docs/` and fixes every path that points at them. Files a build, CI or docs site uses stay put.
  It prints what moved where; you review and commit.
- Rules: `.tools/` has one folder per broad area of work, with long plain names for folders and scripts.
- Fix: wj 0.2.11 also copied `settings.local.json` to the project root, where nothing reads it. `wj init` removes
  that copy while it is unchanged.

## 0.2.12
- Rules: scripts to reuse go in `.tools/` and docs in `.docs/` (was `scripts/` and `docs/`), so they sort first.
  Move an existing `scripts/` or `docs/` folder yourself if you want it to match.
- wj repo: `.tools/release.sh` and `/release` do a whole release in one command (`.docs/release.md`).

## 0.2.11
- Agents never run `wj`: skills, worker and rules do every step by hand. `wj` is the tool you run.
- `wj init` merges a file that differs from the kit with Claude Code instead of keeping or replacing it (old file
  kept as `.bak`). `-no-merge` turns it off.
- New file map `.claude/map.md`: every folder with its scripts, assets and main files. `wj digest`, `wj init` and
  `wj map` write it; `/orca-dream` fills its Notes.
- `/orca-dream` reads memory from Claude Code's own folder too, and works from the digest you give it.
- `/orca-orchestrator`: the session stays the orchestrator until `stop orchestrator`; PLAN.md goes into the next
  commit instead of a commit of its own.
- Unity: rules moved into settings (no `Edit` of scenes and prefabs, ask before reading big YAML assets).
- Rules: reply plainly and cite sources; turn a job that repeats into a script plus a skill.
- Memory in `.claude/memory/` is shared through git; `settings.local.json` turns off unused skills.
- `wj top` removed: `wj usage` refreshes itself and shows progress on the first load; the overlay counts the same
  periods as `wj usage`. Model prices refresh daily from OpenRouter.

## 0.2.10
- `wj tools install wsl`: WSL2 and Ubuntu, with a RAM cap in `~/.wslconfig`.
- Connection scope: the whole machine or this project only (Claude, OpenCode).
