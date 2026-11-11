@AGENTS.md

## Claude Code only
- Search and read with your built-in tools, not shell commands (`rg`, `grep`, `find`, `cat`, `bat`, `ls`):
  - Grep (it runs ripgrep and skips gitignored files) for code search. Use output_mode `files_with_matches` or `count` and `head_limit` to keep results small.
  - Glob for finding files by name.
  - Read with offset/limit for line ranges. On most models Edit only works on a file opened with Read.
- Use Bash only for what has no built-in tool: `git`, `gh`, `jq`, `wj`, builds and test runners.
- Edit: keep `old_string` to the smallest unique snippet (usually 2-5 lines). Don't stretch it to include the enclosing function header.
- Skills: `/orca-orchestrator <goal>` plans tasks into PLAN.md, then `/orca-orchestrator go` runs them as `orca-worker` subagents. `/orca-dream` reviews past sessions and proposes rule changes.
