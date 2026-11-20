# Which model runs which task

The orchestrator reads this file when it plans and writes the result in the Agent column of PLAN.md.
It is yours to edit: kit updates never overwrite it. It is not loaded into sessions, so it costs no tokens
until a plan is made.

## By kind of work
The first row that fits the task wins.

| Kind of work | Agent |
|---|---|
| Security: auth, permissions, crypto, secrets, input validation | `codex:cx/gpt-6.2-astra` |
| Math and algorithms: numeric code, geometry, statistics, optimization, parsers | `codex:cx/gpt-6.2-astra` |
| UI and images: layout, styling, components, visual polish, accessibility, images and graphics | `codex:cx/gpt-6.2-sol` |

## By size and risk
For every task no row above fits.

| Task | Agent |
|---|---|
| SP 1-2 and fully specified: a rename, a small fix, a test for existing behavior, docs | `claude:haiku` |
| SP 3-5: ordinary feature or bug work | `claude:sonnet` |
| Risky or unclear whatever its size: concurrency, data migration, a change across modules | `claude:opus` |

## Models of other vendors
A worker can run on any model the gateway serves to Claude Code (GPT, Gemini, DeepSeek...). To use one:

1. I check that the model id works in Claude Code with a short session.
2. I create that model's worker `.claude/agents/orca-worker-<id slug>.md` (a copy of `orca-worker.md` with
   `model: <id>`). Commit it and reopen Claude Code.
3. Write `claude:<id>` in the Agent cell of the rows it should take, for example
   `| UI: layout, styling, components, visual polish, accessibility | claude:cx/gpt-6.1-sol |`.

Another vendor's model through Claude Code is not officially supported: tools or long sessions can fail
where a short test passed. Give it small tasks first.

## Other coding agents
The Agent cell can also name another coding agent: `codex`, `agy` (Antigravity), or one with a model, as in
`codex:<model>` and `agy:<model>`. Use them in the tables above like any other value.
The orchestrator runs `codex` and `agy` rows itself as workers, like `claude` rows: in an Orca tab (Orca mode),
or with `codex exec` or `agy -p` in its own worktree (subagent mode). Antigravity models: run `agy models`. The task's row, Scope and Verify are the same for every agent.

## When a task comes back BLOCKED
After a real attempt, relaunch it one step up: `claude:haiku` -> `claude:sonnet` -> `claude:opus`.
A task that another vendor's model could not finish goes to `claude:opus`. That includes `codex` tasks.
