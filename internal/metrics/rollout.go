package metrics

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"phatmon/internal/codex"
)

type savedTokens struct {
	Total     int64 `json:"total_tokens"`
	Input     int64 `json:"input_tokens"`
	Cached    int64 `json:"cached_input_tokens"`
	Output    int64 `json:"output_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
}

func (s savedTokens) usage() codex.TokenBreakdown {
	return codex.TokenBreakdown{TotalTokens: s.Total, InputTokens: s.Input, CachedInputTokens: s.Cached, OutputTokens: s.Output, ReasoningOutputTokens: s.Reasoning}
}

// ReadUsage is a bounded, read-only fallback for an unstable on-disk format.
// Only the rollout supplied by app-server inside the registered home is eligible.
func ReadUsage(path, home string) *codex.Usage {
	if path == "" || filepath.Ext(path) != ".jsonl" {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil
	}
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	const limit int64 = 2 << 20
	offset := max(int64(0), info.Size()-limit)
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil
	}
	lines := bytes.Split(data, []byte{'\n'})
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var event struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Payload   struct {
				Type string `json:"type"`
				Info *struct {
					Total  savedTokens `json:"total_token_usage"`
					Last   savedTokens `json:"last_token_usage"`
					Window *int64      `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(lines[i], &event) != nil || event.Type != "event_msg" || event.Payload.Type != "token_count" || event.Payload.Info == nil {
			continue
		}
		v := event.Payload.Info
		return &codex.Usage{Total: v.Total.usage(), Last: v.Last.usage(), ModelContextWindow: v.Window, Source: "saved token event", ObservedAt: event.Timestamp}
	}
	return nil
}

// UsageCache avoids repeatedly parsing large, unchanged rollouts on dashboard refresh.
// Each entry includes home to keep the path containment decision scoped correctly.
type UsageCache struct {
	mu      sync.Mutex
	entries map[string]usageEntry
}
type usageEntry struct {
	size     int64
	modified time.Time
	usage    *codex.Usage
}

func NewUsageCache() *UsageCache { return &UsageCache{entries: map[string]usageEntry{}} }
func (c *UsageCache) Read(path, home string) *codex.Usage {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := home + "\x00" + path
	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()
	if ok && entry.size == info.Size() && entry.modified.Equal(info.ModTime()) {
		return entry.usage
	}
	usage := ReadUsage(path, home)
	c.mu.Lock()
	c.entries[key] = usageEntry{info.Size(), info.ModTime(), usage}
	c.mu.Unlock()
	return usage
}
