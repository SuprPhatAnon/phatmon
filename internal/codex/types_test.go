package codex

import (
	"encoding/json"
	"testing"
)

func TestChildAgentSourceClassification(t *testing.T) {
	for _, test := range []struct {
		source string
		hidden bool
	}{
		{`{"subAgent":"review"}`, true},
		{`{"subagent":"review"}`, true},
		{`"cli"`, false},
		{`"exec"`, false},
		{`{"subAgent":"compact"}`, true},
		{`{"subAgent":{"thread_spawn":{"parent_thread_id":"parent","depth":1}}}`, true},
		{`{"subAgent":{"other":"review"}}`, true},
		{`{"custom":"review"}`, false},
		{`{"subAgent":null}`, false},
		{`null`, false},
		{``, false},
	} {
		thread := Thread{Name: "Review the changes", Source: json.RawMessage(test.source)}
		if got := thread.IsChildAgent(); got != test.hidden {
			t.Fatalf("source %s: hidden=%v, want %v", test.source, got, test.hidden)
		}
	}
}

func TestQuotaWindowsUseReportedDurations(t *testing.T) {
	five, weekly, other := 300, 10080, 60
	for _, test := range []struct {
		name  string
		limit RateLimit
		want  []int
	}{
		{"both", RateLimit{Primary: &Window{WindowDurationMins: &five}, Secondary: &Window{WindowDurationMins: &weekly}}, []int{300, 10080}},
		{"reverse slots", RateLimit{Primary: &Window{WindowDurationMins: &weekly}, Secondary: &Window{WindowDurationMins: &five}}, []int{300, 10080}},
		{"weekly only", RateLimit{Primary: &Window{WindowDurationMins: &other}, Secondary: &Window{WindowDurationMins: &weekly}}, []int{10080}},
		{"five hour only", RateLimit{Secondary: &Window{WindowDurationMins: &five}}, []int{300}},
		{"unknown duration", RateLimit{Primary: &Window{}, Secondary: &Window{WindowDurationMins: &other}}, nil},
		{"no windows", RateLimit{}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			windows := test.limit.DisplayWindows()
			if len(windows) != len(test.want) {
				t.Fatalf("got %d windows, want %d", len(windows), len(test.want))
			}
			for i, window := range windows {
				if *window.WindowDurationMins != test.want[i] {
					t.Fatalf("unexpected window %#v", window)
				}
			}
		})
	}
}

func TestOnlyCodexQuotaBuckets(t *testing.T) {
	for _, test := range []struct {
		name     string
		quotas   Quotas
		wantName string
	}{
		{"prefer codex bucket", Quotas{ByID: map[string]RateLimit{
			"codex": {LimitName: "selected"}, "other": {LimitName: "hidden"},
		}, RateLimits: &RateLimit{LimitID: "codex", LimitName: "fallback"}}, "selected"},
		{"other products", Quotas{ByID: map[string]RateLimit{
			"other": {LimitName: "Codex"}, "codex_other": {LimitName: "hidden"},
		}}, ""},
		{"explicit legacy codex", Quotas{RateLimits: &RateLimit{LimitID: "codex", LimitName: "selected"}}, "selected"},
		{"unidentified legacy", Quotas{RateLimits: &RateLimit{LimitName: "selected"}}, "selected"},
		{"legacy other product", Quotas{RateLimits: &RateLimit{LimitID: "other", LimitName: "hidden"}}, ""},
		{"ambiguous fallback", Quotas{ByID: map[string]RateLimit{"other": {}}, RateLimits: &RateLimit{LimitName: "hidden"}}, ""},
		{"explicit codex fallback", Quotas{ByID: map[string]RateLimit{"other": {}}, RateLimits: &RateLimit{LimitID: "codex", LimitName: "selected"}}, "selected"},
		{"no quotas", Quotas{}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			buckets := test.quotas.Buckets()
			if test.wantName == "" {
				if len(buckets) != 0 {
					t.Fatalf("unexpected quotas: %#v", buckets)
				}
				return
			}
			if len(buckets) != 1 || buckets["codex"].LimitName != test.wantName {
				t.Fatalf("incorrect Codex quota: %#v", buckets)
			}
		})
	}
}

func TestParentFieldExcludesAnyChildRole(t *testing.T) {
	for _, source := range []string{`"cli"`, `"exec"`, `{"custom":"request"}`, `null`, ``} {
		child := Thread{ID: "request-agent", ParentThreadID: "parent", Source: json.RawMessage(source)}
		if !child.IsChildAgent() {
			t.Fatalf("parent ignored for source %s", source)
		}
	}
	var child Thread
	if err := json.Unmarshal([]byte(`{"id":"child","parentThreadId":"parent","source":"unknown"}`), &child); err != nil {
		t.Fatal(err)
	}
	if !child.IsChildAgent() {
		t.Fatal("wire parentThreadId ignored")
	}
	var fork Thread
	if err := json.Unmarshal([]byte(`{"id":"fork","parentThreadId":null,"forkedFromId":"original","source":"cli"}`), &fork); err != nil {
		t.Fatal(err)
	}
	if fork.IsChildAgent() {
		t.Fatal("top-level fork treated as child")
	}
	threads := TopLevelThreads([]Thread{child, fork})
	if len(threads) != 1 || threads[0].ID != "fork" {
		t.Fatalf("incorrect top-level list: %#v", threads)
	}
}
