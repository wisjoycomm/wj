## Orca: plan and parallel workers
`wj` is optional here: without it the skills and the worker do the same steps by hand, which costs more tokens.
- Delete with `wj trash <path>...`, never `rm`: it moves files to the recycle bin, so a wrong delete can be undone (no wj: ask me before deleting anything you did not create in this task).
- Tasks live in `PLAN.md`, one table. Get its state with `wj plan`; don't read the whole file.
- Never read or search `PLAN-ARCHIVE.md`. For an old task run `wj plan log -find T-12` (no wj: Grep the ID in it with `head_limit` 5).
- Parallel workers are `orca-worker` subagents, each in its own git worktree; they also follow `.claude/agents/orca-worker.md`.
- Claude Code skills: `/orca-orchestrator <goal>` plans tasks into PLAN.md, then `/orca-orchestrator go` runs them as `orca-worker` subagents. `/orca-dream` reviews past sessions and memory, and proposes changes.
