# Changelog

What changed for your projects and for you, newest first, a few lines per version. `wj init` shows the entries
newer than the kit a project was set up with.

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
