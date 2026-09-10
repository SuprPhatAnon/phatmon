# Installation

[Documentation index](README.md)

## Requirements

- Go 1.24 or newer, Make, and Git.
- Codex on `PATH` for real sessions; demo mode does not require Codex.
- Linux with a running user systemd manager for `make install`.
- A terminal of at least 65 columns by 18 rows.
- A C compiler and CGO support to run the race tests.

Phatmon uses each home's existing Codex authentication. Before installing services,
create and authenticate the homes you want to use through Codex. The default home
is `~/.codex`; additional homes are immediate children of `~/.codex-homes/`.
The installer does not install Codex or sign you into an account.

## Build from source

```sh
git clone https://github.com/SuprPhatAnon/phatmon.git
cd phatmon
make build
make demo
```

`make build` initializes dependencies automatically. Make places Go dependencies,
downloaded toolchains, and build caches under `.venv/`. This is a Go cache directory;
there is no Python virtual environment to activate. `make clean` removes `bin/`
and retains these caches.

Inspect home discovery before connecting:

```sh
./bin/phatmon --list-homes
```

If you want to run without installing services, use the
[manual shared-server workflow](live-sessions.md#manual-shared-servers).

## Install user services

Stop any manually started app servers using the same ports, then run:

```sh
make install
~/bin/phatmon
```

The installer performs the following operations:

1. Copies the executable to `~/bin/phatmon`.
2. Resolves all configured and discovered Codex homes, including a distinct `CODEX_HOME`.
3. Writes one user systemd service per home under `~/.config/systemd/user/`.
4. Saves each matching endpoint in `~/.config/phatmon/config.json`.
5. Writes `phatmon.env` inside each home with `CODEX_HOME` and `CODEX_REMOTE` exports.
6. Reloads the user systemd manager and enables/starts the services.

Configuration and unit locations honor `XDG_CONFIG_HOME`. No sudo is needed. The
TUI itself stays in your terminal. The installer does not create a Codex launcher,
add shell startup entries, or edit project `.envrc` files. Set up your client using
the [environment and direnv instructions](live-sessions.md).

### Ports and names

| Home name | Default endpoint | Service |
| --- | --- | --- |
| `consumable` | `ws://127.0.0.1:4500` | `phatmon-codex-work.service` |
| `personal` | `ws://127.0.0.1:4501` | `phatmon-codex-personal.service` |
| Additional homes | Unassigned configured port starting at 4502 | `phatmon-codex-<name>-<path-hash>.service` |

Only resolved homes get services; a home named `consumable` is not required.
Additional homes retain saved endpoints on reinstall, and their assigned ports
are reserved before new ports are allocated. The installer prints the complete
home/endpoint/unit mapping. It does not probe ports occupied by unrelated programs.

Override the standard addresses consistently when using the Make shortcuts:

```sh
make install WORK_ADDR=ws://127.0.0.1:4600 PERSONAL_ADDR=ws://127.0.0.1:4601
```

## Installer options

Pass installer flags through `INSTALL_ARGS`. For example:

```sh
make install INSTALL_ARGS='--work-name work'
make install INSTALL_ARGS='--endpoint-env CODEX_APP_SERVER_URL'
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `--binary` | `bin/phatmon` | Executable to copy |
| `--codex` | `codex` | Original Codex executable; Make passes `CODEX` |
| `--bin-dir` | `~/bin` | Installed executable directory |
| `--unit-dir` | User config directory + `/systemd/user` | Unit destination |
| `--config` | User config directory + `/phatmon/config.json` | Phatmon configuration to update |
| `--work-name` | `consumable` | Home receiving the work address and standard work service name |
| `--work-addr` | `ws://127.0.0.1:4500` | Work listener; Make passes `WORK_ADDR` |
| `--personal-addr` | `ws://127.0.0.1:4501` | Personal listener; Make passes `PERSONAL_ADDR` |
| `--endpoint-env` | `CODEX_REMOTE` | Address variable exported by home environment files |
| `--no-start` | Off | Skip all systemctl calls |
| `--no-home-env` | Off | Skip per-home environment files and legacy client cleanup |

Listener addresses must use `ws://127.0.0.1:PORT` or `ws://[::1]:PORT`, without a
URL path. Each selected home must have a distinct port in the installation config.

To stage files without touching actual homes or starting services:

```sh
make install INSTALL_ARGS='--no-start --no-home-env --bin-dir /tmp/phatmon-stage/bin --unit-dir /tmp/phatmon-stage/units --config /tmp/phatmon-stage/config.json'
```

`--no-start` alone still updates home environment files. Use both staging flags
when you only want to inspect the generated files.

## Updates and service lifecycle

```sh
systemctl --user list-units 'phatmon-codex-*.service'
journalctl --user -u phatmon-codex-work.service -f
```

Rerun `make install` after updating Phatmon, adding homes, or changing the Codex
executable or its runtime `PATH`. Other Phatmon settings are preserved except
`codex_binary`, which records the original CLI's absolute path.

Already-running services are not restarted during installation. After changing a
service, wait until its work is finished, then restart it:

```sh
systemctl --user restart phatmon-codex-work.service
```

Restarting a service interrupts sessions hosted by that process. Shared servers
continue running when the Phatmon TUI exits. Enabled user services normally start
at login; the installer does not enable lingering after logout.

On upgrades from the earlier launcher implementation, installation removes only
Phatmon-marked launchers and its standard Bash/Zsh startup blocks. Its generated
legacy `shell.sh` becomes a harmless comment so custom startup files can still
source it. Unrelated executables and shell content are preserved.

## Removing an installation or home

There is no `make uninstall` target. First finish or stop work on the affected
server, then disable it. For example, to remove the work service:

```sh
systemctl --user disable --now phatmon-codex-work.service
rm "$HOME/.config/systemd/user/phatmon-codex-work.service"
systemctl --user daemon-reload
```

Use the actual unit directory if `XDG_CONFIG_HOME` or `--unit-dir` was set. Remove
that home's saved entry from Phatmon's config, or clear its endpoint to return to
an owned stdio server. A directory still under the discovery root will be found
again; [discovery](configuration.md#home-discovery) is independent of service removal.

For a full uninstall, repeat for all printed unit names, remove `~/bin/phatmon`,
and remove the Phatmon environment entries you added to your shell or direnv setup.
Codex homes contain account state and session history; removing Phatmon does not
require deleting them.
