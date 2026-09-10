package tui

import (
	"encoding/json"
	"strings"

	"phatmon/internal/codex"
)

func threadTurn(thread *codex.Thread, id string) *codex.Turn {
	for i := range thread.Turns {
		if thread.Turns[i].ID == id {
			return &thread.Turns[i]
		}
	}
	thread.Turns = append(thread.Turns, codex.Turn{ID: id, Status: "inProgress"})
	return &thread.Turns[len(thread.Turns)-1]
}

func turnItem(turn *codex.Turn, id string) *codex.Item {
	for i := range turn.Items {
		if turn.Items[i].ID == id {
			return &turn.Items[i]
		}
	}
	turn.Items = append(turn.Items, codex.Item{ID: id})
	return &turn.Items[len(turn.Items)-1]
}

func boundedStream(text string) string {
	const limit = 64 << 10
	if len(text) > limit {
		return text[len(text)-limit:]
	}
	return text
}

// Store streaming items in protocol order so tool output belongs to the response
// preceding it, even before an item/completed notification arrives.
func (m *Model) applyDetailEvent(event codex.Event) {
	var p struct {
		TurnID string     `json:"turnId"`
		ItemID string     `json:"itemId"`
		Delta  string     `json:"delta"`
		Turn   codex.Turn `json:"turn"`
		Item   codex.Item `json:"item"`
	}
	if json.Unmarshal(event.Params, &p) != nil {
		return
	}
	markItem := func(turn, item string) {
		if m.detailPending {
			m.detailItems[turn+"/"+item] = true
		}
	}
	switch event.Method {
	case "thread/status/changed":
		if m.detailPending {
			m.detailStatus = true
		}
	case "turn/started", "turn/completed":
		if p.Turn.ID == "" {
			return
		}
		turn := threadTurn(&m.thread, p.Turn.ID)
		items := turn.Items
		for _, item := range p.Turn.Items {
			*turnItem(turn, item.ID) = item
			markItem(p.Turn.ID, item.ID)
		}
		if len(p.Turn.Items) > 0 {
			items = turn.Items
		}
		*turn = p.Turn
		turn.Items = items
		if m.detailPending {
			m.detailTurns[p.Turn.ID] = true
		}
	case "item/started", "item/completed":
		if p.TurnID == "" || p.Item.ID == "" {
			return
		}
		item := turnItem(threadTurn(&m.thread, p.TurnID), p.Item.ID)
		if event.Method == "item/started" && item.Type != "" {
			// A delta may precede its start notification on a resumed connection.
			if item.Command == "" {
				item.Command = p.Item.Command
			}
		} else {
			*item = p.Item
		}
		if event.Method == "item/completed" {
			delete(m.detailUnseeded, p.TurnID+"/"+p.Item.ID)
		}
		markItem(p.TurnID, p.Item.ID)
	case "item/agentMessage/delta", "item/commandExecution/outputDelta":
		if p.TurnID == "" || p.ItemID == "" {
			return
		}
		item := turnItem(threadTurn(&m.thread, p.TurnID), p.ItemID)
		if m.detailPending && item.Type == "" {
			if m.detailUnseeded == nil {
				m.detailUnseeded = map[string]bool{}
			}
			m.detailUnseeded[p.TurnID+"/"+p.ItemID] = true
		}
		if event.Method == "item/agentMessage/delta" {
			item.Type = "agentMessage"
			item.Text = boundedStream(item.Text + p.Delta)
		} else {
			item.Type = "commandExecution"
			item.AggregatedOutput = boundedStream(item.AggregatedOutput + p.Delta)
		}
		markItem(p.TurnID, p.ItemID)
	}
}

// A history request can finish after live events. Overlay only items changed
// during that request so a delayed snapshot cannot erase the current output.
func (m *Model) mergeDetail(snapshot codex.Thread) codex.Thread {
	for _, current := range m.thread.Turns {
		changed := m.detailTurns[current.ID]
		for _, item := range current.Items {
			if m.detailItems[current.ID+"/"+item.ID] {
				turn := threadTurn(&snapshot, current.ID)
				target := turnItem(turn, item.ID)
				if m.detailUnseeded[current.ID+"/"+item.ID] {
					item.Text = mergeStreamPrefix(target.Text, item.Text)
					item.AggregatedOutput = mergeStreamPrefix(target.AggregatedOutput, item.AggregatedOutput)
					if item.Command == "" {
						item.Command = target.Command
					}
				}
				*target = item
			}
		}
		if changed {
			turn := threadTurn(&snapshot, current.ID)
			items := turn.Items
			*turn = current
			turn.Items = items
		}
	}
	if m.detailStatus {
		snapshot.Status = m.thread.Status
	}
	return snapshot
}

// A mid-stream subscription may deliver only the suffix of an item before its
// history snapshot arrives. Retain the snapshot's prefix without repeating overlap.
func mergeStreamPrefix(snapshot, delta string) string {
	if strings.Contains(snapshot, delta) {
		return snapshot
	}
	for overlap := min(len(snapshot), len(delta)); overlap > 0; overlap-- {
		if strings.HasSuffix(snapshot, delta[:overlap]) {
			return snapshot + delta[overlap:]
		}
	}
	return snapshot + delta
}
