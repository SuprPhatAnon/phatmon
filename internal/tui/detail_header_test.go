package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"phatmon/internal/codex"
	"phatmon/internal/metrics"
)

func TestDetailHeaderSelectedHomeMetrics(t *testing.T) {
	m := responseModel(t)
	h := m.homes[m.selected.Home]
	five, week := 300, 10080
	window := int64(100000)
	h.Quotas = codex.Quotas{RateLimits: &codex.RateLimit{Primary: &codex.Window{WindowDurationMins: &five, UsedPercent: 25}, Secondary: &codex.Window{WindowDurationMins: &week, UsedPercent: 60}}}
	h.Usage[m.thread.ID] = &codex.Usage{Last: codex.TokenBreakdown{TotalTokens: 20000}, ModelContextWindow: &window}
	m.thread.Model = "test-model"
	m.thread.Cwd = "/projects/selected-project"
	m.git[m.thread.Cwd] = metrics.Git{Branch: "main", Ahead: 2, Behind: 1, Modified: 3, Untracked: 1}
	for _, home := range m.homes {
		if home != h {
			home.Quotas = codex.Quotas{}
			home.Usage = nil
		}
	}
	header := ansi.Strip(m.detailHeader())
	for _, want := range []string{"Quota left", "5h 75%", "Weekly 40%", "Model test-model", "Ctx 20% (20.0k/100.0k)", "/projects/selected-project", "Git main", "↑2 ↓1", "3 modified, 1 new"} {
		if !strings.Contains(header, want) {
			t.Fatalf("missing %q:\n%s", want, header)
		}
	}
	h.QuotaError = "refresh failed"
	if !strings.Contains(ansi.Strip(m.detailHeader()), "stale") {
		t.Fatal("quota freshness hidden")
	}
	// Missing data must not appear as a zero-percent context or clean repository.
	h.Quotas = codex.Quotas{}
	h.Usage = nil
	m.git = nil
	m.thread.Model = ""
	header = ansi.Strip(m.detailHeader())
	for _, want := range []string{"unavailable", "Model unknown", "Ctx —", "Git unavailable"} {
		if !strings.Contains(header, want) {
			t.Fatalf("missing unknown state %q:\n%s", want, header)
		}
	}
}

func TestDetailHeaderFitsWithoutShrinkingPanes(t *testing.T) {
	m := responseModel(t)
	m.thread.Name = strings.Repeat("Very long title ", 30)
	m.thread.Cwd = "/very/long/parent/directory/with/many/segments/selected-project"
	m.thread.Model = "a-very-long-model-identifier"
	for _, size := range [][2]int{{65, 18}, {80, 24}, {120, 40}, {160, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		header := m.detailHeader()
		if len(strings.Split(header, "\n")) != 4 {
			t.Fatal("header height changed")
		}
		for _, line := range strings.Split(header, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("header overflows at %v", size)
			}
		}
		if !strings.Contains(header, "selected-project") || !strings.Contains(header, "Ctx") {
			t.Fatalf("directory or context lost at %v:\n%s", size, header)
		}
		if m.responses.list.Height != size[1]-16 {
			t.Fatal("header reduced pane height")
		}
	}
}

func TestDetailHeaderModelRefreshIsHomeScoped(t *testing.T) {
	m := responseModel(t)
	thread := m.thread
	thread.Model = "updated-model"
	thread.Cwd = "/updated/project"
	other := "personal"
	if m.selected.Home == other {
		other = "work"
	}
	m.Update(snapshotMsg{Name: other, Threads: []codex.Thread{thread}})
	if m.thread.Model == thread.Model {
		t.Fatal("model crossed homes")
	}
	m.Update(snapshotMsg{Name: m.selected.Home, Threads: []codex.Thread{thread}, Git: map[string]metrics.Git{thread.Cwd: {Branch: "new-branch"}}})
	header := ansi.Strip(m.detailHeader())
	for _, want := range []string{"updated-model", "/updated/project", "new-branch"} {
		if !strings.Contains(header, want) {
			t.Fatalf("refresh missing %s", want)
		}
	}
}
