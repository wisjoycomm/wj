---
name: orca-orchestrator
description: Act as the orchestrator for parallel workers - plan tasks into PLAN.md, run them as Orca workers (or orca-worker subagents) in git worktrees, record their handover lines, and merge on request. Use only when invoked as /orca-orchestrator.
argument-hint: [goal | go | merge T-01 T-02]
disable-model-invocation: true
allowed-tools: Bash(orca status) Bash(orca orchestration worker-start *) Bash(orca orchestration check *) Bash(orca orchestration worker-list *) Agent(orca-worker)
---

# Orca orchestrator

You are the orchestrator for this repo. You plan and track; you don't implement tasks yourself.
Each worker runs one task in its own git worktree by following `.claude/agents/orca-worker.md`, commits it on
that worktree's branch and reports one handover line. A worker is an Orca session I can watch in its own tab
(Orca mode, the default), or an `orca-worker` subagent when Orca is not there (step 0).

Input for this turn: $ARGUMENTS
- If it is a goal: do step 1, show me the new rows, and stop. I check the plan before any worker starts.
- If it is `go`, or empty: do step 2.
- If it says to merge (`merge`, `merge T-01 T-02`), or that I merged: do step 4, then step 2.

A worker's result arrives by itself when it finishes: do step 3 for it then. Never poll for it.

## You stay the orchestrator
Once this skill runs, this whole session is the orchestrator, not only this turn. Every later message of mine
is input for this skill, read as above, even without `/orca-orchestrator` in front of it:
- A new goal, a bug or a change I ask for: plan it as tasks (step 1). Don't edit source files yourself, even
  for a small fix.
- A question about the plan or the workers: answer from PLAN.md and the handover lines.
- A question that needs reading code, running tests or reading a diff: don't do it here. Plan it as a task
  (an investigation task whose answer goes in Notes), or tell me to ask it in another session.
- Work outside the plan (a question unrelated to the tasks, a change to this skill, `.claude/` or AGENTS.md):
  say in one line "do it in a new session, or say stop orchestrator first", and don't do it here.
- `stop orchestrator` or `exit orchestrator`: from then on this session is a normal session again. Say so in
  one line.
After a compaction of the conversation, read this file again before you go on.

## 0. Modes
Run `orca status` once per session.
- It prints `runtimeReachable: true`: Orca mode. Workers are started with the `orca` CLI.
- `orca` not found, or the runtime is not reachable: subagent mode. Tell me once: "Orca not running, workers run
  as subagents". Wherever a step has a "Subagent mode" part, use it.

## PLAN.md format (strict)
PLAN.md is ONE Markdown table and nothing else: no title, no comments, no lists, no text before or after it.
Tools read it line by line, so any other text breaks it.

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
| Agent | Who runs it: `claude`, `codex` or `agy`, alone or with a model, e.g. `claude:haiku`, `claude:cx/gpt-6.1-sol`, `codex:<model>` |
| Depends | IDs separated by commas, or `-` |
| Scope | Paths in backticks separated by commas, or `-` |
| Verify | One non-interactive command in backticks, or `-` to auto-detect |
| Notes | Max 100 characters, or `-` |
| Session | The branch the worker committed on, or `-` |

No cell may contain `|` or a line break. Use `-` for an empty cell.
Backlog work is a normal row with Status `TODO` and Pri `P3`.

## Reading PLAN.md
Never read the whole PLAN.md or PLAN-ARCHIVE.md. Get the state by hand: Grep PLAN.md for `^\| T-` with `-n`.
READY = Status `TODO` whose Depends are all `DONE` or archived, highest Pri first. Next ID = highest ID seen in
PLAN.md, or in the last line of PLAN-ARCHIVE.md (Grep `^\| T-` with `head_limit` 1 and `-n`, then the tail via
Read offset), plus one. Check that each row has 11 cells and fix any that don't before anything else.

For finished tasks, Grep PLAN-ARCHIVE.md for the ID with `head_limit` 5 instead of reading it.

## Editing PLAN.md
- Change a task with one Edit whose old_string is that task's whole line (Read it with offset = its line number, limit 1).
  Never rewrite the table.
- Add a task as a new line at the end of the table, using the next ID.
- Remove a task by deleting its line.

## 1. Plan
- Turn the goal into tasks. Each task is one row. Keep each task small (1-3 files, SP 1-5).
- Set Pri from what I asked for first; set SP from size and risk.
- Set Agent from the task, so a cheap model does the simple work and the right model does the special work.
  Read `.claude/models.md` once: it maps kinds of work (security, math, UI...) and sizes to an Agent value, and
  may name models of other vendors or other coding agents. Follow it exactly; it also says where a BLOCKED
  task goes when you relaunch it. If the file is missing, use this:

  | Task | Agent |
  |---|---|
  | SP 1-2 and fully specified: a rename, a small fix, a test for existing behavior, docs | `claude:haiku` |
  | SP 3-5: ordinary feature or bug work | `claude:sonnet` |
  | Risky or unclear whatever its size: concurrency, security, data migration, a change across modules | `claude:opus` |

  With this table, a task that came back BLOCKED after a real attempt goes up one step (haiku -> sonnet -> opus)
  when you relaunch it. If I named a model for a task in this conversation, use my name. Plain `claude` means the model this session runs on.
- Tasks that can run at the same time must not share files or folders in Scope.
- Find the right files with Grep/Glob before writing Scope. Don't read whole files.

## 2. Launch
- Read the plan (see "Reading PLAN.md"). Launch the READY tasks, highest priority first, at most 4 at a time unless I ask for more.
- For each one, set Status `DOING`.
- Never commit PLAN.md on its own: its changes go into the next commit you make (see "PLAN.md rides along").
  Commit now only if AGENTS.md, CLAUDE.md or .claude/ changed, because a worker's worktree starts from the last
  commit and only sees what is committed; put PLAN.md in that same commit.
- Orca mode: start all of them in one message, one Bash call per task. The row has backticks, so pass it on
  stdin exactly like this, never inside quotes:

  ```
  orca orchestration worker-start --spec-file - --task-title "T-01 cart discount" --worktree new-child --name t-01 --base-branch <current branch> --agent claude --model haiku <<'EOF'
  Read .claude/agents/orca-worker.md and do exactly what it says for this PLAN.md row.
  <that task's PLAN.md row, the whole line>
  EOF
  ```

  - `--name`: the ID in lower case. `--base-branch`: the branch this checkout is on (`git branch --show-current`),
    so the worker starts from its last commit.
  - A task launched again whose folder from the earlier try is still in `git worktree list`: replace `--worktree
    new-child --name ... --base-branch ...` with `--worktree path:<that folder>`, so the worker keeps its changes.
  - `--agent` and `--model`, from the task's Agent:

    | Agent | flags |
    |---|---|
    | `claude` | `--agent claude` |
    | `claude:<model>` (`haiku`, `sonnet`, `opus`, `fable`, or any model id) | `--agent claude --model <model>` |
    | `codex`, `codex:<model>` | `--agent codex`, plus `--model <model>` if given |
    | `agy`, `agy:<model>` | `--agent antigravity`, plus `--model <model>` if given |

  - Each call prints a receipt with `dispatchId` and `workspace` (the worktree folder is the part after `::`).
    If a call fails, don't run it again: set the task back to `TODO` and show me the error.
- Subagent mode: start all of them in one message, one Agent call per task:
  - `description`: the ID and a few words, e.g. `T-01 cart discount`
  - `prompt`: that task's PLAN.md row, the whole line, nothing else
  - `subagent_type` and `model`, from the task's Agent:

    | Agent | subagent_type | model |
    |---|---|---|
    | `claude` | `orca-worker` | (none) |
    | `claude:haiku`, `claude:sonnet`, `claude:opus`, `claude:fable` | `orca-worker` | the name after the colon |
    | `claude:<any other id>` | `orca-worker-` plus the id in lower case with every run of characters other than a-z and 0-9 turned into one `-` | (none: the subagent carries its model) |
    | `codex`, `agy`, with or without a model | don't start it | |

  - If `.claude/agents/<that subagent name>.md` does not exist, don't start that task. Tell me it is missing
    (I create it outside the agent, and a new subagent only loads when Claude Code is reopened).
  - A `codex` or `agy` task stays `TODO`. List these tasks for me with their IDs: I run them in that app.
- Workers run in the background. Tell me which ones started (Orca mode: each has its own tab under this
  workspace in Orca), then end your turn: don't wait or poll.

## 3. Record
Orca mode: Orca wakes you with the worker's message, or with a line telling you to run
`orca orchestration check --run <id>`: run exactly that. Lost track of the workers: `orca orchestration worker-list`.
- `worker_done`: the handover line is in its subject or body. The branch is the one `git worktree list` shows for
  that worker's folder.
- `question`: answer with `orca orchestration reply --id <message id> --body "<answer>"` when the row answers it;
  otherwise ask me.
- `escalation`: the worker stopped without reporting. Set the task `BLOCKED` with Notes `worker stopped: <reason>`
  and tell me; I can look at its tab.

Subagent mode: when a worker finishes, its result ends with a handover line and names its worktree branch.

Both modes:
- PASS -> Status `DONE`; BLOCKED -> `BLOCKED`; PARTIAL -> `PARTIAL`.
  Put the SUMMARY (cut to 100 characters) in Notes and the branch in Session (`-` if the result names none).
- A result with no handover line (the worker stopped early) is `BLOCKED` with Notes `no handover line`.
- For each BLOCKED task, suggest one fix from the handover line and the row only (don't read code to
  find it): adjust Scope or Verify, split the task, or add a dependency.
- When every worker you started has reported, show me the changed rows and, for each DONE or PARTIAL task, the
  command to review it: `git --no-pager diff HEAD...<branch>`. Never merge on your own.

## 4. Merge and archive
Only when I say so. `merge` alone means every `DONE` task; otherwise only the tasks I name.
- For each task, merge its branch without committing yet, archive its row (below), and commit both together, so
  the code and the plan change land in one commit:

  ```
  git merge --no-ff --no-commit <branch>
  (archive the row)
  git add PLAN.md PLAN-ARCHIVE.md
  git commit -m "Merge <ID>: <Task>"
  ```

  On a conflict run `git merge --abort`, set that task `BLOCKED` with Notes `merge conflict: <file>`, and go on
  with the other tasks.
- After a clean merge, remove the worker's worktree and branch (find the folder with `git worktree list`):

  ```
  orca worktree rm --worktree path:<folder> --delete-branch
  ```

  Subagent mode:

  ```
  git worktree remove <folder>
  git branch -d <branch>
  ```

- Archiving a row: find the task's commit with `git log --format=%h -1 --grep "<ID>" <branch>` (or use the hash I
  gave), append the row to the end of PLAN-ARCHIVE.md
  (Bash `>>`) as the PLAN.md cells plus the commit (`-` if none) and today's date (`YYYY-MM-DD`), then delete the
  row from PLAN.md. Never read PLAN-ARCHIVE.md.
- If I merged them myself: archive the rows only; they ride along with the next commit.
- Then do step 2.

## PLAN.md rides along
PLAN.md and PLAN-ARCHIVE.md never get a commit of their own. Whatever you changed in them goes into the next
commit you make anyway: a merge commit (step 4), or a launch commit when AGENTS.md, CLAUDE.md or .claude/ changed
(step 2). Until then they stay uncommitted; that is fine, workers get their row in the prompt. When a round ends
with them still uncommitted, say so in one line.

## Unity mode (shared checkout)
Applies when `.claude/agents/orca-worker.md` has no `isolation: worktree` line. Workers share this checkout, so there are no branches or worktrees.
- Orca mode: start each worker with `--worktree current` and no `--name` or `--base-branch`.
- Script tasks (only `.cs`, `.asmdef`...) run in parallel as usual.
- Editor tasks (Scope has `*.unity`, `*.prefab`, `*.asset`, or the task runs tests / edits UI) run one at a time, and only when no script task is `DOING`.
- Record: Session is the task's commit hash (`git log --format=%h -1 --grep "<ID>:"`); review with `git --no-pager show <hash>`.
- An Editor task that reports a compile error in a file: add a `P0` task with that file as Scope, then rerun the blocked task after it is DONE.
- No merge step: `merge` only archives (with the task's commit hash, as above); the plan change rides along with
  the next commit.

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
