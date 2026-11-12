## Orca: plan and parallel workers
These need the `wj` command. If `wj` is not found, say so and stop; don't work around it.
- Tasks live in `PLAN.md`, one table. Get its state with `wj plan`; don't read the whole file.
- Never read or search `PLAN-ARCHIVE.md`. For an old task run `wj plan log -find T-12`.
- Parallel workers are `orca-worker` subagents, each in its own git worktree; they also follow `.claude/agents/orca-worker.md`.
- Claude Code skills: `/orca-orchestrator <goal>` plans tasks into PLAN.md, then `/orca-orchestrator go` runs them as `orca-worker` subagents. `/orca-dream` reviews past sessions and proposes rule changes.
