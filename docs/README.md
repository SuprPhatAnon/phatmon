# Phatmon documentation

Phatmon monitors and controls local Codex sessions from a terminal. It can combine
multiple Codex homes and connect to a separate shared app server for each home.

| Guide | Use it to |
| --- | --- |
| [Installation](installation.md) | Build Phatmon, install user services, update or remove an installation |
| [Live sessions and direnv](live-sessions.md) | Connect Codex and Phatmon to the same servers; configure personal and work environments |
| [Configuration reference](configuration.md) | Configure discovery, endpoints, refresh intervals, and command-line overrides |
| [TUI guide](tui-guide.md) | Navigate sessions, interpret metrics, send messages, and manage skills or MCP servers |
| [Troubleshooting](troubleshooting.md) | Diagnose missing sessions, connection errors, quotas, and installation problems |
| [Development](development.md) | Understand the architecture, run checks, and contribute |

For a first installation, follow [Installation](installation.md), then
[Live sessions and direnv](live-sessions.md). To explore without connecting an
account, run `make demo` from the repository.

## Example files

- [config.json](examples/config.json): two homes connected to their shared servers.
- [personal.zsh](examples/personal.zsh): personal defaults for a shell startup file.
- [consumable.envrc](examples/consumable.envrc): project-specific work overrides for direnv.

Adapt paths and names to your machine. A Codex home stores Codex configuration and
account state; a session's working directory is the project the agent operates on.
For example, `~/.codex-homes/consumable` can be the home for sessions working in
`~/consumable` or any of its repositories.

The application is a Go TUI. A webview is not implemented. The integration was
checked with Codex CLI 0.154.0; app-server protocol changes may affect compatibility.

[Back to project README](../README.md)
