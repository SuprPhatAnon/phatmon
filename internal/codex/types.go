package codex

import (
	"encoding/json"
	"fmt"
	"time"
)

type Status struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags"`
}

func (s Status) Label() string {
	switch s.Type {
	case "active":
		for _, flag := range s.ActiveFlags {
			if flag == "waitingOnApproval" {
				return "NEEDS APPROVAL"
			}
			if flag == "waitingOnUserInput" {
				return "NEEDS INPUT"
			}
		}
		return "WORKING"
	case "idle":
		return "IDLE"
	case "systemError":
		return "ERROR"
	default:
		return "STORED · UNKNOWN"
	}
}

type Thread struct {
	Source               json.RawMessage `json:"source"`
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Preview              string          `json:"preview"`
	Cwd                  string          `json:"cwd"`
	Model                string          `json:"model"`
	ModelProvider        string          `json:"modelProvider"`
	ReasoningEffort      string          `json:"reasoningEffort"`
	Path                 string          `json:"path"`
	ParentThreadID       string          `json:"parentThreadId"`
	UpdatedAt            int64           `json:"updatedAt"`
	CreatedAt            int64           `json:"createdAt"`
	Status               Status          `json:"status"`
	CanAcceptDirectInput *bool           `json:"canAcceptDirectInput"`
	Turns                []Turn          `json:"turns"`
}

// IsChildAgent excludes any thread with a parent, regardless of its role or title.
// Subagent source metadata also identifies children on older servers that omit
// parentThreadId. Encoding/json accepts both "subAgent" and legacy "subagent".
func (t Thread) IsChildAgent() bool {
	if t.ParentThreadID != "" {
		return true
	}
	var source struct {
		SubAgent json.RawMessage `json:"subAgent"`
	}
	if json.Unmarshal(t.Source, &source) != nil {
		return false
	}
	return len(source.SubAgent) > 0 && string(source.SubAgent) != "null"
}

func TopLevelThreads(threads []Thread) []Thread {
	visible := make([]Thread, 0, len(threads))
	for _, thread := range threads {
		if !thread.IsChildAgent() {
			visible = append(visible, thread)
		}
	}
	return visible
}

func (t Thread) Title() string {
	if t.Name != "" {
		return t.Name
	}
	if t.Preview != "" {
		return t.Preview
	}
	return t.ID
}

type Turn struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Items      []Item          `json:"items"`
	Error      json.RawMessage `json:"error"`
	DurationMS *int64          `json:"durationMs"`
}
type Item struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Content          json.RawMessage `json:"content"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregatedOutput"`
	Status           string          `json:"status"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Changes          json.RawMessage `json:"changes"`
	Name             string          `json:"name"`
	Namespace        string          `json:"namespace"`
	Output           json.RawMessage `json:"output"`
	Result           json.RawMessage `json:"result"`
	Error            json.RawMessage `json:"error"`
	ContentItems     json.RawMessage `json:"contentItems"`
	Query            string          `json:"query"`
	Path             string          `json:"path"`
}

func (i Item) Body() string {
	switch i.Type {
	case "agentMessage":
		return i.Text
	case "userMessage":
		var content []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(i.Content, &content)
		var text string
		for _, entry := range content {
			text += entry.Text + "\n"
		}
		return text
	case "commandExecution":
		return "$ " + i.Command + "\n" + i.AggregatedOutput
	case "fileChange":
		return string(i.Changes)
	case "mcpToolCall":
		return i.Server + " / " + i.Tool + " · " + i.Status + "\n" + outputText(i.Result) + outputText(i.Error)
	case "functionCallOutput":
		return i.Namespace + "/" + i.Name + "\n" + outputText(i.Output)
	case "dynamicToolCall":
		return i.Namespace + "/" + i.Tool + "\n" + outputText(i.ContentItems)
	case "webSearch":
		return i.Query
	case "imageView":
		return i.Path
	case "collabAgentToolCall":
		return i.Tool + " · " + i.Status
	default:
		return i.Text
	}
}

// Preserve structured tool results as readable JSON and render plain string
// outputs without JSON quoting. Terminal sanitization happens in the TUI.
func outputText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text + "\n"
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw) + "\n"
	}
	pretty, _ := json.MarshalIndent(value, "", "  ")
	return string(pretty) + "\n"
}

type TokenBreakdown struct {
	TotalTokens           int64 `json:"totalTokens"`
	InputTokens           int64 `json:"inputTokens"`
	CachedInputTokens     int64 `json:"cachedInputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
	ReasoningOutputTokens int64 `json:"reasoningOutputTokens"`
}
type Usage struct {
	Total              TokenBreakdown `json:"total"`
	Last               TokenBreakdown `json:"last"`
	ModelContextWindow *int64         `json:"modelContextWindow"`
	Source             string         `json:"-"`
	ObservedAt         string         `json:"-"`
}

func (u *Usage) ContextPercent() (float64, bool) {
	if u == nil || u.ModelContextWindow == nil || *u.ModelContextWindow <= 0 {
		return 0, false
	}
	return 100 * float64(u.Last.TotalTokens) / float64(*u.ModelContextWindow), true
}

type Window struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins *int    `json:"windowDurationMins"`
	ResetsAt           *int64  `json:"resetsAt"`
}

func (w Window) Label() string {
	if w.WindowDurationMins == nil {
		return "Window"
	}
	if *w.WindowDurationMins == 10080 {
		return "Weekly"
	}
	return fmt.Sprintf("%gh", float64(*w.WindowDurationMins)/60)
}
func (w Window) Reset() string {
	if w.ResetsAt == nil {
		return "reset unknown"
	}
	return "resets " + time.Unix(*w.ResetsAt, 0).Local().Format("Mon 15:04")
}

type RateLimit struct {
	LimitID   string  `json:"limitId"`
	LimitName string  `json:"limitName"`
	Primary   *Window `json:"primary"`
	Secondary *Window `json:"secondary"`
	PlanType  string  `json:"planType"`
}

// DisplayWindows returns only reported five-hour and weekly windows, in that order.
// Primary/secondary positions and the existence of a bucket don't imply a duration.
func (r RateLimit) DisplayWindows() []*Window {
	var windows []*Window
	for _, duration := range []int{300, 10080} {
		for _, window := range []*Window{r.Primary, r.Secondary} {
			if window != nil && window.WindowDurationMins != nil && *window.WindowDurationMins == duration {
				windows = append(windows, window)
			}
		}
	}
	return windows
}

type Quotas struct {
	RateLimits *RateLimit           `json:"rateLimits"`
	ByID       map[string]RateLimit `json:"rateLimitsByLimitId"`
}

// Buckets returns only the Codex quota. Unidentified legacy limits are accepted
// only when the response has no per-product buckets.
func (q Quotas) Buckets() map[string]RateLimit {
	if limit, ok := q.ByID["codex"]; ok {
		return map[string]RateLimit{"codex": limit}
	}
	if q.RateLimits != nil && (q.RateLimits.LimitID == "codex" ||
		(q.RateLimits.LimitID == "" && len(q.ByID) == 0)) {
		return map[string]RateLimit{"codex": *q.RateLimits}
	}
	return nil
}

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
}
type Plan struct {
	Explanation string `json:"explanation"`
	Steps       []struct {
		Step   string `json:"step"`
		Status string `json:"status"`
	} `json:"plan"`
}
type MCPConfig struct {
	Servers map[string]map[string]any
	Version string
	File    string
}
