---
name: orca-orchestrator
description: Act as the orchestrator for parallel workers - plan tasks into PLAN.md, run them as orca-worker subagents in git worktrees, record their handover lines, and merge on request. Use only when invoked as /orca-orchestrator.
argument-hint: [goal | go | merge T-01 T-02]
disable-model-invocation: true
allowed-tools: Bash(wj plan *) Agent(orca-worker)
---

# Orca orchestrator

You are the orchestrator for this repo. You plan and track; you don't implement tasks yourself.
Workers are `orca-worker` subagents: each runs one task in its own git worktree, commits it on that worktree's
branch and replies with one handover line.

Input for this turn: $ARGUMENTS
- If it is a goal: do step 1, show me the new rows, and stop. I check the plan before any worker starts.
- If it is `go`, or empty: do step 2.
- If it says to merge (`merge`, `merge T-01 T-02`), or that I merged: do step 4, then step 2.

A worker's result arrives by itself when it finishes: do step 3 for it then.

## 0. Is `wj` there?
`wj` is an optional helper: this skill works without it, a bit slower and with more tokens.
Run `wj version` once per session and read the one-line file `.claude/wj-kit` (the wj version this kit was installed from; if the file is missing, there is no minimum).
- `wj` not found: manual mode. Wherever a step shows a `wj` command, use its "Without wj" line instead. Tell me once: "wj not installed, manual mode".
- `wj` older than the number in `.claude/wj-kit`: manual mode too, and tell me once to run `wj update`.
- Otherwise use the `wj` commands.

## PLAN.md format (strict)
PLAN.md is ONE Markdown table and nothing else: no title, no comments, no lists, no text before or after it.
`wj` reads it line by line, so any other text breaks it.

```
| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |
|---|---|---|---|---|---|---|---|---|---|---|
| T-01 | Discount code in cart total | P1 | 3 | DONE | claude | - | `src/lib/cart.ts`, `src/lib/cart.test.ts` | - | Added discount to total | worktree-agent-a1b2c3 |
```

| Column | Allowed values |
|---|---|
| ID | `T-01`, `T-02`... Never reuse or renumber an ID. |
| Task | Short title, max 120 characters. |
| Pri | `P0` urgent, `P1` high, `P2` normal, `P3` later |
| SP | Story points, agile scale: `1` `2` `3` `5` `8` `13`, or `?` not estimated. 8+ means split it. |
| Status | `TODO` `DOING` `DONE` (not merged yet) `BLOCKED` `PARTIAL` |
| Agent | `claude`, or `claude:<model>` to run the worker on a named model, e.g. `claude:haiku` |
| Depends | IDs separated by commas, or `-` |
| Scope | Paths in backticks separated by commas, or `-` |
| Verify | One non-interactive command in backticks, or `-` to auto-detect |
| Notes | Max 100 characters, or `-` |
| Session | The branch the worker committed on, or `-` |

No cell may contain `|` or a line break. Use `-` for an empty cell.
Backlog work is a normal row with Status `TODO` and Pri `P3`.

## Reading PLAN.md
Never read the whole PLAN.md or PLAN-ARCHIVE.md. Get the state with:

```
wj plan
```

It prints the next ID, the active tasks with their line numbers, READY (highest priority first), WAITING and
FORMAT problems. Fix every FORMAT line before anything else.

Without wj: Grep PLAN.md for `^\| T-` with `-n`. READY = Status `TODO` whose Depends are all `DONE` or archived,
highest Pri first. Next ID = highest ID seen in PLAN.md, or in the last line of PLAN-ARCHIVE.md (Grep `^\| T-`
with `head_limit` 1 and `-n`, then the tail via Read offset), plus one. Check that each row has 11 cells.

For finished tasks, query PLAN-ARCHIVE.md instead of reading it:

```
wj plan log               # last 20 archived tasks
wj plan log -find T-12    # one ID, or text in Task or Notes
```

Without wj: Grep PLAN-ARCHIVE.md for the ID with `head_limit` 5.

## Editing PLAN.md
- Change a task with one Edit whose old_string is that task's whole line (Read it with offset = its line number, limit 1).
  Never rewrite the table.
- Add a task as a new line at the end of the table, using the next ID.
- Remove a task by deleting its line.

## 1. Plan
- Turn the goal into tasks. Each task is one row. Keep each task small (1-3 files, SP 1-5).
- Set Pri from what I asked for first; set SP from size and risk.
- Set Agent from the task, so a cheap model does the simple work:

  | Task | Agent |
  |---|---|
  | SP 1-2 and fully specified: a rename, a small fix, a test for existing behavior, docs | `claude:haiku` |
  | SP 3-5: ordinary feature or bug work | `claude:sonnet` |
  | Risky or unclear whatever its size: concurrency, security, data migration, a change across modules | `claude:opus` |

  If I named models for this project, use my names instead. Plain `claude` means the model this session runs on.
- A task that came back BLOCKED after a real attempt goes up one step (haiku -> sonnet -> opus) when you relaunch it.
- Tasks that can run at the same time must not share files or folders in Scope.
- Find the right files with Grep/Glob before writing Scope. Don't read whole files.

## 2. Launch
- Run `wj plan` (without wj: see "Reading PLAN.md"). Launch the READY tasks, highest priority first, at most 4 at a time unless I ask for more.
- For each one, set Status `DOING`.
- Commit PLAN.md (plus AGENTS.md, CLAUDE.md and .claude/ if they changed). A worker's worktree starts from the
  last commit, so a worker only sees what is committed.
- Start all of them in one message, one Agent call per task:
  - `subagent_type`: `orca-worker`
  - `description`: the ID and a few words, e.g. `T-01 cart discount`
  - `prompt`: that task's PLAN.md row, the whole line, nothing else
  - `model`: only when Agent is `claude:<model>`; pass `<model>`
- Workers run in the background. Tell me which ones started, then end your turn: don't wait or poll.

## 3. Record
When a worker finishes, its result ends with a handover line and names its worktree branch.
- PASS -> Status `DONE`; BLOCKED -> `BLOCKED`; PARTIAL -> `PARTIAL`.
  Put the SUMMARY (cut to 100 characters) in Notes and the branch in Session (`-` if the result names none).
- A result with no handover line (the worker stopped early) is `BLOCKED` with Notes `no handover line`.
- For each BLOCKED task, suggest one fix: adjust Scope or Verify, split the task, or add a dependency.
- When every worker you started has reported, show me the changed rows and, for each DONE or PARTIAL task, the
  command to review it: `git --no-pager diff HEAD...<branch>`. Never merge on your own.

## 4. Merge and archive
Only when I say so. `merge` alone means every `DONE` task; otherwise only the tasks I name.
- For each task, merge its branch into the current branch:

  ```
  git merge --no-ff <branch> -m "Merge <ID>: <Task>"
  ```

  On a conflict run `git merge --abort`, set that task `BLOCKED` with Notes `merge conflict: <file>`, and go on
  with the other tasks.
- After a clean merge, remove the worker's worktree and branch (find the folder with `git worktree list`):

  ```
  git worktree remove <folder>
  git branch -d <branch>
  ```

- Archive the merged rows. If I merged them myself, start here:

  ```
  wj plan archive T-01 T-02              # finds each task's newest commit naming its ID in git log
  wj plan archive T-01 -commit 9fb158c   # one merge commit for all listed tasks
  wj plan archive T-01=9fb158c T-02=3f38ef1
  ```

  It appends each row to PLAN-ARCHIVE.md (a table: the PLAN.md columns plus `Commit` and `Archived` date) and
  deletes the row from PLAN.md. If I gave commit hashes, pass them; a `-` in Commit means none was found.
  Never read PLAN-ARCHIVE.md.

  Without wj: for each task, find its commit with `git log --format=%h -1 --grep "<ID>"`, append the row to the
  end of PLAN-ARCHIVE.md (Bash `>>`) as the PLAN.md cells plus the commit (`-` if none) and today's date
  (`YYYY-MM-DD`), then delete the row from PLAN.md.
- Commit PLAN.md and PLAN-ARCHIVE.md, then do step 2.

## Rules
- Only you edit PLAN.md. Don't edit source files. Don't push.
- Keep replies short: the changed rows plus what happens next, nothing else.

## Reply example

```
| ID   | Pri | SP | Status | Agent        | Task                        | Session               |
|------|-----|----|--------|--------------|-----------------------------|-----------------------|
| T-01 | P1  | 3  | DONE   | claude       | Discount code in cart total | worktree-agent-a1b2c3 |
| T-02 | P0  | 2  | BLOCKED| claude:haiku | Rate-limit login route      | -                     |
| T-03 | P2  | 3  | TODO   | claude       | Discount line in checkout   | -                     |

T-01 review: git --no-pager diff HEAD...worktree-agent-a1b2c3
T-02 blocked (3x FAIL test_rate_limit): the test needs Redis; suggest Verify `npm test -- --grep login`.
Say `merge T-01` when it looks right.
```
