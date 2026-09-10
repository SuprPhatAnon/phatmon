# Configuration reference

[Documentation index](README.md)

## File location and defaults

Phatmon creates `~/.config/phatmon/config.json` when it is missing. On Linux,
`XDG_CONFIG_HOME` changes the configuration root. `--config PATH` selects a different
file and also creates it if absent. Startup does not overwrite an existing file.

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

| Setting | Meaning |
| --- | --- |
| `version` | Configuration format version; currently `1` |
| `homes` | Explicit homes and overrides; an empty array still enables discovery |
| `settings.homes_directory` | Root whose immediate subdirectories are Codex homes |
| `settings.codex_binary` | Codex executable used for owned app servers |
| `settings.refresh_interval_seconds` | Session and metric polling interval |
| `settings.quota_refresh_interval_seconds` | Account quota polling interval |
| `settings.live_only` | Initial dashboard filter for loaded sessions |

Both intervals accept integers from 1 to 86,400 seconds. Missing settings receive
defaults. Unknown fields, malformed JSON, invalid versions, and invalid settings
produce an error; Phatmon does not replace the invalid file. Restart after editing
settings. Config writes are atomic and use owner-only permissions.

If `config.json` is absent and a legacy `homes.json` exists in the same directory,
its configuration is copied into the new file. The original is retained.

## Home discovery

Startup resolves homes from:

1. Explicit entries in `homes`.
2. `~/.codex`, normally named `personal`.
3. A distinct `CODEX_HOME` environment path, normally named `environment`.
4. Immediate directory children of `settings.homes_directory`, named by directory.

The child directory itself is the home: discovery neither recurses nor appends
`.codex-home`. Files and broken symlinks are ignored. Directory symlinks are resolved
and deduplicated. Explicit entries win when two paths resolve to the same directory.
Name collisions receive deterministic numeric suffixes such as `personal-2`.

A missing discovery root is allowed and is not created by discovery. Homes need to
exist before connecting; discovery of the default home does not authenticate or
initialize Codex. Directories are rescanned at startup, not continuously.

Inspect the effective list without opening a TUI or starting servers:

```sh
./bin/phatmon --list-homes
```

Discovered entries are normally kept in memory. Registering a home in the TUI saves
manual entries; forgetting a discovered home only hides it for the current run.
The installer persists resolved homes with their endpoints, making those entries
explicit. After installation, deleting a directory alone does not remove its saved
entry or disable its service.

## Home entries and transport

```json
{
  "name": "consumable",
  "path": "~/.codex-homes/consumable",
  "endpoint": "ws://127.0.0.1:4500"
}
```

| Field | Meaning |
| --- | --- |
| `name` | Display name and key used by `--connect` |
| `path` | Codex home; `~` and symlinks are resolved |
| `endpoint` | Optional local shared app-server WebSocket address |

Home names, resolved paths, and nonempty endpoints must be unique. A name must be
1–80 printable characters and must not contain `/`. Paths in JSON are not shell
expressions: use `~` or an actual path rather than `$HOME`.

With no endpoint, Phatmon starts an owned stdio app server for that home. With an
endpoint, it connects to the existing server and checks that the server reports the
same resolved Codex home. Endpoints must use `ws://` with a port on `127.0.0.1`,
`localhost`, or `::1`; credentials, query strings, and fragments are rejected.
The installer has the stricter listener requirements described in
[Installation](installation.md#installer-options).

The [two-home example](examples/config.json) can be adapted for your installation.
It is a Phatmon config, separate from each home's Codex `config.toml`.

## Command-line overrides

| Flag | Purpose |
| --- | --- |
| `--config PATH` | Use another Phatmon configuration file |
| `--home NAME=PATH` | Replace the effective home list; repeat for multiple homes |
| `--connect NAME=URL` | Override a known home's endpoint; repeat for multiple homes |
| `--codex PATH` | Override `settings.codex_binary` for this run |
| `--list-homes` | Print resolved homes and exit |
| `--init-config` | Create config if missing, print its path, and exit |
| `--demo` | Display synthetic data with controls disabled |
| `--version` | Print Phatmon version |
| `--help` | Show CLI help |

`--home` bypasses discovery for that invocation. `--connect` is applied afterward
and must name a home in the resulting list. These CLI overrides are not saved just
by launching Phatmon. Home-management actions in the TUI can save the resulting
manual home list.

```sh
./bin/phatmon \
  --home 'personal=~/.codex' \
  --home 'work=~/.codex-homes/consumable' \
  --connect personal=ws://127.0.0.1:4501 \
  --connect work=ws://127.0.0.1:4500
```

With the renamed `work` entry above, use `--connect work=...`, not
`--connect consumable=...`. Likewise, `make live` assumes the standard home names;
use normal config-based startup for installations with renamed homes.

## Environment files

`make install` writes `<home>/phatmon.env` with `CODEX_HOME` and an address variable
named `CODEX_REMOTE` by default. These files are shell snippets for your setup to
source. Phatmon itself uses the home `endpoint` field. See
[Live sessions and direnv](live-sessions.md) for shell startup, work overrides,
and the explicit `codex --remote "$CODEX_REMOTE"` command.
