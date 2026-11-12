# Wisjoy wj

Formerly `setup-gateway`. Existing installs keep working: the first normal run moves
`~/.setup-gateway` to `~/.wj` and adds a `wj` command beside the old one. The
environment variables `SETUP_GATEWAY_KEY` and `SETUP_GATEWAY_PROJECT` keep their names.

One standalone CLI for setting up a development machine: the original
**Bo_cong_cu** toolkit plus Claude, Codex, OpenCode, Cursor, Antigravity and Orca.
Windows, macOS and Linux builds embed all toolkit scripts. No Go or Node.js is
needed to run the downloaded executable.

Vietnamese instructions: [HUONG-DAN.md](HUONG-DAN.md). Short guides for each part: [docs/](docs/README.md).

## Start on a new Windows machine

Open **CMD** and paste this single command. It downloads and verifies the release
and installs it into your user PATH. It does not open anything: run `wj` when you want it.

```bat
powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm 'https://github.com/wisjoycomm/wj/releases/latest/download/install.ps1')))"
```

No manual download or extraction is needed. You can also run `install.cmd` from
a source checkout. In later terminals, run `wj` again. A terminal that
was already open may need to be restarted before it sees the updated PATH.

For an offline transfer of the CLI itself:

Download and extract `wj-windows-amd64.zip` from GitHub Releases, then
double-click **Run-Setup.cmd**. The menu offers:

- 1 new-machine wizard, 2 project setup, 3 AI agents (connect, profiles, gateway models, config check, token views), 4 applications (install, update, toolkit, window manager), 5 update wj.
- `h` quick help, `0` exit.

Run `wj help` for a plain-language overview, or `help claude`, `help codex`,
`help proxy`, `help apps`, `help tools`, `help init`, `help check`, `help words` (glossary). Menus, prompts and
errors are in Vietnamese; on Windows the console is switched to UTF-8 while it runs.

The menu returns after each operation (including an error); choose `0` to exit.
Every list (apps, models, profiles, clients) is numbered: type `1 2 9`, a range
`2-4`, or `all`; names are still accepted.
Opening it, listing applications and reading the version make no network requests.
The executable and toolkit scripts remain on your machine. You do not clone or
download the repository again for normal use. Installation skips detected apps;
updating them is a separate action. Applications may have their own auto-updaters.

The menu asks before starting a toolkit module. Package installers can request
administrator privileges. Docker and removal of an existing GlazeWM installation
have separate prompts. A new-machine setup does not require installing every group.

To install the CLI into your user PATH from a published release:

```powershell
& ([scriptblock]::Create((irm 'https://github.com/wisjoycomm/wj/releases/latest/download/install.ps1')))
```

Or download `install.ps1` and run `./install.ps1`. Add `-RunSetup` to open the menu straight after installing. Use `-Version vX.Y.Z`
to select a release. `-InstallDir PATH -NoPath` installs into a selected directory
without modifying PATH. The installer detects Windows x64/ARM64, verifies SHA-256,
and makes the command available in the current and future terminals.

macOS / Linux:

```sh
curl -fsSL https://github.com/wisjoycomm/wj/releases/latest/download/install.sh | sh
~/.local/bin/wj
```

The Unix installer requires a release containing `checksums.txt`. It supports
Intel/Apple Silicon macOS and x64/ARM64 Linux. Add `~/.local/bin` to PATH if prompted.
For a source checkout that has not been released yet, use the local build below;
the public install commands fetch the latest **published** binary, not local changes.

## Set up a project

```text
wj init                  ask which job this is (detected ones are suggested)
wj init -type unity,web  choose directly
wj init -dry-run         preview, write nothing
wj init -update          project set up by an older kit: take the current Orca skills
```

Run it anywhere inside a project; it works from the Git root. The kit targets Claude
Code. It installs the shared agent rules and any job rules (`unity`, `web`, `go`,
`python`) into `AGENTS.md`, plus `CLAUDE.md`, `PLAN.md`, an empty `PLAN-ARCHIVE.md`
table, the `/orca-orchestrator` and `/orca-dream` skills and the `orca-worker` subagent
in `.claude/`, Claude permissions (allowed commands, and `Read` deny rules for
lockfiles, caches and `.env`) and `.gitignore` entries. It also keeps Claude Code's
auto memory inside the project (`.claude/memory/`, set through `autoMemoryDirectory`
in the uncommitted `.claude/settings.local.json`) instead of under `~/.claude`. To add a job,
add a folder under `kit/stacks/` and an entry in `initStacksList`. The kit keeps
`.claude/` as `kit/base/dot-claude/`, so Claude Code working on this repo doesn't
load the kit's skills as its own.

Existing files are safe. Files that already exist are kept and reported, including
edited skills. In `AGENTS.md` only the text between `wj:begin` and `wj:end` markers
is refreshed, and shared rules are not added to an `AGENTS.md` that wj did not
create. `.claude/settings.json` and `.gitignore` only gain entries, and a rule you
already set is left as it is. `-update` replaces Orca skills that differ from the
kit's (the old file is backed up) and removes what older kits installed: the `.ps1`
scripts and the `orca-stateless-worker` skill.

Parallel work runs inside one Claude Code session. `/orca-orchestrator <goal>` plans
tasks into `PLAN.md`; `/orca-orchestrator go` starts each ready task as an
`orca-worker` subagent in its own git worktree, on the model its Agent column names;
the worker verifies, commits on its branch and reports one line; `/orca-orchestrator
merge T-01` merges and archives on your say. `wj digest` condenses the failures in the
project's Claude Code transcripts (repeated tool errors, denied commands, handovers
that were not PASS) and `/orca-dream` turns them into proposed fixes for you to approve.

The skills and the worker call `wj plan`, `wj verify` and `wj digest`, so the machine
needs `wj` on PATH; nothing else. `wj verify T-01` lists the changed files, warns about
files outside the task's Scope, runs the task's Verify command (or detects one:
Unity EditMode tests, the `package.json` test script, pytest, `go test`) and ends
with one line: `VERIFY: PASS`, `VERIFY: FAIL (...)` or `VERIFY: NOT_VERIFIED (...)`.

`PLAN.md` is a single Markdown table and nothing else, one task per row:
`| ID | Task | Pri | SP | Status | Agent | Depends | Scope | Verify | Notes | Session |`
(Pri `P0`-`P3`, SP agile points `1 2 3 5 8 13 ?`, Status `TODO DOING DONE BLOCKED PARTIAL`).
Agent is `claude`, or `claude:<model>` for a worker that runs on a named model.
`wj plan` lists ready tasks by priority, `wj plan check` validates the format,
`wj plan log` queries archived tasks and `wj plan convert` turns an old checkbox plan into the table and refreshes the
project's Orca skills (the old files are backed up). See [docs/project.md](docs/project.md).

## Doctor

`wj doctor` checks the machine and offers to fix what it finds: Claude sessions that
lost tool search mid-session (gateway `tool_reference` bug), an outdated or stopped
metering proxy, user-scope MCP servers that only one or two projects use, and old
or broken `PLAN.md` files. `-dry-run` only reports, `-yes` fixes everything, and
`-ai` asks the configured Claude gateway for advice on the report (only the report
lines are sent). `wj proxy bg|stop|restart` manages a background proxy.

## Compare agents

`wj check` shows, for Claude, Codex, Antigravity, Cursor and OpenCode, whether
each is installed, how it connects, whether it has a user-level instructions file,
and its MCP servers and skills. It then lists what some installed agents have and
others lack. It only reads, needs no network and prints names, never values.

## Bundled tools

### Applications and updates

```text
wj apps list
wj apps install claude,codex,opencode,cursor-agent,cursor,antigravity,antigravity-ide,orca,mitmproxy
wj apps install all -dry-run
wj apps update codex,orca
wj version
wj update
```

| App ID | Windows | macOS | Linux |
| --- | --- | --- | --- |
| `claude` | Official native installer | Official installer | Official installer |
| `codex` | Official native installer | Official installer | Official installer |
| `opencode` | WinGet `SST.opencode` | Official installer | Official installer |
| `cursor-agent` | Official Cursor CLI installer | Official installer | Official installer |
| `cursor` | WinGet `Anysphere.Cursor` | Homebrew cask | Manual vendor download |
| `antigravity` (alias `agy`) | Official CLI installer | Official installer | Official installer |
| `antigravity-ide` | WinGet `Google.AntigravityIDE` | Homebrew cask | Manual vendor download |
| `orca` | Official upstream NSIS installer | Upstream ZIP to `~/Applications/Orca.app` | Upstream AppImage to `~/.local/bin/orca-ide` |
| `mitmproxy` | WinGet `mitmproxy.mitmproxy` | Homebrew formula | `uv tool install mitmproxy` |

Use `apps install all` to select every entry. On Linux, the two manual desktop
installers report unsupported and the remaining selections still run. Native
Windows needs WinGet/App Installer for the listed WinGet packages; macOS needs
Homebrew for casks. The developer-toolkit module can install prerequisites.
For manual Linux desktop downloads: [Cursor](https://cursor.com/downloads) and
[Antigravity IDE](https://antigravity.google/download).

Orca comes from [stablyai/orca](https://github.com/stablyai/orca/releases), the
upstream referenced by the installed Orca updater. The `wisjoycomm/orca` fork
does not currently publish installers. Orca and wj release assets
are SHA-256 verified and cached under the OS user-cache directory in
`wj/downloads`. Retrying reuses verified cached assets. Upstream
script installers and package managers manage their own downloads/caches.
Orca's Linux AppImage needs FUSE, or use `orca-ide --appimage-extract-and-run`.

`wj update` replaces this executable after checksum verification;
restart the command to use it. A running Windows image can leave an adjacent
`.old-<timestamp>` file, removable after the previous process exits. The
Windows bootstrap also reuses an already installed executable with the same hash.

### Per-client authentication

```text
wj configure claude
wj configure codex
wj configure opencode
wj configure cursor
wj configure orca
```

Claude retains all four methods described below. Codex supports normal ChatGPT
login, device-code login, direct OpenAI API keys, and Responses-compatible gateways:

```text
wj configure codex -auth login
wj configure codex -auth login -device-auth
wj configure codex -auth api-key
wj configure codex -auth gateway -base-url https://example.com -models YOUR_MODEL_ID
```

Keys use a hidden prompt, `-key-stdin` or `SETUP_GATEWAY_KEY`. Codex direct API
keys go through `codex login --with-api-key` on stdin. Gateway settings respect
`CODEX_HOME`, set the selected default model, and use the Responses API.
Switching to normal login clears user-level provider/model/forced-login overrides;
projects, MCP servers and other settings are retained. Existing named providers
remain available. TOML comments/formatting can change; the original is backed up.
Old shell `OPENAI_BASE_URL` values can still affect ordinary launches outside this
tool; project/managed settings also take precedence where applicable.

OpenCode supports its normal provider login flow, direct OpenAI/Anthropic API
keys, and OpenAI-compatible gateways:

```text
wj configure opencode -auth login
wj configure opencode -auth api-key -provider-id openai
wj configure opencode -auth api-key -provider-id anthropic
wj configure opencode -auth gateway -base-url https://example.com -models YOUR_MODEL_ID
```

OpenCode uses the current `provider`, `npm` and `options` schema, merges existing
JSON/JSONC settings and backs up the edited file. With normal login, use `/models`
in OpenCode to choose the account/provider; existing gateway entries remain.
Use `-skip-check -models YOUR_MODEL_ID` if a gateway has no `/v1/models` route.
Model discovery does not prove that generation endpoints are compatible.

Cursor/Antigravity account sign-in happens in their own UI/CLI. `configure` shows
the relevant steps; it does not import Claude tokens or invent unsupported
gateway settings. Orca uses the separately configured agents: open Orca, add a
project folder and select the desired installed agent.

Official references: [Codex installation](https://developers.openai.com/codex/cli),
[Codex authentication](https://developers.openai.com/codex/auth),
[OpenCode providers](https://opencode.ai/docs/providers/),
[Cursor CLI installation](https://cursor.com/docs/cli/installation),
[Antigravity downloads](https://antigravity.google/download).

### Original Bo_cong_cu modules

```text
wj
wj onboard
wj tools list
wj tools install cli
wj tools install cli -groups 1,2,4
wj tools install wm
wj tools install wm -dry-run
wj tools uninstall cli
wj tools uninstall wm
```

| Group | Contents |
| --- | --- |
| CLI 1: terminal | Starship, Nerd Font, fzf, zoxide; PowerShell 7, Windows Terminal and Clink on Windows |
| CLI 2: utilities | ripgrep, fd, bat, eza, jq, lazygit, delta, btop, GitHub CLI |
| CLI 3: development | Git, fnm/Node.js, uv/Python, Neovim, Antigravity IDE, optional Docker |
| CLI 4: agents | Claude Code, Codex CLI, Antigravity CLI |
| Window manager | Windows: Komorebi + whkd. macOS: AeroSpace. Linux: configuration for existing i3/Hyprland |

Availability varies by OS/package manager. Shell setup includes the original
marked profile blocks, aliases, prompt/font, Git defaults and Node LTS setup.
Window-manager setup changes window layout, shortcuts, startup and taskbar/Dock
integration. Linux users install their desktop session separately.

The scripts come from the owner's `Bo_cong_cu`; see [module provenance](toolkit/README.md).
Their original package-selection and uninstall prompts remain available. Uninstall
can remove previously installed applications if you select them; use only the
configuration cleanup when you want to retain those applications.

## Claude: four authentication methods

Run `wj auth` for a guided choice. The wizard detects missing Claude
Code and offers its official installer. For flag-based setup, add
`-install-claude` to install it if missing. Git for Windows is optional; installing
the toolkit's development/agent group provides it.

| Method | Result in Claude settings | Credential |
| --- | --- | --- |
| `oauth` / `session-key` | `CLAUDE_CODE_OAUTH_TOKEN` | Full `sk-ant-oat...` OAuth token supplied by your account administrator |
| `api-key` | `ANTHROPIC_API_KEY` | Anthropic Console API key, direct API billing |
| `gateway` | `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN` | Custom domain and gateway-issued Bearer credential |
| `login` | Removes old credential/routing overrides, runs `claude auth login --claudeai` | Normal browser login |

`session-key` is an alias for the OAuth method discussed here. A browser
`sessionKey` cookie (`sk-ant-sid...`) is rejected: it is not a Claude Code OAuth
credential. An OAuth token must not be pasted into the gateway API-key field.
The CLI rejects that mistake before sending a request.

### OAuth token, no browser login

```text
wj auth -auth oauth -install-claude -profile work
```

Paste the full `sk-ant-oat...` token at the hidden prompt. You do not need to run
`claude setup-token` if an administrator has already supplied a valid token.
OAuth tokens are saved without a verification request; Claude checks validity
on the first model request. A token from `claude setup-token` is documented as
lasting one year, but can be revoked earlier. Its prefix alone does not reveal
its remaining lifetime.

### Direct Anthropic API key

```text
wj auth -auth api-key -install-claude -profile direct-api
```

Uses the official Anthropic endpoint and `x-api-key`. Model discovery validates
access without generating a completion. Use `-skip-check` to configure offline.

### Custom gateway/domain

```text
wj auth -auth gateway -only claude -base-url https://ai.adnx.vn -profile adnx
```

Enter a key issued by **that gateway**, not the OAuth token. A bare domain,
`/v1` URL, or pasted `/v1/models` URL is accepted. HTTPS is required except for
localhost development endpoints. Path prefixes such as `/proxy/v1` are retained.

If the gateway expects `x-api-key` instead of `Authorization: Bearer`:

```text
wj auth -auth gateway -only claude -base-url https://example.com -auth-header x-api-key
```

A gateway without model discovery can be configured with explicit model IDs:

```text
wj auth -auth gateway -only claude -base-url https://example.com -skip-check -models cc/claude-opus,cc/claude-sonnet,cc/claude-haiku
```

Use the actual IDs from your gateway; the example IDs are placeholders. Explicit
IDs can also use provider-specific aliases. Discovery checks `/v1/models` only;
it does not prove that every advertised model can answer a generation request.

### Normal browser login

```text
wj auth -auth login -install-claude
```

To prepare configuration without opening the browser yet:

```text
wj auth -auth login -no-login
claude auth login --claudeai
```

## Save and switch connections

```text
wj -list-profiles
wj -use-profile adnx
wj -use-profile work -run
```

Profiles live in `~/.wj/profiles/`. Saving a profile also activates it.
Activation updates the user Claude settings (`CLAUDE_CONFIG_DIR` is respected),
removes previous auth variables and gateway model aliases such as `cc/...`, then
writes only the selected connection. Hooks, plugins, status line, permissions,
model settings and unrelated environment settings are preserved. Old
`apiKeyHelper`, `modelOverrides`, `modelPicker` and `fallbackModel` routing values
are removed to prevent a previous provider from taking over.

Profiles select one active user configuration; they are not automatic failover
or isolated per-session accounts. Start a fresh Claude session after switching.
Run `/status` to verify the credential source, base URL and model.

`-run` launches Claude with inherited auth/provider/model environment overrides
removed. It does not change the user/machine environment or project/managed
settings. Those other settings may still control the connection. An ordinary
`claude` launch can inherit old terminal variables; remove those or use `-run`.
Managed organizational policies continue to apply.

## Existing multi-client gateway support

The original flag-based invocation remains supported:

```text
wj -base-url https://example.com -only claude,codex,opencode,cursor
wj -base-url https://example.com -provider-id mygw
```

The second command defaults to the original client list. In the interactive menu,
Claude alone is the default; you can select all clients for gateway setup.
In this legacy `auth` command, OAuth, direct Anthropic API-key, browser login
and named profiles are Claude-only. Use `configure codex` or `configure opencode`
for their own authentication methods.
Antigravity CLI has no supported custom endpoint and is reported as skipped.

| Client | Configuration |
| --- | --- |
| Claude | `~/.claude/settings.json`; model aliases discovered from gateway |
| Codex | `~/.codex/config.toml`; gateway Responses API provider and credential |
| OpenCode | `~/.config/opencode/opencode.json` or existing `.jsonc`; provider options and models |
| Cursor | Manual instructions for Settings > Models; no global environment writes |

## Token usage from session files

`wj usage`, `wj top` and `wj overlay` (Windows) read Claude Code and Codex session files,
so they need no proxy or network. See [docs/token.md](docs/token.md).
Gateway model refresh and probing: [docs/model.md](docs/model.md).

## Token usage through mitmproxy

```text
# One command: installs mitmproxy if missing (asks; -yes for scripts), starts a
# private proxy, runs the app, stops the proxy when the app exits.
wj proxy run claude         # also codex, opencode or any command

# Step by step: a long-running proxy shared by several terminals.
wj proxy setup              # install mitmproxy and create its CA once
wj proxy start              # terminal 1, keep open (token metering + web UI at http://127.0.0.1:8081; -no-web, -web-port N, -port N)
wj proxy run claude         # terminal 2; reuses the running proxy

wj proxy usage              # whole machine, all apps, last 24h; -since 168h, -all
wj proxy usage -by project  # per project; also -by project-app, -by model
wj proxy usage -project remora -app codex
wj proxy status
```

One log covers the whole machine and every client. Each call is tagged with a
project: `proxy run` uses the git repository folder name (override with
`-project NAME`). The tag travels as the proxy URL user name, so Claude, Codex
and OpenCode sharing one `proxy start` instance are still told apart.

**Per-project mode (Claude Code).** In a project folder, `wj proxy
project on` writes `HTTPS_PROXY` (tagged with the project), `NODE_EXTRA_CA_CERTS`
and `NO_PROXY` into that project's `.claude/settings.local.json` and makes sure it
is gitignored. Every Claude session started there, including `--continue` and
`--resume`, is then metered while `proxy start` runs; no `proxy run` needed.
`proxy project off` removes only the keys it added; `proxy project list` shows
enabled projects. Codex ignores `model_provider`/`profile` in project-local
config, so it is not covered by this mode; use `proxy run codex`.

`wj apps install mitmproxy` still works if you'd rather install it yourself.

`proxy start` runs mitmdump on 127.0.0.1 with an embedded addon that appends one
JSON line per model call to `~/.wj/proxy/usage.jsonl`: time, project,
client, host, model, HTTP status and input/output/cache token counts from the response
`usage` field (Anthropic Messages, OpenAI Responses and Chat Completions, JSON or
SSE, plus Responses over WebSocket). Prompts, responses and headers are not logged.
The addon also repairs Claude Code tool search for gateways that rename tools
upstream (`Monitor` -> `Monitor_ide`) but not `tool_reference` blocks: after the
first `400 Tool reference '...' not found` from a host it appends the suffix to
those names, so tool search stays on instead of falling back to every tool schema
on every call. Set `--set tool_ref_suffix=` (empty) to disable. See [docs/model.md](docs/model.md).
`proxy run` sets `HTTPS_PROXY`, `NODE_EXTRA_CA_CERTS` and `CODEX_CA_CERTIFICATE`
for that child process only; client configuration and the user environment are
not changed. To use your own mitmproxy instance, load
`~/.wj/proxy/usage_meter.py` with `-s` and run
`proxy run -proxy http://HOST:PORT -ca CA.pem claude`.

## Preview, credentials and backups

```text
wj auth -auth oauth -dry-run
wj auth -auth gateway -only claude -base-url https://example.com -dry-run
wj tools install cli -groups 1,4 -dry-run
```

Dry runs do not write files, contact endpoints, install anything or launch login.
Credentials are hidden during interactive entry and are not printed in previews,
errors or profile listings. Automation can use `-key-stdin` (one input line) or
`SETUP_GATEWAY_KEY`; `-key` exists for compatibility but exposes the value in shell
history/process arguments. Keep real credentials out of commands you share.

Auth profiles and client configuration contain credentials locally. They are not
portable encrypted vaults: protect these files and their backups. Auth/config
writes use owner-only Unix permissions, private temporary files and unique
`.bak-<timestamp>` backups. Windows files inherit the user's directory ACLs.
Malformed Claude settings are refused without replacement. Toolkit scripts retain
their own `.bak-wisjoy` backup conventions.

## Build and verification

Use the Go version declared in `go.mod` (currently Go 1.27.1):

```sh
go test ./...
go vet ./...
go build -o wj .
```

On Windows, build a portable ZIP containing the executable, launcher and README:

```powershell
./build.ps1
# Optional: all six platform/architecture binaries plus checksums
./build.ps1 -All
```

Output is in `dist/` and is excluded from Git. The binary includes every toolkit
script. CI runs Go tests/vet and script syntax checks; installing or uninstalling
real applications and interactive OAuth are deliberately not performed by tests.
Tagging a reviewed release with `v*` triggers the release workflow to build all
binaries, a Windows ZIP and `checksums.txt`. Workflow dispatch builds downloadable
CI artifacts without publishing a release.
