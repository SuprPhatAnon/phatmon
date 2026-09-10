# Troubleshooting

[Documentation index](README.md)

## A session shows STORED · UNKNOWN

The connected server can read the history but does not report the session as loaded.
Check that Phatmon's home endpoint and the Codex client's `--remote` address refer
to the same server:

```sh
~/bin/phatmon --list-homes
printf 'Home: %s\nServer: %s\n' "$CODEX_HOME" "$CODEX_REMOTE"
```

If the session is running in an independent CLI, exit that CLI before resuming its
history through the shared server:

```sh
codex resume --remote "$CODEX_REMOTE" SESSION_ID
```

In Phatmon, refresh with `r`, open the session, then press `a` to attach. A TCP
connection alone does not prove that the specific session is loaded on that server.

## CODEX_REMOTE is set but the client is not live

Phatmon exports the variable for your client setup to consume; it does not install
a launcher that consumes it. Use `codex --remote "$CODEX_REMOTE" --cd "$PWD"` explicitly. The
dashboard separately uses endpoints stored in its config. See
[Live sessions and direnv](live-sessions.md).

If direnv reports that `.envrc` is blocked, review the changes and run
`direnv allow /path/to/project`. A changed environment file does not move an
already-running client to another server. Restart or resume the client through
the desired endpoint.

## Codex starts in the wrong directory

Pass your current directory explicitly when opening a session on the shared server:

```sh
codex --remote "$CODEX_REMOTE" --cd "$PWD"
```

The installed app-server service starts in your home directory. `--cd "$PWD"`
selects the project directory from the shell where you invoke the client.

## Connection refused or a home remains disconnected

Check the relevant user service and recent diagnostics:

```sh
systemctl --user status phatmon-codex-personal.service
journalctl --user -u phatmon-codex-personal.service -n 50 --no-pager
ss -ltn '( sport = :4500 or sport = :4501 )'
```

Use the unit printed by the installer for additional homes. Confirm that its home
directory exists, Codex is executable, and another process is not occupying its
port. After starting the service, press `r` in Phatmon to reconnect. One unavailable
home does not prevent other homes from connecting.

If services cannot find a runtime or Codex moved, reinstall with the correct
`CODEX=/path/to/codex` and `PATH`. Restart affected services when their work is done.

## Endpoint CODEX_HOME does not match

The server reports a different home than Phatmon registered for that endpoint.
Compare `--list-homes`, your service's `CODEX_HOME`, and the address being used.
Correct the mapping or start the service with the intended home. Swapping ports
between personal and work homes without updating both sides causes this error.

## A home or session is missing

- Use `--list-homes` to check discovery and explicit overrides. `--home` replaces
  the entire discovered list for that invocation.
- Restart after adding a directory under the discovery root; discovery runs at
  startup. Rerun installation to create a service for a new home.
- Clear the search/home filters and toggle `l` to show saved plus loaded sessions.
- Child agents and archived threads are deliberately excluded. A parent field
  excludes a thread regardless of its title or role.
- Very large histories may reach the listing cap; Phatmon shows a truncation notice.

A removed home can remain in installed configuration because installation saves
explicit endpoint mappings. Update the saved entry and manage its service
separately; deleting the directory alone does neither.

## Quota or context is unavailable

Phatmon only displays reported Codex five-hour and weekly windows. Some accounts
do not return these windows. Check the home's account through Codex and refresh
with `r`. An error/stale label describes a failed refresh, not an exhausted quota.

Context needs token usage and a reported model context window. Attach and wait for
a live token event. Historical rollout fallback may be unavailable or may not
reflect the latest request. Cumulative token totals are not current context.

## Cannot message, steer, or edit MCP

Press `a` before messaging. Refresh session details if the active turn ID is missing.
For an approval or question, respond in Requests. If a turn has already ended,
there is nothing left to interrupt.

MCP editing requires a base user configuration layer with a version. It does not
edit project or managed entries. On a version conflict, refresh before retrying.
If a write succeeds but runtime reload fails, the saved config remains changed;
resolve the reload error before assuming the active server is using the new value.

## Installation or startup errors

| Symptom | Action |
| --- | --- |
| User systemd manager unavailable | Run from a user login with a working user manager, or use the manual-server workflow |
| Home directory does not exist | Initialize the intended home through Codex, or correct/remove the configured path |
| Invalid config | Correct the reported JSON field; the original file is retained |
| Unknown home in `--connect` | Use the resolved name from `--list-homes`; manual naming overrides discovery |
| `make live` fails after renaming homes | Start with saved config using `~/bin/phatmon`, or pass your own `--connect` names |
| Terminal too small | Enlarge it to at least 65 by 18; use 110+ columns for extra dashboard metrics |
| Race tests cannot compile | Check CGO and your C compiler; see [Development](development.md) |

`--no-start` can stage files without systemd; also use `--no-home-env` and redirected
destinations to avoid changes to actual homes. Full commands are in
[Installation](installation.md#installer-options).

## Reporting a problem

Include Phatmon and Codex versions, operating system, transport type (owned stdio
or shared WebSocket), and the action that failed in a
[GitHub issue](https://github.com/SuprPhatAnon/phatmon/issues). For build failures,
also include the Go version. Remove credentials and private conversation content
from any excerpts you share.
