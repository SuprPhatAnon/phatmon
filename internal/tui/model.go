package tui

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"phatmon/internal/codex"
	"phatmon/internal/config"
	"phatmon/internal/metrics"
)

type homeState struct {
	Home                                config.Home
	Client                              *codex.Client
	Threads                             []codex.Thread
	Usage                               map[string]*codex.Usage
	Plans                               map[string]codex.Plan
	Attached                            map[string]bool
	Active                              map[string]string
	Requests                            []codex.Event
	Quotas                              codex.Quotas
	Error, QuotaError                   string
	Refreshed, QuotaChecked             time.Time
	Refreshing, QuotaLoading, Truncated bool
}
type row struct {
	Home   string
	Thread codex.Thread
}

func (r row) key() string { return r.Home + "/" + r.Thread.ID }

type connMsg struct {
	Name   string
	Client *codex.Client
	Err    error
}
type snapshotMsg struct {
	Name      string
	Client    *codex.Client
	Threads   []codex.Thread
	Usage     map[string]*codex.Usage
	Git       map[string]metrics.Git
	Truncated bool
	Err       error
}
type quotaMsg struct {
	Name   string
	Client *codex.Client
	Quotas codex.Quotas
	Err    error
}
type eventMsg struct {
	Name   string
	Client *codex.Client
	Event  codex.Event
}
type detailMsg struct {
	Key        string
	Thread     codex.Thread
	Older      bool
	Err        error
	Generation uint64
	Client     *codex.Client
	Attached   bool
	AttachErr  error
}
type inventoryMsg struct {
	Key                string
	Skills             []codex.Skill
	MCP                codex.MCPConfig
	SkillErr, MCPError error
}
type operationMsg struct {
	Op, Key, Text, Turn, RequestID string
	Thread                         codex.Thread
	Err                            error
}
type tickMsg time.Time

type form struct {
	Kind, Title, Key string
	Labels           []string
	Inputs           []textinput.Model
	Index            int
	Confirm          bool
	Request          *codex.Event
}

type Model struct {
	ctx              context.Context
	cancel           context.CancelFunc
	homes            map[string]*homeState
	order            []string
	registry, binary string
	settings         config.Settings
	demo             bool
	width, height    int
	homeFilter       int
	liveOnly         bool
	filter           string
	rows             []row
	cursor           int
	selected         *row
	tab              int
	viewport         viewport.Model
	thread           codex.Thread
	older            bool
	skills           []codex.Skill
	mcp              codex.MCPConfig
	inventoryError   string
	inventoryCursor  int
	form             *form
	composer         textarea.Model
	composing        bool
	busy             bool
	skillContent     string
	usageCache       *metrics.UsageCache
	notice           string
	git              map[string]metrics.Git
	responses        responseView
	detailGeneration uint64
	detailPending    bool
	detailItems      map[string]bool
	detailUnseeded   map[string]bool
	detailTurns      map[string]bool
	detailStatus     bool
}

func New(homes []config.Home, registry string, settings config.Settings, demo bool) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Model{ctx: ctx, cancel: cancel, homes: map[string]*homeState{}, registry: registry, binary: settings.CodexBinary, settings: settings, liveOnly: settings.LiveOnly, demo: demo, width: 120, height: 40, git: map[string]metrics.Git{}, usageCache: metrics.NewUsageCache()}
	for _, h := range homes {
		m.addHome(h)
	}
	m.viewport = viewport.New(116, 24)
	m.resetResponses()
	m.composer = textarea.New()
	m.composer.Placeholder = "Message this session. Ctrl+S sends; Esc keeps the draft."
	m.composer.SetHeight(6)
	m.composer.CharLimit = 32000
	if demo {
		m.seedDemo()
	}
	return m
}
func (m *Model) addHome(h config.Home) {
	m.order = append(m.order, h.Name)
	m.homes[h.Name] = &homeState{Home: h, Usage: map[string]*codex.Usage{}, Plans: map[string]codex.Plan{}, Attached: map[string]bool{}, Active: map[string]string{}}
}
func (m *Model) Close() {
	m.cancel()
	for _, h := range m.homes {
		if h.Client != nil {
			h.Client.Close()
		}
	}
}
func (m *Model) tick() tea.Cmd {
	return tea.Tick(min(m.settings.RefreshInterval(), m.settings.QuotaRefreshInterval()), func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m *Model) Init() tea.Cmd {
	if m.demo {
		m.rebuildRows()
		return nil
	}
	cmds := []tea.Cmd{m.tick()}
	for _, name := range m.order {
		cmds = append(cmds, m.connect(name))
	}
	return tea.Batch(cmds...)
}
func (m *Model) connect(name string) tea.Cmd {
	h := m.homes[name]
	if h.Error == "Connecting…" {
		return nil
	}
	h.Error = "Connecting…"
	home, ctx, binary := h.Home, m.ctx, m.binary
	return func() tea.Msg { c, err := codex.Connect(ctx, home, binary); return connMsg{name, c, err} }
}
func waitEvent(ctx context.Context, name string, c *codex.Client) tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-c.Events:
			return eventMsg{name, c, event}
		case <-ctx.Done():
			return nil
		}
	}
}
func (m *Model) poll(name string) tea.Cmd {
	h := m.homes[name]
	if h.Refreshing || h.Client == nil || !h.Client.Connected() {
		return nil
	}
	h.Refreshing = true
	ctx, c, cache := m.ctx, h.Client, m.usageCache
	return func() tea.Msg {
		threads, truncated, err := c.Threads(ctx)
		usage, git := metrics.Snapshot(ctx, threads, c.Home.Path, cache)
		return snapshotMsg{name, c, threads, usage, git, truncated, err}
	}
}
func (m *Model) pollQuota(name string) tea.Cmd {
	h := m.homes[name]
	if h.QuotaLoading || h.Client == nil || !h.Client.Connected() {
		return nil
	}
	h.QuotaLoading = true
	ctx, c := m.ctx, h.Client
	return func() tea.Msg { q, err := c.Quotas(ctx); return quotaMsg{name, c, q, err} }
}
func (m *Model) readDetail() tea.Cmd {
	if m.selected == nil {
		return nil
	}
	r := *m.selected
	h := m.homes[r.Home]
	if m.demo {
		m.thread = r.Thread
		m.updateViewport()
		return nil
	}
	if h.Client == nil || !h.Client.Connected() {
		m.notice = "Home disconnected; reconnect with r"
		return nil
	}
	c, ctx := h.Client, m.ctx
	m.detailGeneration++
	generation := m.detailGeneration
	m.detailPending = true
	m.detailItems = map[string]bool{}
	m.detailUnseeded = map[string]bool{}
	m.detailTurns = map[string]bool{}
	m.detailStatus = false
	attached := h.Attached[r.Thread.ID]
	return func() tea.Msg {
		t, older, err := c.Detail(ctx, r.Thread.ID)
		result := detailMsg{Key: r.key(), Thread: t, Older: older, Err: err, Generation: generation, Client: c}
		// Subscribe only to a thread already loaded on this server. Stored history
		// still requires an explicit resume because its independent runtime is unknown.
		if err == nil && !attached && (t.Status.Type == "active" || t.Status.Type == "idle" || t.Status.Type == "systemError") {
			live, resumeErr := c.Resume(ctx, r.Thread.ID)
			result.AttachErr = resumeErr
			result.Attached = resumeErr == nil
			if resumeErr == nil {
				result.Thread = live
				if len(live.Turns) > 20 {
					result.Thread.Turns = live.Turns[len(live.Turns)-20:]
					result.Older = true
				}
			}
		}
		return result
	}
}
func (m *Model) inventory() tea.Cmd {
	if m.selected == nil || m.demo {
		return nil
	}
	r := *m.selected
	h := m.homes[r.Home]
	if h.Client == nil || !h.Client.Connected() {
		return nil
	}
	c, ctx := h.Client, m.ctx
	return func() tea.Msg {
		skills, se := c.Skills(ctx, r.Thread.Cwd)
		servers, me := c.MCP(ctx)
		return inventoryMsg{r.key(), skills, servers, se, me}
	}
}
func (m *Model) rebuildRows() {
	key := ""
	if m.cursor < len(m.rows) {
		key = m.rows[m.cursor].key()
	}
	m.rows = nil
	for _, name := range m.order {
		if m.homeFilter > 0 && m.order[m.homeFilter-1] != name {
			continue
		}
		h := m.homes[name]
		for _, t := range h.Threads {
			if t.IsChildAgent() {
				continue
			}
			if m.liveOnly && (t.Status.Type == "notLoaded" || t.Status.Type == "" || (!m.demo && (h.Client == nil || !h.Client.Connected() || h.Error != ""))) {
				continue
			}
			if m.filter != "" && !strings.Contains(strings.ToLower(name+" "+t.Title()+" "+t.Cwd+" "+t.Status.Label()), strings.ToLower(m.filter)) {
				continue
			}
			m.rows = append(m.rows, row{name, t})
		}
	}
	sort.SliceStable(m.rows, func(i, j int) bool {
		a, b := m.rows[i].Thread, m.rows[j].Thread
		if (a.Status.Type == "active") != (b.Status.Type == "active") {
			return a.Status.Type == "active"
		}
		return a.UpdatedAt > b.UpdatedAt
	})
	m.cursor = min(m.cursor, max(0, len(m.rows)-1))
	for i, r := range m.rows {
		if r.key() == key {
			m.cursor = i
			break
		}
	}
}
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.viewport.Width = max(20, msg.Width-4)
		m.viewport.Height = max(3, msg.Height-15)
		m.composer.SetWidth(max(20, msg.Width-8))
		m.updateViewport()
	case connMsg:
		h, ok := m.homes[msg.Name]
		if !ok {
			if msg.Client != nil {
				msg.Client.Close()
			}
			return m, nil
		}
		if msg.Err != nil {
			h.Error = msg.Err.Error()
			return m, nil
		}
		h.Client = msg.Client
		if m.selected != nil && m.selected.Home == msg.Name {
			m.detailGeneration++
			m.detailPending = false
		}
		h.Refreshing = false
		h.QuotaLoading = false
		h.Error = ""
		h.Attached = map[string]bool{}
		h.Active = map[string]string{}
		h.Requests = nil
		return m, tea.Batch(waitEvent(m.ctx, msg.Name, msg.Client), m.poll(msg.Name), m.pollQuota(msg.Name))
	case snapshotMsg:
		h, ok := m.homes[msg.Name]
		if !ok || h.Client != msg.Client {
			return m, nil
		}
		h.Refreshing = false
		if msg.Err != nil {
			h.Error = msg.Err.Error()
			return m, nil
		}
		h.Error = ""
		h.Threads = codex.TopLevelThreads(msg.Threads)
		if m.selected != nil && m.selected.Home == msg.Name {
			for _, thread := range h.Threads {
				if thread.ID != m.selected.Thread.ID {
					continue
				}
				if thread.Model != "" {
					m.thread.Model = thread.Model
				}
				if thread.ModelProvider != "" {
					m.thread.ModelProvider = thread.ModelProvider
				}
				if thread.ReasoningEffort != "" {
					m.thread.ReasoningEffort = thread.ReasoningEffort
				}
				if thread.Cwd != "" {
					m.thread.Cwd = thread.Cwd
				}
				break
			}
		}
		h.Truncated = msg.Truncated
		h.Refreshed = time.Now()
		for id, u := range msg.Usage {
			if h.Usage[id] == nil || h.Usage[id].Source != "live token event" {
				h.Usage[id] = u
			}
		}
		for path, g := range msg.Git {
			m.git[path] = g
		}
		m.rebuildRows()
		m.updateViewport()
		if m.selected != nil && m.selected.Home == msg.Name && !m.detailPending && !h.Attached[m.selected.Thread.ID] && !m.demo {
			for _, thread := range h.Threads {
				if thread.ID == m.selected.Thread.ID && (thread.Status.Type == "active" || thread.Status.Type == "idle") {
					return m, m.readDetail()
				}
			}
		}
	case quotaMsg:
		h, ok := m.homes[msg.Name]
		if !ok || h.Client != msg.Client {
			return m, nil
		}
		h.QuotaLoading = false
		h.QuotaChecked = time.Now()
		if msg.Err != nil {
			h.QuotaError = msg.Err.Error()
		} else {
			h.Quotas = msg.Quotas
			h.QuotaError = ""
		}
		m.updateViewport()
	case eventMsg:
		h, ok := m.homes[msg.Name]
		if !ok || h.Client != msg.Client {
			return m, nil
		}
		m.applyEvent(msg.Name, msg.Event)
		m.rebuildRows()
		m.updateViewport()
		if msg.Event.Method != "phatmon/disconnected" {
			return m, waitEvent(m.ctx, msg.Name, msg.Client)
		}
	case detailMsg:
		if m.selected == nil || m.selected.key() != msg.Key || msg.Generation != m.detailGeneration || (msg.Client != nil && msg.Client != m.homes[m.selected.Home].Client) {
			return m, nil
		}
		m.detailPending = false
		if msg.Err != nil {
			m.notice = msg.Err.Error()
			return m, nil
		}
		m.thread = m.mergeDetail(msg.Thread)
		m.older = msg.Older
		m.selected.Thread = m.thread
		h := m.homes[m.selected.Home]
		if msg.Attached {
			h.Attached[msg.Thread.ID] = true
		}
		if msg.AttachErr != nil {
			m.notice = "History loaded; live attachment failed: " + msg.AttachErr.Error()
		}
		delete(h.Active, msg.Thread.ID)
		for _, t := range m.thread.Turns {
			if t.Status == "inProgress" {
				h.Active[msg.Thread.ID] = t.ID
			}
		}
		m.updateViewport()
	case inventoryMsg:
		if m.selected == nil || m.selected.key() != msg.Key {
			return m, nil
		}
		m.skills = msg.Skills
		m.mcp = msg.MCP
		m.inventoryError = ""
		if msg.SkillErr != nil {
			m.inventoryError += msg.SkillErr.Error() + "\n"
		}
		if msg.MCPError != nil {
			m.inventoryError += msg.MCPError.Error()
		}
		m.inventoryCursor = 0
		m.updateViewport()
	case operationMsg:
		m.busy = false
		if msg.Err != nil {
			m.notice = msg.Err.Error()
			return m, nil
		}
		m.notice = msg.Text
		if msg.Op == "message" {
			m.composer.Reset()
			m.composing = false
		}
		for name, h := range m.homes {
			if strings.HasPrefix(msg.Key, name+"/") {
				id := strings.TrimPrefix(msg.Key, name+"/")
				if msg.Op == "attach" || msg.Op == "new" {
					h.Attached[msg.Thread.ID] = true
					id = msg.Thread.ID
				}
				if msg.Turn != "" {
					h.Active[id] = msg.Turn
				}
				if msg.RequestID != "" {
					requests := h.Requests[:0]
					for _, request := range h.Requests {
						if string(request.ID) != msg.RequestID {
							requests = append(requests, request)
						}
					}
					h.Requests = requests
				}
				if msg.Op == "attach" {
					for _, turn := range msg.Thread.Turns {
						if turn.Status == "inProgress" {
							h.Active[id] = turn.ID
						}
					}
				}
				if msg.Op == "new" {
					h.Threads = append([]codex.Thread{msg.Thread}, h.Threads...)
					r := row{name, msg.Thread}
					m.selected = &r
					m.thread = msg.Thread
					m.tab = 1
					m.resetResponses()
					m.updateViewport()
				}
				break
			}
		}
		if msg.Op == "skill" || msg.Op == "mcp" {
			return m, m.inventory()
		}
		if msg.Op == "attach" || msg.Op == "message" || msg.Op == "interrupt" || msg.Op == "reply" {
			return m, m.readDetail()
		}
	case tickMsg:
		cmds := []tea.Cmd{m.tick()}
		for _, name := range m.order {
			if time.Since(m.homes[name].Refreshed) >= m.settings.RefreshInterval() {
				cmds = append(cmds, m.poll(name))
			}
			if time.Since(m.homes[name].QuotaChecked) >= m.settings.QuotaRefreshInterval() {
				cmds = append(cmds, m.pollQuota(name))
			}
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy {
			m.notice = "Operation pending; waiting for Codex response"
			return m, nil
		}
		if m.form != nil {
			return m, m.formKey(msg)
		}
		if m.composing {
			if msg.String() == "esc" {
				m.composing = false
				m.composer.Blur()
				return m, nil
			}
			if msg.String() == "ctrl+s" {
				return m, m.sendMessage()
			}
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			return m, cmd
		}
		return m, m.key(msg)
	}
	if m.form != nil && !m.form.Confirm && len(m.form.Inputs) > 0 {
		var cmd tea.Cmd
		m.form.Inputs[m.form.Index], cmd = m.form.Inputs[m.form.Index].Update(message)
		return m, cmd
	}
	if m.composing {
		var cmd tea.Cmd
		m.composer, cmd = m.composer.Update(message)
		return m, cmd
	}
	return m, nil
}

func (m *Model) applyEvent(name string, event codex.Event) {
	h := m.homes[name]
	var p struct {
		ThreadID   string          `json:"threadId"`
		TurnID     string          `json:"turnId"`
		Status     codex.Status    `json:"status"`
		TokenUsage codex.Usage     `json:"tokenUsage"`
		Thread     codex.Thread    `json:"thread"`
		Turn       codex.Turn      `json:"turn"`
		RequestID  json.RawMessage `json:"requestId"`
		Message    string          `json:"message"`
	}
	_ = json.Unmarshal(event.Params, &p)
	if len(event.ID) > 0 {
		h.Requests = append(h.Requests, event)
		m.notice = name + ": request needs attention — open Requests tab"
	}
	switch event.Method {
	case "thread/tokenUsage/updated":
		p.TokenUsage.Source = "live token event"
		p.TokenUsage.ObservedAt = time.Now().Format(time.RFC3339)
		h.Usage[p.ThreadID] = &p.TokenUsage
	case "thread/status/changed":
		for i := range h.Threads {
			if h.Threads[i].ID == p.ThreadID {
				h.Threads[i].Status = p.Status
			}
		}
		if m.selected != nil && m.selected.key() == name+"/"+p.ThreadID {
			m.thread.Status = p.Status
		}
	case "thread/started":
		if p.Thread.IsChildAgent() {
			h.Threads = removeThread(h.Threads, p.Thread.ID)
			break
		}
		found := false
		for i := range h.Threads {
			if h.Threads[i].ID == p.Thread.ID {
				h.Threads[i] = p.Thread
				found = true
			}
		}
		if !found {
			h.Threads = append(h.Threads, p.Thread)
		}
	case "turn/started":
		h.Active[p.ThreadID] = p.Turn.ID
	case "turn/completed":
		delete(h.Active, p.ThreadID)
	case "turn/plan/updated":
		var plan codex.Plan
		_ = json.Unmarshal(event.Params, &plan)
		h.Plans[p.ThreadID] = plan
	case "account/rateLimits/updated":
		_ = json.Unmarshal(event.Params, &h.Quotas)
		h.QuotaError = ""
		h.QuotaChecked = time.Now()
	case "serverRequest/resolved":
		requests := h.Requests[:0]
		for _, request := range h.Requests {
			if string(request.ID) != string(p.RequestID) {
				requests = append(requests, request)
			}
		}
		h.Requests = requests
	case "phatmon/disconnected":
		h.Error = p.Message
		h.Attached = map[string]bool{}
		h.Active = map[string]string{}
		h.Requests = nil
	}
	if m.selected != nil && m.selected.key() == name+"/"+p.ThreadID {
		m.applyDetailEvent(event)
	}
}

func removeThread(threads []codex.Thread, id string) []codex.Thread {
	result := threads[:0]
	for _, thread := range threads {
		if thread.ID != id {
			result = append(result, thread)
		}
	}
	return result
}
