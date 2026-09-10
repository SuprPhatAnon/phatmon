# Phatmon

A local **Go TUI** for monitoring and controlling Codex sessions across multiple
`CODEX_HOME` directories.

The dashboard shows top-level sessions with task status, working-directory names,
Git state, context usage, and Codex quotas. Open a session to inspect its
conversation and plan, send messages, respond to requests, or manage skills and
user-level MCP servers. Phatmon uses Bubble Tea, Bubbles, and Lip Gloss.

## Quick start

Requires Go 1.24+, Make, Git, and Codex for real sessions. The integration was
checked with Codex CLI 0.154.0. Use a terminal at least 65 columns by 18 rows.

```sh
git clone https://github.com/SuprPhatAnon/phatmon.git
cd phatmon
make build
make demo
```

Demo mode displays synthetic sessions without account access. To inspect your
homes and open the real dashboard:

```sh
./bin/phatmon --list-homes
./bin/phatmon
```

Phatmon creates `~/.config/phatmon/config.json` if it is missing. It discovers
`~/.codex` and immediate directories under `~/.codex-homes/`. A distinct
`CODEX_HOME` is also included. Make downloads missing dependencies into a
project-local `.venv/` Go cache.

## Install services and connect live sessions

On Linux with user systemd, initialize and authenticate your Codex homes first,
then install:

```sh
make install
~/bin/phatmon
```

Installation copies Phatmon to `~/bin`, creates a user app-server service per home,
saves matching endpoints in Phatmon's config, and writes a `phatmon.env` file in
each home. Personal defaults to port 4501, work (`consumable`) to 4500, and
additional homes receive separate saved ports starting at 4502.

In another terminal, source a home's environment file and start Codex explicitly
connected to its server:

```sh
source "$HOME/.codex/phatmon.env"
codex --remote "$CODEX_REMOTE" --cd "$PWD"
```

`CODEX_REMOTE` carries the address for your client setup to consume. Phatmon does
not install a Codex launcher or automatically convert the variable to client
arguments. Its own connections use the saved home endpoints. In the dashboard,
open the session with **Enter** and press **a** to attach.

For a one-time shell shortcut that lets you just run `codex` from any project,
see [the Zsh shortcut](docs/live-sessions.md#one-time-shortcut-for-zsh).

See [Live sessions and direnv](docs/live-sessions.md) for personal `.zshrc`
defaults, work `.envrc` overrides, and manual servers without systemd.
Stop manually running listeners on the same ports before installing services.

## Documentation

| Guide | Covers |
| --- | --- |
| [Documentation index](docs/README.md) | Suggested reading order and example files |
| [Installation](docs/installation.md) | Build, user services, installer flags, updates, and removal |
| [Live sessions and direnv](docs/live-sessions.md) | Multiple servers, environment variables, attachment, and resume |
| [Configuration](docs/configuration.md) | Home discovery, JSON settings, endpoints, and CLI overrides |
| [TUI guide](docs/tui-guide.md) | Keyboard controls, metrics, messaging, skills, and MCP |
| [Troubleshooting](docs/troubleshooting.md) | Missing sessions, connection failures, quotas, and setup errors |
| [Development](docs/development.md) | Architecture, Make targets, testing, and contribution workflow |

## Scope and limits

- Only top-level sessions appear; agents with parents and subagent sources are filtered out.
- Quotas show only Codex five-hour and weekly windows when reported.
- Stored history does not identify the live state of an independently hosted session.
- Shared WebSocket endpoints must be local and report the matching Codex home.
- Exiting Phatmon stops its owned stdio servers; shared services continue running.
- The current interface is a TUI. A webview and remote skill installation are not implemented.

## Development

```sh
make check build
make demo
```

Checks include formatting, vet, and race tests. The race detector requires a C
compiler. An opt-in `make test-integration` checks the installed Codex in a temporary
home without sending a model prompt. See the [development guide](docs/development.md)
for details.

Report problems in [GitHub Issues](https://github.com/SuprPhatAnon/phatmon/issues).
See [LICENSE](LICENSE) for licensing terms.
