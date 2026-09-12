package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const hotkeys = `MESSAGING
m                 New message to the selected session
Ctrl+S            Send the message (in the composer)
Enter             New line (in the composer)
Esc               Close composer and keep the draft
a                 Attach/resume before messaging if needed
i                 Interrupt the attached session's active turn

RESPONSES — TWO PANES
Tab / Shift+Tab   Switch pane focus
Enter             Switch pane focus
Up/Down or k/j    Select a response, or scroll tool output
PgUp / PgDn       Scroll the focused pane by half a page
Home / End        Scroll the focused pane to top/bottom
f                 Follow the current response and live output
s                 Swap pane positions

SESSION VIEWS
1                 Overview
2                 Responses (default)
3                 Plan
4                 Git
5                 Skills
6                 MCP servers
7                 Pending requests
Left / Right      Previous/next view
Tab / Shift+Tab   Next/previous view outside Responses
Up/Down or k/j    Scroll, or select a skill/server/request
PgUp / PgDn       Scroll by half a page
Home / End        Scroll to top/bottom
Esc               Return to the dashboard

SKILLS / MCP / REQUESTS
Enter             View selected skill file or respond to request
t                 Toggle selected skill or MCP server
+ or =            Add an MCP server (MCP view)
d                 Remove MCP server, or decline/reject request

DASHBOARD
Up/Down or k/j    Select a session
Enter             Open the selected session
l                 Toggle live-only / all sessions
a                 Register a Codex home
x                 Forget the currently filtered home
Esc               Clear search and live-only filter

DASHBOARD AND SESSION VIEWS
?                 Open this hotkey reference
/                 Search sessions by task, home, path, or status
h                 Cycle home filters and return to dashboard
r                 Refresh / reconnect
n                 Create a new session in the selected home
q                 Quit (confirms if owned servers have active work)
Ctrl+C            Quit immediately, including from forms and help

FORMS AND CONFIRMATIONS
Tab / Down        Next field
Shift+Tab / Up    Previous field
Enter             Continue / save
Esc               Cancel
 y / n            Confirm / cancel a confirmation

HELP
Up/Down or k/j    Scroll this reference
PgUp / PgDn       Scroll by half a page
Home / End        Top / bottom
? / Esc / q       Close help and return to the same view

Typing ? in a form or message inserts a question mark.
Messaging requires attachment; demo controls are disabled.`

func (m *Model) resizeHelp() {
	m.helpViewport.Width = max(1, m.width-2)
	m.helpViewport.Height = max(1, m.height-8)
	m.helpViewport.SetContent(ansi.Wrap(hotkeys, m.helpViewport.Width, ""))
}

func (m *Model) helpKey(key tea.KeyMsg) {
	switch key.String() {
	case "?", "esc", "q":
		m.showHelp = false
	case "up", "k":
		m.helpViewport.ScrollUp(1)
	case "down", "j":
		m.helpViewport.ScrollDown(1)
	case "pgup":
		m.helpViewport.HalfPageUp()
	case "pgdown":
		m.helpViewport.HalfPageDown()
	case "home":
		m.helpViewport.GotoTop()
	case "end":
		m.helpViewport.GotoBottom()
	}
}
