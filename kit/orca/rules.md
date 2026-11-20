## Orca: plan and parallel workers
Never run `wj` yourself: it is a tool I run outside the agent. Do every step by hand.
- Ask me before deleting anything you did not create in this task.
- Tasks live in `PLAN.md`, one table. Grep it for `^\| T-` with `-n`; don't read the whole file.
- Never read or search `PLAN-ARCHIVE.md`. For an old task Grep the ID in it with `head_limit` 5.
- Parallel workers each run in their own git worktree and follow `.claude/agents/orca-worker.md`: Orca sessions started with the `orca` CLI, or `orca-worker` subagents when Orca is not running.
- Claude Code skills: `/orca-orchestrator <goal>` plans tasks into PLAN.md, then `/orca-orchestrator go` runs them as workers. `/orca-dream` reviews past sessions and memory, and proposes changes.
