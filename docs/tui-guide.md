# TUI guide

[Documentation index](README.md)

## Dashboard

The dashboard combines sessions from all resolved homes. Active sessions appear
first, followed by other sessions ordered by update time. The current selection
is retained across refreshes where possible.

Rows show the home, working-directory basename, task title, and runtime status.
At widths of 110 columns or more, additional context, Git, and age columns appear.
The full directory path and metrics remain available in session details at smaller
sizes. The terminal minimum is 65 columns by 18 rows.

Only top-level sessions appear. Threads with a parent or a subagent source are
excluded, including spawned review, request, and compaction agents. A top-level
session doing a review is still eligible. Archived sessions are not listed.

Press `/` to search by home, task, path, or status. Press `h` to cycle home filters
and `l` to toggle loaded sessions versus saved plus loaded sessions. Clearing a
filter can reveal sessions without changing their runtime state.

## Keys

Press `?` from the dashboard or any detail view for the complete, scrollable hotkey
reference. Scroll with Up/Down or PageUp/PageDown; close it with `?` or Esc to return
to your previous position. Inside a form or message, `?` remains normal text.

The drill-down footer highlights **m — new message** and **? — all hotkeys**.
In the composer, **Ctrl+S** sends and **Esc** keeps your draft.

These keys apply outside text-entry forms and the message composer.

| Location | Keys | Action |
| --- | --- | --- |
| Dashboard/details | `?` | Open the hotkey reference |
| Dashboard | Up/Down or `j`/`k` | Select a row |
| Dashboard | Enter | Open session details |
| Dashboard | `/`, `l` | Search; toggle live-only filter |
| Dashboard | `a` | Register an existing home |
| Dashboard | `x` | Forget the currently filtered home |
| Anywhere | `h` | Cycle home filters and return to the dashboard |
| Anywhere | `r` | Refresh; retry disconnected homes |
| Anywhere | `n` | Create a session in the selected home and directory |
| Details | `1`–`7` | Select a detail tab |
| Details | Left/Right | Cycle tabs |
| Responses | Tab/Shift+Tab or Enter | Switch pane focus |
| Responses | Up/Down or `j`/`k` | Select a response, or scroll tool output in the focused pane |
| Responses | PageUp/PageDown, Home/End | Scroll the focused pane |
| Responses | `f`, `s` | Follow the current response; swap pane positions |
| Other detail tabs | Tab/Shift+Tab | Cycle tabs |
| Details | Up/Down, PageUp/PageDown, Home/End | Scroll or select entries |
| Details | `a` | Attach to a loaded thread or resume stored history |
| Details | `m` | Open the message composer |
| Composer | Ctrl+S, Esc | Send; keep draft and close composer |
| Details | `i` | Interrupt the attached active turn |
| Skills | Enter, `t` | View skill file; toggle enabled state |
| MCP | `+`, `t`, `d` | Add, toggle, or remove a user-level server |
| Requests | Enter, `d` | Respond/approve; decline or reject |
| Dashboard/details | Esc | Return to dashboard, or clear dashboard search/live-only filter |
| Dashboard/details | `q` | Quit; confirm if an owned server has active work |

## Session information

The upper-right corner of session details shows the selected home's remaining
five-hour and weekly quota, the session model, context usage, directory, and Git
branch/status. Context is a percentage used; wider terminals also show tokens used
and the context-window size. Quota is a percentage remaining and is marked stale
when its refresh fails. Missing values stay unavailable.

At narrower widths, Git changes use `S` for staged, `M` for modified, `?` for
untracked, and `!` for conflicts. Arrows show commits ahead/behind. Long directory
paths keep the project name at the end. These four header rows stay visible while
either pane scrolls.

## Session tabs

| Tab | Contents |
| --- | --- |
| 1 — Overview | Session identity, full directory, runtime, tokens, quota, and Git summary |
| 2 — Responses (default) | Assistant responses and the tool output following the selected response, in two panes |
| 3 — Plan | The plan reported by the agent and current step |
| 4 — Git | Repository state for the session directory |
| 5 — Skills | Skills Codex discovers for this session directory |
| 6 — MCP | User-level MCP configuration for the selected home |
| 7 — Requests | Pending command/file approvals and user-input questions |

Opening details selects the latest assistant response and automatically attaches to
sessions already loaded on the connected server. The left pane lists only assistant
responses; the right pane shows tool activity after the selected response up to the
next assistant response, including across turn boundaries. User messages, reasoning,
and plan items are omitted from this view.

Live mode follows incoming responses and scrolls to the newest output. Selecting an
older response or scrolling either pane pauses following, preserving your position
while new events arrive. Press `f` to return to the current response and follow live
output. Tab switches focus; `s` swaps the panes without changing their scroll positions.
The focused pane has a highlighted border and a `›` in its heading.

Stored sessions stay history-only until you press **a** and confirm resuming them.
[Connect both clients to the same server](live-sessions.md) before
controlling a session that is also open in a Codex terminal.

## Messaging and requests

After attaching, press `m`, type the message, and send with Ctrl+S. For an idle
session this starts a turn. For an active session Phatmon steers the existing turn
using its current ID. If the active turn ID is unavailable, refresh details before
trying to steer or interrupt.

Approvals and questions appear in Requests. Phatmon preserves the Codex server's
sandbox and approval configuration. Unsupported request types can be explicitly
rejected. Cancelling a form does not send a response.

Creating or resuming a session on an owned stdio server puts its runtime under
Phatmon's process lifetime. Exiting Phatmon stops owned servers and their work.
Shared WebSocket servers continue independently.

## Understanding metrics

### Runtime and task state

| Label | Meaning |
| --- | --- |
| `WORKING` | The connected server reports an active turn |
| `NEEDS APPROVAL` | An active turn is waiting on an approval |
| `NEEDS INPUT` | An active turn is waiting on user input |
| `IDLE` | The loaded session has no active turn |
| `ERROR` | The server reports a session system error |
| `STORED · UNKNOWN` | Saved history is available, but this server does not report a loaded runtime |

Idle does not mean the overall task is finished. Task titles and agent-reported
plan steps provide additional context; Phatmon does not infer completion from them.

### Context and tokens

Context percentage uses the last model request's total tokens divided by its
reported model context window. It approximates current context, especially around
compaction. Cumulative input, output, reasoning, and cached-input metrics are
separate; cumulative usage is not the current context size.

Before a live token event, Phatmon may read a bounded tail of a saved rollout JSONL
file inside that home. The detail view identifies its source and timestamp.
Missing data stays unavailable; it is not presented as zero usage.

### Quotas

Only Codex-specific quota windows are displayed, and only when their reported
duration is five hours (300 minutes) or one week (10,080 minutes). Other products,
durations, and unknown-duration windows are omitted. Older single-bucket responses
are accepted when they identify Codex or provide no product identity.

Quota is account-level, displayed per home. Homes signed into the same account can
share allowance. Failed refreshes retain the previous values with a stale/error
label. An unavailable quota is not evidence that the allowance is exhausted.

### Git and refresh

Git data includes branch, ahead/behind, staged, modified, untracked, and conflicted
counts. Git checks are read-only; Phatmon does not fetch, stage, or commit.

Sessions and metrics refresh every five seconds by default; quotas refresh every
60 seconds. Both intervals are [configurable](configuration.md). Thread listing is
bounded to 2,000 stored entries with a truncation notice, plus loaded-thread merging.
Details read the latest 20 turns when supported and fall back to full history on
older servers. Displayed output is bounded.

## Skills and MCP management

Skill discovery is scoped to the selected session directory. Enter opens the skill
file; `t` enables or disables it through Codex. Installing remote skill packages is
outside the current TUI's scope.

MCP controls edit the selected home's base user `config.toml` layer. Add a command
with a JSON argument array, or an HTTP(S) URL. Arguments are passed as arguments,
not assembled into shell commands. Writes use the config version observed during
the read, preserve unrelated settings, and request a runtime reload. A concurrent
edit produces a conflict instead of overwriting newer configuration.

Project, profile, plugin, and managed MCP layers are outside this editor. Use Codex
for OAuth login and advanced server settings. Secret environment values and auth
headers are not rendered in the server list.
