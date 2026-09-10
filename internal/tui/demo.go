package tui

import (
	"encoding/json"
	"time"

	"phatmon/internal/codex"
	"phatmon/internal/config"
	"phatmon/internal/metrics"
)

func (m *Model) seedDemo() {
	m.homes = map[string]*homeState{}
	m.order = nil
	m.addHome(config.Home{Name: "personal", Path: "/demo/.codex"})
	m.addHome(config.Home{Name: "work", Path: "/demo/.codex-work", Endpoint: "ws://127.0.0.1:4500"})
	now := time.Now().Unix()
	week, hours := 10080, 300
	reset := now + 3600
	window := int64(258000)
	examples := []struct {
		home, id, title, cwd, status, model string
		flags                               []string
		tokens                              int64
	}{
		{"work", "demo-api", "Add streaming progress to the API", "/demo/projects/atlas", "active", "gpt-5.6-terra", nil, 74000},
		{"personal", "demo-review", "Review filesystem permission changes", "/demo/projects/phatmon", "active", "gpt-5.6-terra", []string{"waitingOnApproval"}, 42000},
		{"work", "demo-ui", "Polish account settings navigation", "/demo/projects/console", "idle", "gpt-5.6-terra", nil, 128000},
		{"personal", "demo-stored", "Investigate flaky integration tests", "/demo/projects/tools", "notLoaded", "gpt-5.6-terra", nil, 19000},
	}
	for i, e := range examples {
		t := codex.Thread{ID: e.id, Name: e.title, Preview: e.title, Cwd: e.cwd, Model: e.model, ModelProvider: "openai", ReasoningEffort: "high", Status: codex.Status{Type: e.status, ActiveFlags: e.flags}, UpdatedAt: now - int64(i*300), CreatedAt: now - 3600}
		t.Turns = []codex.Turn{{ID: "turn-" + e.id, Status: "inProgress", Items: []codex.Item{{ID: "user", Type: "userMessage", Content: json.RawMessage(`[{"type":"text","text":"Please work on this task and report progress."}]`)}, {ID: "agent", Type: "agentMessage", Text: "I have identified the relevant code and am checking the implementation against the existing tests."}}}}
		if e.status != "active" {
			t.Turns[0].Status = "completed"
		}
		h := m.homes[e.home]
		h.Threads = append(h.Threads, t)
		h.Refreshed = time.Now()
		h.QuotaChecked = time.Now()
		h.Usage[e.id] = &codex.Usage{Last: codex.TokenBreakdown{TotalTokens: e.tokens}, Total: codex.TokenBreakdown{InputTokens: e.tokens * 4, OutputTokens: 18000, CachedInputTokens: e.tokens * 3, ReasoningOutputTokens: 6000, TotalTokens: e.tokens*4 + 18000}, ModelContextWindow: &window, Source: "synthetic demo", ObservedAt: time.Now().Format(time.RFC3339)}
		h.Quotas = codex.Quotas{RateLimits: &codex.RateLimit{Primary: &codex.Window{UsedPercent: float64(22 + i*8), WindowDurationMins: &hours, ResetsAt: &reset}, Secondary: &codex.Window{UsedPercent: float64(38 + i*6), WindowDurationMins: &week, ResetsAt: &reset}}}
		m.git[e.cwd] = metrics.Git{Branch: "feat/session-monitor", Ahead: 2, Modified: 3, Untracked: 1, Files: []string{".M internal/session.go", ".M internal/events.go", "? internal/session_test.go"}}
		var plan codex.Plan
		_ = json.Unmarshal([]byte(`{"explanation":"Make the change and validate it locally.","plan":[{"step":"Inspect the existing implementation","status":"completed"},{"step":"Implement the changes","status":"inProgress"},{"step":"Run focused checks","status":"pending"}]}`), &plan)
		h.Plans[e.id] = plan
	}
}
