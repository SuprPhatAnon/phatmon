package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"phatmon/internal/codex"
	"phatmon/internal/config"
)

func press(m *Model, key string) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	}
	m.Update(msg)
}
func TestDashboardDrilldownAndFilters(t *testing.T) {
	m := New(nil, t.TempDir()+"/registry.json", config.DefaultSettings(), true)
	defer m.Close()
	m.Init()
	view := ansi.Strip(m.View())
	for _, want := range []string{"PHATMON", "personal", "work", "DIRECTORY", "atlas", "phatmon", "NEEDS APPROVAL", "STORED", "Weekly"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %s:\n%s", want, view)
		}
	}
	press(m, "enter")
	if m.selected == nil {
		t.Fatal("enter did not drill down")
	}
	if !strings.Contains(m.View(), "Responses") {
		t.Fatal("detail tabs missing")
	}
	press(m, "2")
	if !strings.Contains(m.responses.list.View(), "identified the relevant code") {
		t.Fatal("responses missing")
	}
	press(m, "3")
	if !strings.Contains(m.viewport.View(), "Implement the changes") {
		t.Fatal("plan missing")
	}
	press(m, "4")
	if !strings.Contains(m.viewport.View(), "session.go") {
		t.Fatal("git files missing")
	}
	press(m, "esc")
	press(m, "l")
	if len(m.rows) != 3 {
		t.Fatal("stored sessions in live filter")
	}
	press(m, "h")
	for _, r := range m.rows {
		if r.Home != "personal" {
			t.Fatal("home filter failed")
		}
	}
	press(m, "/")
	m.form.Inputs[0].SetValue("permission")
	m.submitForm()
	if len(m.rows) != 1 {
		t.Fatalf("search: %#v", m.rows)
	}
}
func TestTerminalSizesAndUntrustedText(t *testing.T) {
	m := New(nil, "", config.DefaultSettings(), true)
	defer m.Close()
	m.Init()
	for _, size := range [][2]int{{65, 18}, {80, 24}, {120, 40}, {160, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "DIRECTORY") || !strings.Contains(view, "atlas") {
			t.Fatalf("directory column missing at %dx%d:\n%s", size[0], size[1], view)
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("line exceeds %d: %q", size[0], line)
			}
		}
	}
	if got := safe("hello\x1b]52;c;secret\x07\x1b[31mred\x1b[0m\rworld"); strings.ContainsAny(got, "\x1b\x07\r") || strings.Contains(got, "secret") {
		t.Fatalf("terminal escape survived: %q", got)
	}
}
func TestLiveUsageAndRequestsStayHomeScoped(t *testing.T) {
	m := New([]config.Home{{Name: "one", Path: t.TempDir()}, {Name: "two", Path: t.TempDir()}}, "", config.DefaultSettings(), false)
	defer m.Close()
	data := json.RawMessage(`{"threadId":"same-id","tokenUsage":{"last":{"totalTokens":20000},"total":{"totalTokens":90000},"modelContextWindow":100000}}`)
	m.applyEvent("one", codex.Event{Method: "thread/tokenUsage/updated", Params: data})
	if m.homes["two"].Usage["same-id"] != nil {
		t.Fatal("usage crossed homes")
	}
	p, ok := m.homes["one"].Usage["same-id"].ContextPercent()
	if !ok || p != 20 {
		t.Fatalf("wrong context %f", p)
	}
	m.applyEvent("one", codex.Event{ID: json.RawMessage(`5`), Method: "item/fileChange/requestApproval", Params: json.RawMessage(`{"threadId":"same-id"}`)})
	if len(m.homes["one"].Requests) != 1 {
		t.Fatal("approval lost")
	}
	m.applyEvent("one", codex.Event{Method: "serverRequest/resolved", Params: json.RawMessage(`{"requestId":5}`)})
	if len(m.homes["one"].Requests) != 0 {
		t.Fatal("resolved request retained")
	}
}
func TestStaleDetailDoesNotReplaceSelection(t *testing.T) {
	m := New(nil, "", config.DefaultSettings(), true)
	defer m.Close()
	m.Init()
	press(m, "enter")
	title := m.thread.Title()
	m.Update(detailMsg{Key: "other/thread", Thread: codex.Thread{Name: "wrong"}})
	if m.thread.Title() != title {
		t.Fatal("stale detail overwrote selected thread")
	}
}
func TestMCPCloneDoesNotMutateSource(t *testing.T) {
	original := codex.MCPConfig{Servers: map[string]map[string]any{"a": {"enabled": true, "env": map[string]any{"TOKEN": "secret"}}}, Version: "v1"}
	clone := cloneMCP(original)
	clone.Servers["a"]["enabled"] = false
	if original.Servers["a"]["enabled"] != true {
		t.Fatal("configuration mutated before commit")
	}
}

func TestConfiguredSettingsApplied(t *testing.T) {
	settings := config.DefaultSettings()
	settings.CodexBinary = "/opt/custom-codex"
	settings.RefreshIntervalSeconds = 10
	settings.QuotaRefreshIntervalSeconds = 120
	settings.LiveOnly = true
	m := New(nil, "", settings, true)
	defer m.Close()
	m.Init()
	if m.binary != "/opt/custom-codex" || !m.liveOnly || len(m.rows) != 3 {
		t.Fatalf("settings not applied: binary=%s live=%v rows=%d", m.binary, m.liveOnly, len(m.rows))
	}
	if m.settings.RefreshInterval().Seconds() != 10 || m.settings.QuotaRefreshInterval().Seconds() != 120 {
		t.Fatal("configured intervals not used")
	}
}

func TestChildAgentsHiddenInSnapshotsEventsAndCounts(t *testing.T) {
	for _, child := range []codex.Thread{
		{ID: "review", Name: "Hidden review agent", Source: json.RawMessage(`{"subAgent":"review"}`)},
		{ID: "request", Name: "Hidden request agent", ParentThreadID: "parent", Source: json.RawMessage(`"unknown"`)},
		{ID: "other", Name: "Hidden child agent", Source: json.RawMessage(`{"subAgent":{"thread_spawn":{"parent_thread_id":"parent","depth":1}}}`)},
	} {
		t.Run(child.ID, func(t *testing.T) {
			m := New(nil, "", config.DefaultSettings(), true)
			defer m.Close()
			m.Init()
			child.Status = codex.Status{Type: "active"}
			h := m.homes["personal"]
			originals := append([]codex.Thread{}, h.Threads...)
			m.Update(snapshotMsg{Name: "personal", Threads: append(originals, child)})
			if len(h.Threads) != len(originals) || len(m.rows) != 4 {
				t.Fatal("snapshot exposed child agent")
			}
			params, _ := json.Marshal(map[string]any{"thread": child})
			m.applyEvent("personal", codex.Event{Method: "thread/started", Params: params})
			m.rebuildRows()
			if len(h.Threads) != len(originals) || len(m.rows) != 4 {
				t.Fatal("live event exposed child agent")
			}
			h.Threads = append(h.Threads, child)
			m.rebuildRows()
			view := ansi.Strip(m.View())
			if len(m.rows) != 4 || !strings.Contains(view, "4 sessions") || strings.Contains(view, child.Name) {
				t.Fatalf("child affected rows/counts:\n%s", view)
			}
		})
	}
}

func TestOnlyFiveHourAndWeeklyQuotasRendered(t *testing.T) {
	m := New(nil, "", config.DefaultSettings(), true)
	defer m.Close()
	m.Init()
	five, weekly, other := 300, 10080, 60
	for _, h := range m.homes {
		h.Quotas = codex.Quotas{ByID: map[string]codex.RateLimit{
			"codex":        {Primary: &codex.Window{WindowDurationMins: &five, UsedPercent: 25}, Secondary: &codex.Window{WindowDurationMins: &weekly, UsedPercent: 40}},
			"hidden-extra": {Primary: &codex.Window{WindowDurationMins: &five}, Secondary: &codex.Window{WindowDurationMins: &weekly}},
			"codex_other":  {Primary: &codex.Window{WindowDurationMins: &five}},
		}}
		for _, view := range []string{ansi.Strip(m.dashboard()), quotaDetail(h)} {
			if !strings.Contains(view, "5h") || !strings.Contains(view, "Weekly") || strings.Contains(view, "hidden-extra") || strings.Contains(view, "codex_other") {
				t.Fatalf("incorrect quota display:\n%s", view)
			}
		}
	}
	h := m.homes["personal"]
	h.Quotas = codex.Quotas{RateLimits: &codex.RateLimit{Primary: &codex.Window{WindowDurationMins: &other}}}
	detail := quotaDetail(h)
	if !strings.Contains(detail, "No five-hour or weekly quota") || strings.Contains(detail, "1h:") {
		t.Fatalf("unsupported window displayed: %s", detail)
	}
}
