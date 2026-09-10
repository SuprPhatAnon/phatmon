package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"phatmon/internal/codex"
	"phatmon/internal/config"
)

func responseModel(t *testing.T) *Model {
	t.Helper()
	m := New(nil, "", config.DefaultSettings(), true)
	t.Cleanup(m.Close)
	m.Init()
	press(m, "enter")
	m.thread.Turns = []codex.Turn{{ID: "turn", Items: []codex.Item{
		{ID: "user", Type: "userMessage", Content: json.RawMessage(`[{"text":"USER ONLY"}]`)},
		{ID: "a", Type: "agentMessage", Text: "First response"},
		{ID: "reason", Type: "reasoning", Text: "REASONING ONLY"},
		{ID: "cmd-a", Type: "commandExecution", Command: "first command", AggregatedOutput: "FIRST OUTPUT"},
		{ID: "b", Type: "agentMessage", Text: "Latest response"},
		{ID: "cmd-b", Type: "commandExecution", Command: "latest command", AggregatedOutput: strings.Repeat("line\n", 60) + "LATEST OUTPUT"},
	}}}
	m.updateViewport()
	return m
}

func sendEvent(m *Model, method string, params map[string]any) {
	params["threadId"] = m.selected.Thread.ID
	raw, _ := json.Marshal(params)
	m.Update(eventMsg{Name: m.selected.Home, Event: codex.Event{Method: method, Params: raw}})
}

func TestResponsesDefaultGroupingAndNavigation(t *testing.T) {
	m := responseModel(t)
	r := &m.responses
	if m.tab != 1 || r.selected != 1 || !r.following || !r.output.AtBottom() {
		t.Fatal("did not open latest response in follow mode")
	}
	if len(r.blocks) != 2 || len(r.blocks[0].tools) != 1 || len(r.blocks[1].tools) != 1 {
		t.Fatalf("incorrect grouping: %#v", r.blocks)
	}
	list := r.list.View()
	for _, hidden := range []string{"USER ONLY", "REASONING ONLY", "FIRST OUTPUT", "LATEST OUTPUT"} {
		if strings.Contains(list, hidden) {
			t.Fatalf("non-response %s leaked into list", hidden)
		}
	}
	if !strings.Contains(r.output.View(), "LATEST OUTPUT") || strings.Contains(r.output.View(), "FIRST OUTPUT") {
		t.Fatal("wrong tool output selected")
	}
	press(m, "k")
	if r.selected != 0 || r.following || !strings.Contains(r.output.View(), "FIRST OUTPUT") {
		t.Fatal("older response was not selected")
	}
	press(m, "tab")
	if !r.focusOutput || m.tab != 1 {
		t.Fatal("Tab did not switch pane focus")
	}
	press(m, "s")
	if !r.swapped || strings.Index(m.responsesView(), "Tool output") > strings.Index(m.responsesView(), "Responses") {
		t.Fatal("pane positions did not swap")
	}
	press(m, "f")
	if r.selected != 1 || !r.following || !r.output.AtBottom() {
		t.Fatal("follow did not restore current response")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	offset := r.output.YOffset
	if r.following || r.output.AtBottom() {
		t.Fatal("scrolling did not pause follow")
	}
	sendEvent(m, "item/commandExecution/outputDelta", map[string]any{"turnId": "turn", "itemId": "cmd-b", "delta": "\nNEW OUTPUT"})
	if r.output.YOffset != offset {
		t.Fatal("live update moved paused scroll position")
	}
}

func TestResponsesLiveOrderingAndPinnedSelection(t *testing.T) {
	m := responseModel(t)
	sendEvent(m, "item/started", map[string]any{"turnId": "turn", "item": codex.Item{ID: "c", Type: "agentMessage"}})
	sendEvent(m, "item/agentMessage/delta", map[string]any{"turnId": "turn", "itemId": "c", "delta": "New response"})
	sendEvent(m, "item/started", map[string]any{"turnId": "turn", "item": codex.Item{ID: "cmd-c", Type: "commandExecution", Command: "test"}})
	sendEvent(m, "item/commandExecution/outputDelta", map[string]any{"turnId": "turn", "itemId": "cmd-c", "delta": "streamed tool output"})
	if m.responses.selected != 2 || !strings.Contains(m.responses.list.View(), "New response") || !strings.Contains(m.responses.output.View(), "streamed tool output") {
		t.Fatal("new response and tool output did not stream live")
	}
	// Completion replaces the streaming item rather than duplicating it.
	sendEvent(m, "item/completed", map[string]any{"turnId": "turn", "item": codex.Item{ID: "c", Type: "agentMessage", Text: "New response complete"}})
	if len(m.responses.blocks) != 3 || len(m.responses.blocks[2].tools) != 1 {
		t.Fatal("completion duplicated or reordered items")
	}
	press(m, "k")
	pinned := m.responses.key
	sendEvent(m, "turn/started", map[string]any{"turn": codex.Turn{ID: "next", Status: "inProgress"}})
	sendEvent(m, "item/agentMessage/delta", map[string]any{"turnId": "next", "itemId": "d", "delta": "Next turn"})
	if m.responses.key != pinned || m.responses.following {
		t.Fatal("new response stole selection from history")
	}
	press(m, "f")
	if m.responses.key != "next/d" {
		t.Fatal("follow did not select newest response")
	}
	// Empty refreshes and a different session must not retain stale selection.
	m.thread.Turns = nil
	m.updateViewport()
	if len(m.responses.blocks) != 0 {
		t.Fatal("empty history retained responses")
	}
	press(m, "esc")
	press(m, "enter")
	if !m.responses.following || m.responses.focusOutput || m.responses.swapped {
		t.Fatal("session retained previous pane state")
	}
}

func TestResponsesCrossTurnBoundaries(t *testing.T) {
	thread := codex.Thread{Turns: []codex.Turn{
		{ID: "a", Items: []codex.Item{{ID: "first", Type: "agentMessage", Text: "First"}}},
		{ID: "b", Items: []codex.Item{{Type: "userMessage"}, {Type: "commandExecution", Command: "before next response"}, {ID: "second", Type: "agentMessage", Text: "Second"}, {Type: "functionCallOutput", Output: json.RawMessage(`"tool result"`)}}},
	}}
	blocks := responseBlocks(thread)
	if len(blocks) != 2 || len(blocks[0].tools) != 1 || len(blocks[1].tools) != 1 || !strings.Contains(blocks[1].tools[0].Body(), "tool result") {
		t.Fatalf("incorrect response boundaries: %#v", blocks)
	}
}

func TestResponsesResizeAndIndependentScroll(t *testing.T) {
	m := responseModel(t)
	m.thread.Turns[0].Items[1].Text = strings.Repeat("Long response text. ", 100)
	for _, size := range [][2]int{{65, 18}, {80, 24}, {120, 40}, {160, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if !strings.Contains(view, "Responses") || !strings.Contains(view, "Tool output") {
			t.Fatalf("missing pane at %v", size)
		}
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("too tall at %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("too wide at %v: %q", size, line)
			}
		}
	}
	press(m, "k")
	outputOffset := m.responses.output.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.responses.list.YOffset == 0 || m.responses.output.YOffset != outputOffset {
		t.Fatal("list scrolling affected tool output")
	}
}

func TestHistoryRefreshPreservesNewLiveItems(t *testing.T) {
	m := responseModel(t)
	// Simulate a request in flight before the event arrives.
	raw, _ := json.Marshal(m.thread)
	var snapshot codex.Thread
	json.Unmarshal(raw, &snapshot)
	m.detailPending = true
	m.detailItems = map[string]bool{}
	m.detailTurns = map[string]bool{}
	sendEvent(m, "item/agentMessage/delta", map[string]any{"turnId": "new-turn", "itemId": "new-response", "delta": "arrived during refresh"})
	m.Update(detailMsg{Key: m.selected.key(), Thread: snapshot, Generation: m.detailGeneration})
	if len(m.responses.blocks) != 3 || m.responses.key != "new-turn/new-response" {
		t.Fatal("history refresh erased incoming response")
	}
	// A stale response from an earlier open of this same session is ignored.
	m.Update(detailMsg{Key: m.selected.key(), Thread: codex.Thread{ID: "wrong"}, Generation: m.detailGeneration - 1})
	if m.thread.ID == "wrong" {
		t.Fatal("stale read replaced reopened session")
	}
}

func TestResponsesHomeIsolation(t *testing.T) {
	m := responseModel(t)
	other := "personal"
	if m.selected.Home == other {
		other = "work"
	}
	params := json.RawMessage(fmt.Sprintf(`{"threadId":%q,"turnId":"turn","itemId":"b","delta":"WRONG HOME"}`, m.thread.ID))
	m.Update(eventMsg{Name: other, Event: codex.Event{Method: "item/agentMessage/delta", Params: params}})
	if strings.Contains(m.responses.list.View(), "WRONG HOME") {
		t.Fatal("cross-home delta leaked")
	}
}

func TestInitialHistorySeedsMidStreamOutput(t *testing.T) {
	m := responseModel(t)
	original := m.thread
	m.thread.Turns = nil
	m.detailPending = true
	m.detailItems = map[string]bool{}
	m.detailTurns = map[string]bool{}
	m.detailUnseeded = map[string]bool{}
	sendEvent(m, "item/commandExecution/outputDelta", map[string]any{"turnId": "turn", "itemId": "cmd-b", "delta": "LATEST OUTPUT\nNEW SUFFIX"})
	m.Update(detailMsg{Key: m.selected.key(), Thread: original, Generation: m.detailGeneration})
	output := m.thread.Turns[0].Items[5]
	if output.Command != "latest command" || !strings.HasPrefix(output.AggregatedOutput, "line\n") || strings.Count(output.AggregatedOutput, "LATEST OUTPUT") != 1 || !strings.HasSuffix(output.AggregatedOutput, "NEW SUFFIX") {
		t.Fatalf("lost snapshot prefix or duplicated overlap: %#v", output)
	}
}
