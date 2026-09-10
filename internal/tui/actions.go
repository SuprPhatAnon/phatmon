package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"phatmon/internal/codex"
	"phatmon/internal/config"
)

func (m *Model) key(key tea.KeyMsg) tea.Cmd {
	if key.String() == "tab" || key.String() == "shift+tab" || key.String() == "left" || key.String() == "right" || key.String() == "esc" || key.String() == "r" || len(key.String()) == 1 {
		m.skillContent = ""
	}
	switch key.String() {
	case "q":
		for _, h := range m.homes {
			if h.Home.Endpoint == "" && len(h.Active) > 0 && !m.demo {
				return m.confirm("quit", "Quit and stop Phatmon-owned servers and their active turns?", "")
			}
		}
		return tea.Quit
	case "esc":
		if m.selected != nil {
			m.selected = nil
			m.skills = nil
			m.stream = map[string]string{}
			m.notice = ""
		} else {
			m.filter = ""
			m.liveOnly = false
			m.rebuildRows()
		}
	case "h":
		m.homeFilter = (m.homeFilter + 1) % (len(m.order) + 1)
		m.selected = nil
		m.rebuildRows()
	case "l":
		if m.selected == nil {
			m.liveOnly = !m.liveOnly
			m.rebuildRows()
		}
	case "/":
		return m.openForm("filter", "Filter by task, home, path, or status", "", []string{"Search"}, []string{m.filter})
	case "r":
		if m.demo {
			m.notice = "Demo uses synthetic data; no account access"
			return nil
		}
		cmds := []tea.Cmd{}
		for _, name := range m.order {
			h := m.homes[name]
			if h.Client == nil || !h.Client.Connected() {
				if h.Client != nil {
					h.Client.Close()
				}
				cmds = append(cmds, m.connect(name))
			} else {
				cmds = append(cmds, m.poll(name), m.pollQuota(name))
			}
		}
		if m.selected != nil {
			cmds = append(cmds, m.readDetail())
			if m.tab == 4 || m.tab == 5 {
				cmds = append(cmds, m.inventory())
			}
		}
		return tea.Batch(cmds...)
	case "a":
		if m.selected == nil {
			return m.openForm("home", "Register a Codex home", "", []string{"Name", "Existing CODEX_HOME path", "Local WebSocket endpoint (blank = owned stdio)"}, []string{"", "", ""})
		}
		if m.demo {
			m.notice = "Demo controls are disabled"
			return nil
		}
		title := "Attach to this live session and receive its events?"
		if m.thread.Status.Type == "notLoaded" || m.thread.Status.Type == "" {
			title = "Resume stored history here? Stop any independent CLI using this session first; its runtime is unknown."
		}
		return m.confirm("attach", title, m.selected.key())
	case "x":
		if m.selected == nil && m.homeFilter > 0 {
			return m.confirm("forget", "Forget this home? Its files stay on disk. Discovered homes return on restart. Its Phatmon-owned server and active work will stop.", m.order[m.homeFilter-1])
		}
	case "n":
		if len(m.order) == 0 {
			m.notice = "Add a home first with a"
			return nil
		}
		home := m.order[0]
		if m.homeFilter > 0 {
			home = m.order[m.homeFilter-1]
		}
		if m.selected != nil {
			home = m.selected.Home
		}
		cwd, _ := os.Getwd()
		return m.openForm("new", "New session in "+home, home+"/", []string{"Working directory"}, []string{cwd})
	case "enter":
		if m.selected == nil && len(m.rows) > 0 {
			r := m.rows[m.cursor]
			m.selected = &r
			m.thread = r.Thread
			m.tab = 0
			m.inventoryCursor = 0
			m.composer.Reset()
			m.stream = map[string]string{}
			m.viewport.GotoTop()
			m.updateViewport()
			return m.readDetail()
		}
		if m.selected != nil && m.tab == 4 {
			return m.viewSkill()
		}
		if m.selected != nil && m.tab == 6 {
			return m.requestForm(false)
		}
	case "m":
		if m.selected != nil {
			h := m.homes[m.selected.Home]
			if !h.Attached[m.selected.Thread.ID] {
				m.notice = "Press a to attach / resume before messaging"
				return nil
			}
			m.composing = true
			return m.composer.Focus()
		}
	case "i":
		if m.selected != nil {
			return m.confirm("interrupt", "Interrupt this session's current turn?", m.selected.key())
		}
	case "t":
		if m.selected != nil && m.tab == 4 {
			return m.toggleSkill()
		}
		if m.selected != nil && m.tab == 5 {
			return m.changeMCP("toggle")
		}
	case "+", "=":
		if m.selected != nil && m.tab == 5 {
			return m.openForm("mcp-add", "Add a user-level MCP server", m.selected.key(), []string{"Name", "Command or http(s) URL", "Arguments (JSON array; blank for HTTP)"}, []string{"", "", "[]"})
		}
	case "d":
		if m.selected != nil && m.tab == 5 {
			return m.confirm("mcp-remove", "Remove selected MCP server from this home's user configuration?", m.selected.key())
		}
		if m.selected != nil && m.tab == 6 {
			return m.requestForm(true)
		}
	case "tab", "right":
		if m.selected != nil {
			m.tab = (m.tab + 1) % 7
			m.inventoryCursor = 0
			m.viewport.GotoTop()
			m.updateViewport()
			if m.tab == 4 || m.tab == 5 {
				return m.inventory()
			}
		}
	case "shift+tab", "left":
		if m.selected != nil {
			m.tab = (m.tab + 6) % 7
			m.inventoryCursor = 0
			m.viewport.GotoTop()
			m.updateViewport()
			if m.tab == 4 || m.tab == 5 {
				return m.inventory()
			}
		}
	case "1", "2", "3", "4", "5", "6", "7":
		if m.selected != nil {
			m.tab = int(key.String()[0] - '1')
			m.inventoryCursor = 0
			m.viewport.GotoTop()
			m.updateViewport()
			if m.tab == 4 || m.tab == 5 {
				return m.inventory()
			}
		}
	case "up", "k":
		if m.selected == nil {
			m.cursor = max(0, m.cursor-1)
		} else if m.tab >= 4 {
			m.inventoryCursor = max(0, m.inventoryCursor-1)
			m.updateViewport()
		} else {
			m.viewport.ScrollUp(1)
		}
	case "down", "j":
		if m.selected == nil {
			m.cursor = min(max(0, len(m.rows)-1), m.cursor+1)
		} else if m.tab >= 4 {
			m.inventoryCursor = min(max(0, m.inventoryCount()-1), m.inventoryCursor+1)
			m.updateViewport()
		} else {
			m.viewport.ScrollDown(1)
		}
	case "pgdown":
		m.viewport.HalfPageDown()
	case "pgup":
		m.viewport.HalfPageUp()
	case "home":
		m.viewport.GotoTop()
	case "end":
		m.viewport.GotoBottom()
	}
	return nil
}
func (m *Model) openForm(kind, title, key string, labels, values []string) tea.Cmd {
	f := &form{Kind: kind, Title: title, Key: key, Labels: labels}
	for i := range labels {
		input := textinput.New()
		input.CharLimit = 4000
		input.Width = max(20, m.width-12)
		if i < len(values) {
			input.SetValue(values[i])
		}
		f.Inputs = append(f.Inputs, input)
	}
	m.form = f
	if len(f.Inputs) > 0 {
		return f.Inputs[0].Focus()
	}
	return nil
}
func (m *Model) confirm(kind, title, key string) tea.Cmd {
	m.form = &form{Kind: kind, Title: title, Key: key, Confirm: true}
	return nil
}
func (m *Model) formKey(key tea.KeyMsg) tea.Cmd {
	f := m.form
	if key.String() == "esc" {
		m.form = nil
		return nil
	}
	if f.Confirm {
		if key.String() == "n" {
			m.form = nil
			return nil
		}
		if key.String() == "y" {
			return m.submitForm()
		}
		return nil
	}
	if key.String() == "tab" || key.String() == "down" {
		f.Inputs[f.Index].Blur()
		f.Index = (f.Index + 1) % len(f.Inputs)
		return f.Inputs[f.Index].Focus()
	}
	if key.String() == "shift+tab" || key.String() == "up" {
		f.Inputs[f.Index].Blur()
		f.Index = (f.Index + len(f.Inputs) - 1) % len(f.Inputs)
		return f.Inputs[f.Index].Focus()
	}
	if key.String() == "enter" {
		if f.Index+1 < len(f.Inputs) {
			f.Inputs[f.Index].Blur()
			f.Index++
			return f.Inputs[f.Index].Focus()
		}
		return m.submitForm()
	}
	var cmd tea.Cmd
	f.Inputs[f.Index], cmd = f.Inputs[f.Index].Update(key)
	return cmd
}
func (m *Model) submitForm() tea.Cmd {
	if m.busy {
		m.notice = "An operation is still pending"
		return nil
	}
	f := m.form
	values := []string{}
	for _, input := range f.Inputs {
		values = append(values, input.Value())
	}
	switch f.Kind {
	case "quit":
		m.form = nil
		return tea.Quit
	case "filter":
		m.filter = values[0]
		m.form = nil
		m.rebuildRows()
		return nil
	case "home":
		if m.demo {
			m.notice = "Home changes are disabled in demo mode"
			m.form = nil
			return nil
		}
		home := config.Home{Name: values[0], Path: values[1], Endpoint: strings.TrimSpace(values[2])}
		if err := home.Validate(); err != nil {
			m.notice = err.Error()
			return nil
		}
		info, err := os.Stat(home.Path)
		if err != nil || !info.IsDir() {
			m.notice = "Codex home must already exist"
			return nil
		}
		homes := m.homeConfigs()
		homes = append(homes, home)
		if err := config.SaveHomes(m.registry, homes); err != nil {
			m.notice = err.Error()
			return nil
		}
		m.addHome(home)
		m.form = nil
		m.notice = "Home registered"
		return m.connect(home.Name)
	case "forget":
		if m.demo {
			m.notice = "Home changes are disabled in demo mode"
			m.form = nil
			return nil
		}
		homes := []config.Home{}
		for _, home := range m.homeConfigs() {
			if home.Name != f.Key {
				homes = append(homes, home)
			}
		}
		if err := config.SaveHomes(m.registry, homes); err != nil {
			m.notice = err.Error()
			return nil
		}
		if c := m.homes[f.Key].Client; c != nil {
			c.Close()
		}
		delete(m.homes, f.Key)
		m.order = nil
		for _, home := range homes {
			m.order = append(m.order, home.Name)
		}
		m.homeFilter = 0
		m.form = nil
		m.rebuildRows()
		m.notice = "Home forgotten; files retained. Automatically discovered homes return on restart."
		return nil
	case "mcp-remove":
		m.form = nil
		return m.changeMCP("remove")
	case "mcp-add":
		if values[0] == "" || values[1] == "" {
			m.notice = "Name and command / URL are required"
			return nil
		}
		if _, exists := m.mcp.Servers[values[0]]; exists {
			m.notice = "Server already exists"
			return nil
		}
		server := map[string]any{"enabled": true}
		if strings.HasPrefix(values[1], "http://") || strings.HasPrefix(values[1], "https://") {
			u, err := url.Parse(values[1])
			if err != nil || u.Host == "" || u.User != nil {
				m.notice = "Enter a valid HTTP URL without embedded credentials"
				return nil
			}
			server["url"] = values[1]
		} else {
			var args []string
			if err := json.Unmarshal([]byte(values[2]), &args); err != nil {
				m.notice = "Arguments must be a JSON array of strings, e.g. [\"--flag\"]"
				return nil
			}
			server["command"] = values[1]
			server["args"] = args
		}
		conf := cloneMCP(m.mcp)
		if conf.Servers == nil {
			m.notice = "Load MCP configuration first"
			return nil
		}
		conf.Servers[values[0]] = server
		m.form = nil
		return m.writeMCP(conf)
	}
	m.form = nil
	if m.demo {
		m.notice = "Demo controls are disabled"
		return nil
	}
	if f.Kind == "new" {
		name := strings.TrimSuffix(f.Key, "/")
		return m.operation("new", f.Key, func(ctx context.Context, c *codex.Client) operationMsg {
			t, err := c.NewThread(ctx, values[0])
			return operationMsg{Thread: t, Text: "Session created", Err: err}
		}, name)
	}
	if m.selected == nil || m.selected.key() != f.Key {
		m.notice = "Session selection changed; action cancelled"
		return nil
	}
	r := *m.selected
	switch f.Kind {
	case "attach":
		return m.operation("attach", r.key(), func(ctx context.Context, c *codex.Client) operationMsg {
			t, err := c.Resume(ctx, r.Thread.ID)
			return operationMsg{Thread: t, Text: "Attached; live events enabled", Err: err}
		}, r.Home)
	case "interrupt":
		turn := m.homes[r.Home].Active[r.Thread.ID]
		if !m.homes[r.Home].Attached[r.Thread.ID] {
			m.notice = "Attach first"
			return nil
		}
		return m.operation("interrupt", r.key(), func(ctx context.Context, c *codex.Client) operationMsg {
			return operationMsg{Text: "Interrupt requested", Err: c.Interrupt(ctx, r.Thread.ID, turn)}
		}, r.Home)
	case "approve", "decline", "input", "reject":
		request := *f.Request
		return m.operation("reply", r.key(), func(ctx context.Context, c *codex.Client) operationMsg {
			var err error
			switch f.Kind {
			case "approve":
				err = c.Reply(ctx, request.ID, map[string]string{"decision": "accept"})
			case "decline":
				err = c.Reply(ctx, request.ID, map[string]string{"decision": "decline"})
			case "reject":
				err = c.Reject(ctx, request.ID)
			case "input":
				var params struct {
					Questions []struct {
						ID string `json:"id"`
					} `json:"questions"`
				}
				_ = json.Unmarshal(request.Params, &params)
				answers := map[string]any{}
				for i, q := range params.Questions {
					answers[q.ID] = map[string]any{"answers": []string{values[i]}}
				}
				err = c.Reply(ctx, request.ID, map[string]any{"answers": answers})
			}
			return operationMsg{Text: "Response sent", RequestID: string(request.ID), Err: err}
		}, r.Home)
	}
	return nil
}
func (m *Model) homeConfigs() []config.Home {
	homes := []config.Home{}
	for _, name := range m.order {
		homes = append(homes, m.homes[name].Home)
	}
	return homes
}
func (m *Model) operation(op, key string, fn func(context.Context, *codex.Client) operationMsg, home string) tea.Cmd {
	if m.busy {
		m.notice = "An operation is still pending"
		return nil
	}
	h := m.homes[home]
	if h == nil || h.Client == nil || !h.Client.Connected() {
		m.notice = "Home disconnected; reconnect with r"
		return nil
	}
	m.busy = true
	m.notice = "Working…"
	ctx, c := m.ctx, h.Client
	return func() tea.Msg { result := fn(ctx, c); result.Op = op; result.Key = key; return result }
}
func (m *Model) sendMessage() tea.Cmd {
	if m.selected == nil || m.demo {
		return nil
	}
	r := *m.selected
	h := m.homes[r.Home]
	if !h.Attached[r.Thread.ID] {
		m.notice = "Attach before sending"
		return nil
	}
	text, turn := m.composer.Value(), h.Active[r.Thread.ID]
	return m.operation("message", r.key(), func(ctx context.Context, c *codex.Client) operationMsg {
		turn, err := c.Message(ctx, r.Thread.ID, turn, text)
		return operationMsg{Text: "Message accepted", Turn: turn, Err: err}
	}, r.Home)
}
func (m *Model) toggleSkill() tea.Cmd {
	if m.selected == nil || m.inventoryCursor >= len(m.skills) || m.demo {
		return nil
	}
	r := *m.selected
	skill := m.skills[m.inventoryCursor]
	return m.operation("skill", r.key(), func(ctx context.Context, c *codex.Client) operationMsg {
		return operationMsg{Text: "Skill setting saved", Err: c.ToggleSkill(ctx, skill)}
	}, r.Home)
}
func (m *Model) viewSkill() tea.Cmd {
	if m.inventoryCursor >= len(m.skills) {
		return nil
	}
	s := m.skills[m.inventoryCursor]
	file, err := os.Open(s.Path)
	if err != nil {
		m.notice = err.Error()
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 128<<10))
	if err != nil {
		m.notice = err.Error()
		return nil
	}
	m.skillContent = s.Path + "\n\n" + string(data)
	m.viewport.SetContent(wrap(m.skillContent, m.viewport.Width))
	m.viewport.GotoTop()
	m.notice = "Skill file (up to 128 KiB); PgUp/PgDn scroll, tab changes view"
	return nil
}
func cloneMCP(original codex.MCPConfig) codex.MCPConfig {
	data, _ := json.Marshal(original.Servers)
	var servers map[string]map[string]any
	_ = json.Unmarshal(data, &servers)
	return codex.MCPConfig{Servers: servers, Version: original.Version, File: original.File}
}
func (m *Model) mcpNames() []string {
	var names []string
	for n := range m.mcp.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func (m *Model) changeMCP(action string) tea.Cmd {
	names := m.mcpNames()
	if m.inventoryCursor >= len(names) || m.demo {
		return nil
	}
	name := names[m.inventoryCursor]
	conf := cloneMCP(m.mcp)
	if action == "remove" {
		delete(conf.Servers, name)
	} else {
		enabled, ok := conf.Servers[name]["enabled"].(bool)
		conf.Servers[name]["enabled"] = ok && !enabled
	}
	return m.writeMCP(conf)
}
func (m *Model) writeMCP(conf codex.MCPConfig) tea.Cmd {
	if m.selected == nil || m.demo {
		return nil
	}
	r := *m.selected
	return m.operation("mcp", r.key(), func(ctx context.Context, c *codex.Client) operationMsg {
		return operationMsg{Text: "MCP configuration saved and reload requested", Err: c.WriteMCP(ctx, conf)}
	}, r.Home)
}
func (m *Model) requests() []codex.Event {
	if m.selected == nil {
		return nil
	}
	var requests []codex.Event
	for _, request := range m.homes[m.selected.Home].Requests {
		var p struct {
			ThreadID string `json:"threadId"`
		}
		_ = json.Unmarshal(request.Params, &p)
		if p.ThreadID == "" || p.ThreadID == m.selected.Thread.ID {
			requests = append(requests, request)
		}
	}
	return requests
}
func (m *Model) inventoryCount() int {
	switch m.tab {
	case 4:
		return len(m.skills)
	case 5:
		return len(m.mcp.Servers)
	case 6:
		return len(m.requests())
	}
	return 0
}
func (m *Model) requestForm(decline bool) tea.Cmd {
	requests := m.requests()
	if m.inventoryCursor >= len(requests) {
		return nil
	}
	request := requests[m.inventoryCursor]
	key := m.selected.key()
	switch request.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		kind, title := "approve", "Approve this request once?"
		if decline {
			kind, title = "decline", "Decline this request?"
		}
		m.confirm(kind, title+"\n"+pretty(request.Params), key)
		m.form.Request = &request
	case "item/tool/requestUserInput":
		var params struct {
			Questions []struct {
				ID, Question string
				Options      []struct{ Label, Description string }
			}
		}
		_ = json.Unmarshal(request.Params, &params)
		labels := []string{}
		for _, q := range params.Questions {
			label := q.Question
			for _, o := range q.Options {
				label += "\n • " + o.Label + ": " + o.Description
			}
			labels = append(labels, label)
		}
		if len(labels) == 0 {
			m.notice = "Request contains no supported questions"
			return nil
		}
		cmd := m.openForm("input", "Codex needs input", key, labels, nil)
		m.form.Request = &request
		return cmd
	default:
		m.confirm("reject", fmt.Sprintf("Unsupported request %s. Reject it so the turn can continue?", request.Method), key)
		m.form.Request = &request
	}
	return nil
}
func pretty(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
