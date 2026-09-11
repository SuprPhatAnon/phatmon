package tui

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent        = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8C0"))
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("#8995A8"))
	warning       = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2C66D"))
	danger        = lipgloss.NewStyle().Foreground(lipgloss.Color("#F48B9B"))
	bold          = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#263A46")).Foreground(lipgloss.Color("#FFFFFF"))
)

func safe(text string) string {
	text = ansi.Strip(text)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
}
func one(text string) string { return strings.Join(strings.Fields(safe(text)), " ") }
func fit(text string, width int) string {
	text = ansi.Truncate(one(text), max(0, width), "…")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}
func directoryName(cwd string) string {
	if cwd == "" {
		return "—"
	}
	return filepath.Base(filepath.Clean(cwd))
}
func wrap(text string, width int) string { return ansi.Hardwrap(safe(text), max(10, width), true) }
func meter(percent float64, width int) string {
	percent = math.Max(0, math.Min(100, percent))
	n := int(math.Round(percent * float64(width) / 100))
	bar := strings.Repeat("━", n) + strings.Repeat("╌", width-n)
	if percent >= 90 {
		return danger.Render(bar)
	}
	if percent >= 70 {
		return warning.Render(bar)
	}
	return accent.Render(bar)
}
func compact(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
func age(timestamp int64) string {
	if timestamp == 0 {
		return "—"
	}
	d := max(time.Duration(0), time.Since(time.Unix(timestamp, 0)))
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
func (m *Model) View() string {
	if m.width < 65 || m.height < 18 {
		return "Phatmon needs a terminal of at least 65 columns × 18 rows.\nResize the terminal, or press Ctrl+C to exit.\n"
	}
	title := accent.Bold(true).Render(" PHATMON ") + muted.Render(" / local Codex control")
	if m.demo {
		title += "  " + warning.Render("DEMO · SYNTHETIC DATA")
	}
	home := "all homes"
	if m.homeFilter > 0 {
		home = m.order[m.homeFilter-1]
	}
	title += "\n " + bold.Render(one(home)) + muted.Render("   h switch home   / filter   r refresh")
	var content, help string
	if m.selected == nil {
		content = m.dashboard()
		help = "↑↓ select  enter inspect  l live/all  a add home  x forget filtered home  n new  q quit"
	} else {
		content = m.detailView()
		help = "1–7/tab views  ↑↓/PgUp/PgDn scroll  a attach  m message  i interrupt  esc dashboard"
		if m.tab == 1 {
			help = "Tab pane  ↑↓ select/scroll  PgUp/Dn scroll  f live  s swap  1–7 views  Esc back"
		}
		if m.tab == 4 {
			help = "↑↓ select skill  enter view file  t enable/disable  tab next view  esc dashboard"
		}
		if m.tab == 5 {
			help = "↑↓ select server  + add  t enable/disable  d remove  r reload  esc dashboard"
		}
		if m.tab == 6 {
			help = "↑↓ select request  enter respond/approve  d decline  tab next view  esc dashboard"
		}
	}
	if m.form != nil {
		content = m.formView()
		help = "tab next field  enter continue/save  esc cancel"
		if m.form.Confirm {
			help = "y confirm   n / esc cancel"
		}
	}
	if m.composing {
		content = m.detailHeader() + "\n\n" + m.composer.View()
		help = "ctrl+s send to selected session   esc keep draft"
	}
	available := max(1, m.height-6)
	lines := strings.Split(content, "\n")
	if len(lines) > available {
		lines = lines[:available]
	}
	for len(lines) < available {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	body := strings.Join(lines, "\n")
	notice := m.notice
	if notice == "" {
		notice = "No account prompts are sent until you send a message. Quotas are per account; homes may share allowance."
	}
	return title + "\n" + body + "\n" + warning.Render(fit(notice, m.width)) + "\n" + muted.Render(fit(help, m.width))
}
func (m *Model) dashboard() string {
	working, waiting, live, total := 0, 0, 0, 0
	for _, h := range m.homes {
		for _, t := range h.Threads {
			if t.IsChildAgent() {
				continue
			}
			total++
			if !m.demo && (h.Client == nil || !h.Client.Connected() || h.Error != "") {
				continue
			}
			if t.Status.Type == "active" {
				if t.Status.Label() == "WORKING" {
					working++
				} else {
					waiting++
				}
			}
			if t.Status.Type == "active" || t.Status.Type == "idle" {
				live++
			}
		}
	}
	result := fmt.Sprintf("\n %s sessions   %s working   %s waiting   %s live\n", bold.Render(fmt.Sprint(total)), accent.Render(fmt.Sprint(working)), warning.Render(fmt.Sprint(waiting)), bold.Render(fmt.Sprint(live)))
	quotaLines := []string{}
	for _, name := range m.order {
		if m.homeFilter > 0 && m.order[m.homeFilter-1] != name {
			continue
		}
		h := m.homes[name]
		line := " " + bold.Render(one(name)) + "  "
		if h.QuotaError != "" {
			line += warning.Render("STALE ")
		}
		if h.Error != "" {
			line += danger.Render(one(h.Error))
		} else {
			labels := []string{}
			buckets := h.Quotas.Buckets()
			keys := []string{}
			for key := range buckets {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				b := buckets[key]
				for _, w := range b.DisplayWindows() {
					if w != nil {
						labels = append(labels, fmt.Sprintf("%s %s %s %.0f%% used · %s", one(key), w.Label(), meter(w.UsedPercent, 10), w.UsedPercent, w.Reset()))
					}
				}
			}
			if len(labels) == 0 {
				line += muted.Render("5h / weekly quotas unavailable")
			} else {
				line += strings.Join(labels, "  │  ")
			}
			if h.QuotaError != "" {
				line += "  " + warning.Render("STALE / "+one(h.QuotaError))
			}
		}
		if h.Truncated {
			line += " · first 2,000 sessions"
		}
		quotaLines = append(quotaLines, ansi.Truncate(line, m.width, "…"))
	}
	maxHomeLines := max(1, (m.height-12)/3)
	if len(quotaLines) > maxHomeLines {
		quotaLines = append(quotaLines[:maxHomeLines], muted.Render(" More homes: press h to filter and inspect each home's quota"))
	}
	result += strings.Join(quotaLines, "\n") + "\n\n"
	filter := "top-level sessions · saved + live"
	if m.liveOnly {
		filter = "top-level sessions · live on connected servers"
	}
	if m.filter != "" {
		filter += " · filter: " + one(m.filter)
	}
	result += muted.Render(" "+filter) + "\n"
	wide := m.width >= 110
	directoryWidth := min(22, max(12, m.width/7))
	taskWidth := m.width - 1 - 12 - directoryWidth - 21
	if wide {
		taskWidth -= 9 + 22 + 4
	}
	header := " " + fit("HOME", 12) + fit("DIRECTORY", directoryWidth) + fit("TASK / CURRENT STEP", taskWidth) + fit("STATUS", 21)
	if wide {
		header += fit("CONTEXT", 9) + fit("GIT", 22) + fit("AGE", 4)
	}
	result += muted.Render(header) + "\n"
	count := max(1, m.height-10-len(quotaLines))
	start := max(0, m.cursor-count+1)
	end := min(len(m.rows), start+count)
	if len(m.rows) == 0 {
		result += "\n No matching sessions.\n"
		if len(m.order) == 0 {
			result += " Press a to register an existing Codex home."
		} else {
			result += " Waiting for connections, or press n to create a session.\n Press l to toggle live-only, Esc to clear filters."
		}
	}
	for i := start; i < end; i++ {
		r := m.rows[i]
		h := m.homes[r.Home]
		task := r.Thread.Title()
		if plan, ok := h.Plans[r.Thread.ID]; ok {
			for _, step := range plan.Steps {
				if step.Status == "inProgress" {
					task = r.Thread.Title() + " · " + step.Step
					break
				}
			}
		}
		status := r.Thread.Status.Label()
		if !m.demo && (h.Client == nil || !h.Client.Connected() || h.Error != "") {
			status = "DISCONNECTED · STALE"
		}
		prefix := " "
		if i == m.cursor {
			prefix = "›"
		}
		line := prefix + fit(r.Home, 12) + fit(directoryName(r.Thread.Cwd), directoryWidth-1) + " " + fit(task, taskWidth) + fit(status, 21)
		if wide {
			context := "—"
			if p, ok := h.Usage[r.Thread.ID].ContextPercent(); ok {
				context = fmt.Sprintf("%.0f%%", p)
			}
			g := m.git[r.Thread.Cwd]
			git := g.Branch + " " + g.Summary()
			line += fit(context, 9) + fit(git, 22) + fit(age(r.Thread.UpdatedAt), 4)
		}
		if i == m.cursor {
			line = selectedStyle.Render(ansi.Truncate(line, m.width, "…") + strings.Repeat(" ", max(0, m.width-ansi.StringWidth(line))))
		}
		result += line + "\n"
	}
	return strings.TrimSuffix(result, "\n")
}
func (m *Model) detailHeader() string {
	if m.selected == nil {
		return ""
	}
	r := m.selected
	h := m.homes[r.Home]
	status := m.thread.Status.Label()
	if !m.demo && (h.Client == nil || !h.Client.Connected() || h.Error != "") {
		status = "DISCONNECTED · STALE"
	}
	attached := "observing"
	if h.Attached[r.Thread.ID] {
		attached = "attached"
	}
	rightWidth := min(78, max(38, m.width*3/5))
	leftWidth := max(1, m.width-rightWidth-3)
	title := strings.Split(ansi.Wrap(one(m.thread.Title()), leftWidth, ""), "\n")
	left := []string{"", "", one(r.Home) + " · " + attached, status}
	for i := 0; i < min(2, len(title)); i++ {
		left[i] = title[i]
	}
	if len(title) > 2 {
		left[1] = ansi.Truncate(left[1], max(1, leftWidth-1), "") + "…"
	}
	right := m.detailInfo(rightWidth)
	lines := make([]string, 4)
	for i := range lines {
		label := fit(left[i], leftWidth)
		if i < 2 {
			label = bold.Render(label)
		} else {
			label = accent.Render(label)
		}
		lines[i] = label + muted.Render(" │ ") + right[i]
	}
	return strings.Join(lines, "\n")
}

// Keep the header four rows tall so metrics do not take space from the panes.
func (m *Model) detailInfo(width int) []string {
	h := m.homes[m.selected.Home]
	quota := "Quota left"
	if h.QuotaError != "" || h.Error != "" {
		quota += " (stale)"
	}
	windows := []string{}
	for _, bucket := range h.Quotas.Buckets() {
		for _, window := range bucket.DisplayWindows() {
			windows = append(windows, fmt.Sprintf("%s %.0f%%", window.Label(), math.Max(0, math.Min(100, 100-window.UsedPercent))))
		}
	}
	if len(windows) == 0 {
		windows = append(windows, "unavailable")
	}
	quota += " · " + strings.Join(windows, " · ")
	// On small terminals shorten the stale label to keep both quota windows visible.
	if ansi.StringWidth(quota) > width {
		quota = strings.Replace(quota, "Quota left (stale)", "Stale quota", 1)
	}
	context := "Ctx —"
	usage := h.Usage[m.selected.Thread.ID]
	if percent, ok := usage.ContextPercent(); ok {
		context = fmt.Sprintf("Ctx %.0f%%", percent)
		if width >= 60 {
			context += fmt.Sprintf(" (%s/%s)", compact(usage.Last.TotalTokens), compact(*usage.ModelContextWindow))
		}
	}
	model := one(m.thread.Model)
	if model == "" {
		model = "unknown"
	}
	modelWidth := max(1, width-6-3-ansi.StringWidth(context))
	modelLine := "Model " + strings.TrimRight(fit(model, modelWidth), " ") + " · " + context
	directory := one(m.thread.Cwd)
	if directory == "" {
		directory = "unknown"
	}
	// Preserve the project name at the end of a long path.
	if ansi.StringWidth(directory) > width-4 {
		directory = ansi.TruncateLeft(directory, ansi.StringWidth(directory)-(width-4)+1, "…")
	}
	git := "Git unavailable"
	if g, ok := m.git[m.thread.Cwd]; ok {
		if g.Error != "" {
			git = "Git · " + g.Error
		} else {
			branch := one(g.Branch)
			if branch == "" {
				branch = "—"
			}
			state := g.Summary()
			if width < 60 {
				parts := []string{}
				for _, entry := range []struct {
					n     int
					label string
				}{{g.Staged, "S"}, {g.Modified, "M"}, {g.Untracked, "?"}, {g.Conflicts, "!"}} {
					if entry.n > 0 {
						parts = append(parts, fmt.Sprintf("%s%d", entry.label, entry.n))
					}
				}
				if len(parts) > 0 {
					state = strings.Join(parts, " ")
				}
			}
			changes := fmt.Sprintf("↑%d ↓%d · %s", g.Ahead, g.Behind, state)
			branchWidth := max(1, width-4-3-ansi.StringWidth(changes))
			git = "Git " + strings.TrimRight(fit(branch, branchWidth), " ") + " · " + changes
		}
	}
	lines := []string{quota, modelLine, "Dir " + directory, git}
	for i := range lines {
		lines[i] = muted.Render(fit(lines[i], width))
	}
	if h.QuotaError != "" || h.Error != "" {
		lines[0] = warning.Render(fit(quota, width))
	}
	return lines
}

func (m *Model) detailView() string {
	labels := []string{"Overview", "Responses", "Plan", "Git", "Skills", "MCP", "Requests"}
	tabs := []string{}
	for i, label := range labels {
		title := fmt.Sprintf("%d %s", i+1, label)
		if i == m.tab {
			title = accent.Bold(true).Render(title)
		} else {
			title = muted.Render(title)
		}
		tabs = append(tabs, title)
	}
	body := m.viewport.View()
	if m.tab == 1 {
		body = m.responsesView()
	}
	return m.detailHeader() + "\n\n " + strings.Join(tabs, "  ") + "\n\n" + body
}
func (m *Model) updateViewport() {
	if m.selected == nil {
		return
	}
	m.updateResponses()
	if m.tab == 1 {
		return
	}
	r := m.selected
	h := m.homes[r.Home]
	text := ""
	switch m.tab {
	case 0:
		text = fmt.Sprintf("SESSION\n%s\n\nMODEL\n%s · %s · reasoning %s\n\nTASK STATUS\n%s\n", m.thread.ID, m.thread.Model, m.thread.ModelProvider, m.thread.ReasoningEffort, m.thread.Status.Label())
		if m.thread.Status.Type == "notLoaded" || m.thread.Status.Type == "" {
			text += "Stored history. Runtime in other CLI processes is unknown.\n"
		}
		if m.thread.ParentThreadID != "" {
			text += "Parent session: " + m.thread.ParentThreadID + "\n"
		}
		if len(m.thread.Turns) > 0 {
			t := m.thread.Turns[len(m.thread.Turns)-1]
			text += "Last turn: " + t.Status
			if t.DurationMS != nil {
				text += fmt.Sprintf(" · %.1fs", float64(*t.DurationMS)/1000)
			}
			text += "\n"
		}
		text += "\nCONTEXT & TOKENS\n"
		u := h.Usage[r.Thread.ID]
		if u == nil {
			text += "No token usage reported yet. Attach for live updates.\n"
		} else {
			if p, ok := u.ContextPercent(); ok {
				text += fmt.Sprintf("Last request: %s / %s tokens (%.1f%% of context window)\n", compact(u.Last.TotalTokens), compact(*u.ModelContextWindow), p)
			} else {
				text += "Context window unavailable\n"
			}
			text += fmt.Sprintf("Cumulative: %s input · %s output · %s reasoning · %s total\n", compact(u.Total.InputTokens), compact(u.Total.OutputTokens), compact(u.Total.ReasoningOutputTokens), compact(u.Total.TotalTokens))
			if u.Total.InputTokens > 0 {
				text += fmt.Sprintf("Cached input: %.1f%%\n", 100*float64(u.Total.CachedInputTokens)/float64(u.Total.InputTokens))
			}
			text += "Source: " + u.Source + " · " + u.ObservedAt + "\nLast request approximates current context; compaction can change it.\n"
		}
		g := m.git[m.thread.Cwd]
		text += "\nGIT\n" + g.Branch + " · " + g.Summary() + fmt.Sprintf(" · ↑%d ↓%d\n", g.Ahead, g.Behind)
		text += "\nHOME\n" + h.Home.Path + "\n"
		if h.Home.Endpoint != "" {
			text += "Shared server: " + h.Home.Endpoint
		} else {
			text += "Phatmon-owned stdio server"
		}
		if h.Error != "" {
			text += "\nConnection error: " + h.Error
		}
		text += "\n\nQUOTA\n" + quotaDetail(h)
	case 2:
		plan, ok := h.Plans[r.Thread.ID]
		if !ok {
			text = "No live plan received.\n\nAttach to receive plan updates. A session's title is its task summary;\nidle means no running turn, not necessarily a completed task."
		} else {
			text = plan.Explanation + "\n\n"
			for _, step := range plan.Steps {
				text += fmt.Sprintf("%-12s %s\n", step.Status, step.Step)
			}
		}
	case 3:
		g := m.git[m.thread.Cwd]
		text = m.thread.Cwd + "\n\n" + g.Branch + fmt.Sprintf(" · ahead %d · behind %d\n", g.Ahead, g.Behind) + g.Summary() + "\n\n" + strings.Join(g.Files, "\n")
	case 4:
		text = "Skills resolved for " + m.thread.Cwd + "\nEnter views SKILL.md · t toggles enabled state\n\n"
		for i, s := range m.skills {
			prefix := "  "
			if i == m.inventoryCursor {
				prefix = "› "
			}
			enabled := "enabled"
			if !s.Enabled {
				enabled = "disabled"
			}
			text += fmt.Sprintf("%s%s · %s · %s\n   %s\n", prefix, s.Name, enabled, s.Scope, s.Description)
		}
		if len(m.skills) == 0 {
			text += "No skills loaded. Press r to retry.\n"
		}
		text += "\n" + m.inventoryError
	case 5:
		text = "User-level MCP servers for " + r.Home + "\n+ adds · t toggles · d removes · changes request runtime reload\nProject, plugin, and managed entries are outside this editor.\n\n"
		for i, name := range m.mcpNames() {
			server := m.mcp.Servers[name]
			prefix := "  "
			if i == m.inventoryCursor {
				prefix = "› "
			}
			enabled := "enabled"
			if value, ok := server["enabled"].(bool); ok && !value {
				enabled = "disabled"
			}
			target, _ := server["command"].(string)
			if target == "" {
				target, _ = server["url"].(string)
			}
			// Never render environment values or authentication headers.
			text += fmt.Sprintf("%s%s · %s\n   %s\n", prefix, name, enabled, target)
		}
		if len(m.mcp.Servers) == 0 {
			text += "No user MCP servers loaded.\n"
		}
		text += "\n" + m.inventoryError
	case 6:
		text = "Pending requests for this session / home\nEnter responds · d declines command/file approvals\n\n"
		requests := m.requests()
		if len(requests) == 0 {
			text += "Nothing needs attention. Attach to receive requests."
		}
		for i, request := range requests {
			prefix := "  "
			if i == m.inventoryCursor {
				prefix = "› "
			}
			text += prefix + request.Method + "\n"
			if i == m.inventoryCursor {
				text += pretty(request.Params) + "\n"
			}
		}
	}
	if m.tab == 4 && m.skillContent != "" {
		text = m.skillContent
	}
	bottom := m.viewport.AtBottom()
	offset := m.viewport.YOffset
	m.viewport.SetContent(wrap(text, m.viewport.Width))
	if bottom && m.tab == 1 {
		m.viewport.GotoBottom()
	} else {
		m.viewport.SetYOffset(offset)
	}
	if m.tab >= 4 && m.skillContent == "" {
		for i, line := range strings.Split(wrap(text, m.viewport.Width), "\n") {
			if strings.HasPrefix(line, "› ") {
				if i < m.viewport.YOffset {
					m.viewport.SetYOffset(i)
				} else if i >= m.viewport.YOffset+m.viewport.Height {
					m.viewport.SetYOffset(i - m.viewport.Height + 3)
				}
				break
			}
		}
	}
}
func quotaDetail(h *homeState) string {
	var text string
	buckets := h.Quotas.Buckets()
	keys := []string{}
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b := buckets[key]
		for _, w := range b.DisplayWindows() {
			if w != nil {
				text += fmt.Sprintf("%s · %s: %.1f%% used, %.1f%% remaining · %s\n", key, w.Label(), w.UsedPercent, math.Max(0, 100-w.UsedPercent), w.Reset())
			}
		}
	}
	if text == "" {
		text = "No five-hour or weekly quota reported for this account.\n"
	}
	if h.QuotaError != "" {
		text += "STALE / refresh error: " + h.QuotaError + "\n"
	}
	if !h.QuotaChecked.IsZero() {
		text += "Last attempt: " + h.QuotaChecked.Local().Format("15:04:05") + "\n"
	}
	return text
}
func (m *Model) formView() string {
	f := m.form
	title := wrap(f.Title, m.width-6)
	if f.Confirm {
		// Keep the decision controls visible; details can be inspected in Requests before confirming.
		lines := strings.Split(title, "\n")
		if len(lines) > m.height-12 {
			lines = append(lines[:max(1, m.height-13)], "… See Requests for full details.")
		}
		return "\n " + warning.Bold(true).Render("CONFIRM") + "\n\n" + strings.Join(lines, "\n") + "\n\n Press y to confirm, n to cancel."
	}
	text := "\n " + accent.Bold(true).Render(one(title)) + "\n\n"
	// Show the focused field so long question sets remain usable on small terminals.
	for i, label := range f.Labels {
		if i != f.Index {
			continue
		}
		text += fmt.Sprintf("Field %d of %d\n\n%s\n\n%s\n", i+1, len(f.Labels), wrap(label, m.width-8), f.Inputs[i].View())
	}
	return text
}
