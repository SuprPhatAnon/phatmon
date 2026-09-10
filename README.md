# Phatmon

[github.com/SuprPhatAnon/phatmon](https://github.com/SuprPhatAnon/phatmon)

A local **Go TUI** for monitoring and controlling Codex sessions across multiple
`CODEX_HOME` directories. Built with Bubble Tea, Bubbles, and Lip Gloss.

The landing dashboard combines sessions from every home. It shows working-directory names, task titles,
runtime status, context usage, Git status, activity age, and account quota windows.
Select a session to inspect its conversation, task plan, metrics, files, skills,
MCP configuration, and pending requests.

## Features

- One dashboard across multiple Codex homes, with task status and session drill-down.
- Automatic discovery of the default home and homes under `~/.codex-homes/`.
- Git status, context usage, token counts, and account quota windows.
- Conversation history, live task plans, messaging, steering, and interruption.
- Command/file approval handling and responses to agent questions.
- Skill enable/disable controls and user-level MCP server management.

This is an initial implementation. The terminal UI is available now; a webview is
not implemented. See the connection and metric notes below for integration limits.

## Quick start

Requires Go 1.24+, Git, and a recent Codex CLI on `PATH`. The integration was checked
against `codex-cli 0.154.0`. Phatmon uses each home's existing Codex authentication.

```sh
git clone https://github.com/SuprPhatAnon/phatmon.git
cd phatmon
make init
make build

# Explore synthetic sessions without starting Codex or accessing accounts.
./bin/phatmon --demo

# Monitor your automatically discovered homes.
./bin/phatmon
```

`make build`, `make run`, and `make test` initialize dependencies automatically;
`make init` is useful when preparing the environment separately. `make run` starts
the application, and `make demo` runs the demo. Starting the application creates its
configuration file if missing, including in demo mode.

The Makefile stores Go dependencies, downloaded toolchains (when Go downloads one),
build caches, and temporary build files under `.venv/`. It creates missing directories
and downloads only missing dependencies. This is a Go workspace cache; no Python
virtual environment or activation step is involved. `.venv/` is excluded from Git.
An installed Go toolchain is required to bootstrap it; Phatmon does not install Go
or Codex itself.

Without Make, build with `go build -o bin/phatmon ./cmd/phatmon` or run from source
with `go run ./cmd/phatmon`. Direct Go commands use your normal Go environment;
the project-local cache settings apply to Make targets.

## Install for your user (Linux/systemd)

```sh
make install
~/bin/phatmon
```

The installer copies Phatmon to `~/bin/phatmon` and creates a user systemd app-server
service for every resolved home: `~/.codex`, immediate directory children of
`~/.codex-homes/`, and manually configured or environment-discovered homes. The
configured discovery directory is honored and symlink aliases are deduplicated.
Units go under `~/.config/systemd/user/`; installation reloads systemd and enables
and starts them. No sudo is needed. The TUI runs in your terminal.

Each home gets a `phatmon.env` file containing its matching home and server address:

```sh
export CODEX_HOME='/home/you/.codex-homes/consumable'
export CODEX_REMOTE='ws://127.0.0.1:4500'
```

For your consumable project's direnv `.envrc`, source the generated file:

```sh
source_env "$HOME/.codex-homes/consumable/phatmon.env"
```

For the default home, source `~/.codex/phatmon.env`. The address variable defaults
to `CODEX_REMOTE`; choose another name with
`INSTALL_ARGS="--endpoint-env CODEX_APP_SERVER_URL"`. This is an environment
convention for your client setup to consume. Phatmon does not install a Codex
launcher or implement client routing, and it does not edit your `.envrc` or add
shell startup entries.

The same endpoints are saved in `~/.config/phatmon/config.json`, so the dashboard
connects to the corresponding servers automatically. Other settings are preserved
except `codex_binary`, which records the original CLI's absolute path.
Configuration locations honor `XDG_CONFIG_HOME`.

The defaults are **consumable** on port **4500** and **personal** on **4501** when
those homes exist. Additional homes receive ports starting at **4502**, skipping
already assigned ports. Assignments are saved and reused on subsequent installs.
This allocates configured ports; it does not probe unrelated listeners. All
resolved home directories must already exist and be authenticated. Stop any
manually running servers on the assigned ports before installing.

Use `WORK_ADDR=...` and `PERSONAL_ADDR=...` to override the standard addresses.
For a renamed work home, use `INSTALL_ARGS="--work-name work"`. The installer prints
every home, endpoint, and unit name. The standard units are
`phatmon-codex-work.service` and `phatmon-codex-personal.service`; additional names
include the home name and a path hash.

```sh
systemctl --user list-units 'phatmon-codex-*.service'
journalctl --user -u phatmon-codex-work.service -f
```

Rerun installation to update the binary, services, environment files, or add homes.
Already-running services are not restarted automatically. After changing settings,
restart the affected service when its work is finished:
`systemctl --user restart phatmon-codex-work.service`. Units capture the original
Codex executable and current `PATH`. Enabled user services normally start at login;
installation does not enable lingering after logout. Removing a home directory
does not automatically remove its saved configuration or service.

Upgrading from the earlier launcher implementation removes only Phatmon-marked
Codex launchers and its standard Bash/Zsh startup blocks. A generated legacy
`shell.sh` becomes a harmless comment for any custom startup file still sourcing
it. Other executables and shell configuration are preserved.

`--no-start` writes files without invoking systemd. For a staged installation that
leaves actual home directories untouched, also use `--no-home-env`:

```sh
make install INSTALL_ARGS="--no-start --no-home-env --bin-dir /tmp/phatmon-stage/bin --unit-dir /tmp/phatmon-stage/units --config /tmp/phatmon-stage/config.json"
```

Run `make install INSTALL_ARGS="--help"` for all options.

## Home discovery

On startup, Phatmon discovers `~/.codex` as **personal** and each immediate
subdirectory of `~/.codex-homes/` by its directory name. For example,
`~/.codex-homes/consumable` becomes the **consumable** home. No per-home registration is
needed; each directory should already be a usable Codex home. A distinct
`CODEX_HOME` environment value is also included as **environment**.

```sh
# Inspect discovery without starting Codex or opening the TUI.
./bin/phatmon --list-homes
```

To override discovery for one invocation:

```sh
./bin/phatmon --home personal=~/.codex --home work=~/.codex-homes/consumable
```

Terminal minimum: 65 columns × 18 rows. The **DIRECTORY** column shows the session's
working-directory name (for example, `phatmon` for `/projects/phatmon`); the full path
is available in session details. At 110+ columns, the dashboard also shows context
and Git columns. Those metrics are available in details at any size.

## Navigation

| Location | Keys | Action |
| --- | --- | --- |
| Dashboard | ↑/↓ or j/k, Enter | Select a session and dive into details |
| Anywhere | h | Cycle through all homes and individual homes |
| Dashboard | /, l | Search; toggle live sessions versus saved + live |
| Anywhere | r | Refresh data and quotas; reconnect disconnected homes |
| Dashboard | a | Register an existing Codex home and save the registry |
| Dashboard | x | Forget the filtered home; retain its files |
| Anywhere | n | Create a session in the selected home and directory |
| Details | 1–7, Tab/Shift+Tab | Overview, conversation, plan, Git, skills, MCP, requests |
| Details | ↑/↓, PgUp/PgDn, Home/End | Scroll / select |
| Details | a | Attach to a loaded session or resume stored history |
| Details | m | Compose a message; Ctrl+S sends, Esc keeps the draft |
| Details | i | Interrupt the attached session's active turn |
| Skills | Enter, t | View SKILL.md; toggle enabled state |
| MCP | +, t, d | Add, toggle, or remove a user-level MCP server |
| Requests | Enter, d | Respond/approve, or decline a command/file approval |
| Anywhere | Esc, q | Back/cancel; quit |

Home forms ask for a name, existing directory, and optional local WebSocket
endpoint. MCP forms accept a command with a JSON argument array or an HTTP URL.
Arguments are passed as arguments; Phatmon does not invoke a shell to build commands.

## Connecting to running sessions

By default, Phatmon starts a separate stdio app server for each home. It can inspect
saved history, create sessions, and resume sessions into that server. **Stored
history does not reveal whether an independent CLI is still working on the session.**
The dashboard labels this `STORED · UNKNOWN`.

Phatmon can connect to multiple app servers at once, one per home. Each Codex
terminal must use the same server as Phatmon for its home. Start both servers in
separate terminals and leave them running:

```sh
# Work server: ~/.codex-homes/consumable, port 4500
make server

# Personal server: ~/.codex, port 4501
make server SESSION_NAME=personal
```

Open Codex terminals for either or both homes:

```sh
make codex SESSION_DIR=/path/to/project
make codex SESSION_NAME=personal SESSION_DIR=/path/to/personal/project
```

Then start one Phatmon dashboard connected to **both** servers:

```sh
make live
```

No address arguments are needed. `WORK_ADDR` defaults to `ws://127.0.0.1:4500`
and `PERSONAL_ADDR` to `ws://127.0.0.1:4501`. Override either variable consistently
on the server, Codex, and Phatmon commands to use different ports. `make live`
keeps other discovered homes visible and maintains independent connections: an
unavailable server shows an error for its home. Press **r** after starting it to
reconnect. `SESSION_DIR` defaults to this repository.

For a single shared server, use `make live-one` or
`make live-one SESSION_NAME=personal`. For another discovered home, set
`SESSION_NAME` to its name and choose a distinct `SERVER_ADDR` if running multiple
servers, using `make server`, `make codex`, and `make live-one` with matching values.
`SESSION_NAME` and `SERVER_ADDR` select a single server; `make live` always uses
`WORK_ADDR` and `PERSONAL_ADDR`. `SESSION_HOME` can
override the server/CLI home path; it must match that home's path in Phatmon's
configuration or discovery. `CODEX` overrides the CLI executable.

To resume saved history, exit its existing Codex terminal first, then run
`make codex CODEX_ARGS="resume SESSION_ID"` (with the same home/address overrides).
`CODEX_ARGS` passes additional Codex arguments; `ARGS` passes Phatmon arguments.
For example, `make live ARGS="--connect lab=ws://127.0.0.1:4502"` adds a third
discovered home's server. `make run` retains the normal
configuration-based startup behavior.

Select the session and press **a** to attach and receive live events. Sending a
message starts a turn when idle or steers the active turn using its ID. If the
session is running in an independent CLI, stop that CLI before resuming its history
here, or use a shared server. Phatmon does not inject keystrokes into other terminals.

Exiting stops Phatmon-owned stdio servers and their work. Shared WebSocket servers
keep running. Existing Codex sandbox and approval policies are preserved. Command
and file approvals require a decision; user-input questions get a form. Unsupported
server requests can be explicitly rejected in Requests.

## What the metrics mean

- **Status** comes from the connected server: working, waiting for approval/input,
  idle, error, or stored/runtime unknown. Idle does not mean the overall task is
  complete. A received live plan contributes the current step to the dashboard and
  is shown in full in the Plan view. No plan is invented when one is unavailable.
  Only top-level sessions appear in lists and dashboard counts. Any agent with a
  parent is excluded, including request, review, and other child agents. Subagent
  source metadata also identifies children when an older server omits the parent
  field. Top-level sessions about reviewing code remain visible.
- **Context** is the last reported request's total tokens divided by the model's
  context window. This approximates current context, especially around compaction.
  Cumulative input/output/reasoning tokens and cached-input ratio are separate.
- **Historical tokens** use a bounded, read-only JSONL fallback until a live token
  event arrives. The detail view identifies the source and timestamp. Unchanged
  rollouts are cached; unavailable values remain unknown.
- **Quota** is account-level, displayed per home. The dashboard and session details
  show only the Codex bucket's five-hour (300-minute) and weekly (10,080-minute)
  windows when reported. Other products, durations, and windows without a reported
  duration are omitted. Homes logged
  into the same account can share allowance. Accounts without either supported window
  show unavailable; failed refreshes retain the last values with a stale/error label.
- **Git** reports branch, ahead/behind, staged, modified, untracked, and conflicted
  files. Renames and filenames with spaces are supported. Phatmon does not fetch,
  stage, commit, or change repositories.
- **Refresh** defaults to every 5 seconds, with quotas every 60 seconds; both
  intervals are configurable. Stored-thread pagination is capped at 2,000 per home
  with a notice; loaded sessions are merged into the listing. Archived sessions are excluded. Details show the latest 20
  turns on servers supporting pagination. Older servers use full-history fallback.
  Individual item output and live text are bounded in the display.

## Configuration management

Phatmon creates `~/.config/phatmon/config.json` on first startup (or
`$XDG_CONFIG_HOME/phatmon/config.json` when set on Linux). It stores both homes and
application settings:

```json
{
  "version": 1,
  "homes": [],
  "settings": {
    "homes_directory": "~/.codex-homes",
    "codex_binary": "codex",
    "refresh_interval_seconds": 5,
    "quota_refresh_interval_seconds": 60,
    "live_only": false
  }
}
```

The generated file starts with `"homes": []`; that list is for optional manual
homes and overrides. For example, this entry names the discovered `consumable`
home **work** and connects it to an existing local server:

```json
{
  "name": "work",
  "path": "~/.codex-homes/consumable",
  "endpoint": "ws://127.0.0.1:4500"
}
```

Omit `endpoint` to let Phatmon start an owned stdio server for that home.

Startup merges configured homes with
`~/.codex`, a distinct `CODEX_HOME` environment value, and directories under
`settings.homes_directory` (default `~/.codex-homes`). The immediate child directory
is the Codex home itself; discovery does not recurse or append `.codex-home`.

Configured names and endpoints take precedence for the same resolved path.
Directory symlinks are supported and deduplicated; regular files and broken symlinks
are ignored. Name collisions receive deterministic numeric suffixes. A missing
discovery directory is fine and does not get created. Discovery reads directory
metadata, not credentials.

Discovered homes are kept in memory and rescanned on every startup, so adding or
removing directories takes effect at the next launch. Adding a manual home in the
TUI saves only manual entries, preserving discovery and existing settings. Forgetting
a discovered home disconnects it for the current run; it returns on restart.

Edit the file and restart to apply settings. Intervals accept 1–86,400 seconds.
Missing settings use defaults; malformed files or invalid settings produce an error
without overwriting your file. Existing `homes.json` registries are copied into the
new configuration on first startup, retaining the old file.

Use `--config PATH` for another configuration file; missing files are also created.
`--init-config` creates the file if needed, prints its path, and exits without
starting Codex or the TUI:

```sh
./bin/phatmon --init-config
```

`--codex` overrides the configured executable for one run. Repeated `--home`
arguments replace the entire merged home list and bypass discovery for one run;
`--connect` overrides endpoints.
These overrides are not automatically persisted. Adding or forgetting a home in the
TUI saves the manual home list while preserving the file's settings. Configuration
writes are atomic and owner-only; forgetting a home never deletes its directory.

**Skills** are discovered for the selected session directory through Codex. You can
view and enable/disable them. Installing remote skills is outside this initial version.

**MCP management** edits the selected home's user-level MCP table. Writes include an
optimistic version check, preserve unrelated settings and other servers, and request
a runtime reload. Concurrent edits produce a conflict instead of being overwritten.
Project, plugin, and managed MCP entries are outside this editor. Environment values
and auth headers are never rendered in the MCP list. Use Codex for OAuth login and
advanced server settings for now.

Phatmon never parses `auth.json`. Shared endpoints must be loopback WebSockets and
report the registered `CODEX_HOME` before controls become available. The Codex server
handles account authentication. The integration packages are separate from the TUI,
so a future webview can reuse them.

## Command-line options

| Option | Purpose |
| --- | --- |
| `--config PATH` | Use another configuration file; create it if missing |
| `--home NAME=PATH` | Set an explicit home list; repeat for multiple homes |
| `--connect NAME=ws://127.0.0.1:PORT` | Connect a known home to a shared local server |
| `--codex PATH` | Override the configured Codex executable |
| `--list-homes` | Print resolved homes and exit without starting Codex |
| `--init-config` | Create configuration if needed, print its path, and exit |
| `--demo` | Open the TUI with synthetic data and disabled controls |
| `--version` | Print the version |
| `--help` | List available options |

## Development and validation

The Go module is currently local (`module phatmon`); use the clone-and-build steps
above to install from source.

```sh
make init               # Create .venv and download missing dependencies
make test               # All tests with the race detector
make vet                # Static checks
make fmt                # Format source
make check              # Formatting check + vet + race tests
make build              # Build bin/phatmon
make run                # Build and run
make demo               # Build and run synthetic sessions
make list-homes         # Inspect discovery without starting Codex
make init-config        # Create configuration if missing
make clean              # Remove bin/; retain .venv caches
make help               # Show all targets

# Pass application flags or test filters.
make run ARGS='--home work=~/.codex-homes/consumable'
make test TEST_ARGS='-run TestDiscovery -count=1'

# Optional installed-CLI smoke test: temporary home, configuration writes,
# and an empty session. No model prompt is sent.
make test-integration
```

The race detector requires a supported platform with CGO enabled and a C compiler
(for example, GCC or Clang). `GO=/path/to/go` and `GOFMT=/path/to/gofmt` can select
specific installed tools. To reuse a populated `.venv/` completely offline, run
`GOPROXY=off make check build`.

Tests cover home discovery, configuration overrides, symlink deduplication, home
isolation, RPC routing/events/timeouts, pagination, start-versus-steer,
MCP version conflicts, Git parsing against a real temporary repository, token fallback,
terminal rendering, filtering, drill-down, and control-character sanitization. Tests
use temporary homes and a local fake WebSocket server. They never change real Codex
configuration or send prompts to live accounts.

Protocol references: [Codex app server](https://learn.chatgpt.com/docs/app-server)
and [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).
App-server methods and the rollout format can change between CLI releases; unsupported
features surface errors rather than fabricated telemetry.

## Contributing

Report bugs and feature requests in
[GitHub Issues](https://github.com/SuprPhatAnon/phatmon/issues). Include your Phatmon,
Go, and Codex versions, the operating system, and whether the affected connection
uses owned stdio or a shared WebSocket. Remove credentials and private conversation
content from any logs or configuration excerpts you share.

For code changes, run `gofmt` on modified Go files and `make check` before opening a
pull request. Keep `go.mod` and `go.sum` in version control. Local builds, environment
files, and machine-specific Phatmon configuration are excluded by `.gitignore`.
