# Analysis of agentdriven.dev/settings.json

Source: `curl https://agentdriven.dev/settings.json` (HTTP 200, 2970 bytes, 111 lines, fetched 2026-11-20).
The article text was not read; this covers the settings file only.

## What it is
A Claude Code `settings.json` with three parts: one housekeeping value, one env var, and a `permissions` block (19 `allow` rules, 80 `deny` rules).

## Settings

| Key | Value | Meaning |
|---|---|---|
| `cleanupPeriodDays` | `36500` | Keep session transcripts about 100 years (default is 30 days). Effectively "never delete". |
| `env.CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY` | `"3"` | Run at most 3 tool calls in parallel. Not checked: whether this variable is in the official docs. |

## Allow rules (run without asking)
- Read-only or harmless shell: `ls`, `echo`, `git log`, `git remote get-url`, `git check-ignore`, `gh run list`, `npm view`.
- Git writes: `git add *`, `git commit *`. Commit is allowed, push is not mentioned.
- Package managers: `npm install` (exact, no arguments), `pnpm install`, `pnpm add`, `brew install`.
- Checks: `npx tsc`, `pnpm tsc`, `pnpm typecheck`, `pnpm eslint`, `pnpm prettier`.
- `Bash(pnpm:*)` allows every pnpm subcommand.

## Deny rules (always blocked)
1. **Secrets via CLI** (8): `sst secret`, `aws secretsmanager`, `aws ssm get-parameter(s)`, `cat .env*`, `cat ~/.aws/*`, `cat ~/.ssh/*`.
2. **Secret files via Read/Grep** (about 20): `.env`, `.env.*`, `application.yml`, at any depth.
3. **Personal folders and credentials in `~`** (about 45): Desktop, Documents, Downloads, Library, Movies, Music, Pictures, Public, `.aws`, `.ssh`, `.gnupg`, `.config`, `.docker`, `.kube`, `.npm`, `.cargo`, `.rustup`, `.netrc`, `.npmrc`, shell and psql history.
4. **Outward-facing GitHub actions** (17): create, comment, review, edit, merge, close or reopen PRs and issues, create, edit or delete releases, create gists. The agent can read GitHub but not write to it.

## Design idea
Let the agent work freely on code and local checks. Block secrets and anything that speaks for the user to the outside world. Prompt for everything else.

## Weak points
- **Redundant rules.** `Bash(pnpm:*)` already covers `pnpm install`, `add`, `tsc`, `typecheck`, `eslint`, `prettier`. The narrow rules add nothing. Worse, `pnpm:*` allows `pnpm exec` and `pnpm dlx`, which run arbitrary code.
- **Deny on `cat` is easy to bypass.** `Bash(cat .env*)` does not stop `head .env`, `grep KEY .env`, `less`, or `cp .env x`. The `Read(...)`/`Grep(...)` rules only cover Claude's own tools, not the shell. Real protection needs a sandbox or a hook. (From general knowledge of how Claude Code matches rules; not tested here.)
- **`Bash(echo:*)` can write files** with `echo x > file`. Only if shell redirection is not matched separately; not tested.
- **`git push` is neither allowed nor denied**, so it prompts. Fine, but `git add`/`commit` are auto-allowed.
- **macOS-shaped.** `~/Library`, `~/Movies`, `brew` do nothing on Windows or Linux. Windows paths such as `C:\Users\...` are not covered.
- **Project-specific.** `sst`, `application.yml`, `pnpm` and TypeScript reflect the author's stack. Copy only what matches yours.
- **`cleanupPeriodDays: 36500`** keeps all transcripts forever. They can contain secrets that were printed in tool output, and they grow without bound.
- **`deny` for `.env` uses exact names.** Files like `secrets.json` or `*.pem` are not blocked.

## Takeaways for this repo
- The deny-list pattern (secrets, home credentials, GitHub write commands) is worth copying. Adapt the paths to Windows.
- Use one broad allow per tool instead of many narrow ones, or the reverse, but not both.
- Treat deny rules on Bash as a hint, not a wall.
