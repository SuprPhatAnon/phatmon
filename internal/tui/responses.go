package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"phatmon/internal/codex"
)

type responseBlock struct {
	key, text string
	tools     []codex.Item
}

type responseView struct {
	list, output                    viewport.Model
	blocks                          []responseBlock
	selected                        int
	key                             string
	focusOutput, swapped, following bool
	starts                          []int
}

func (m *Model) resetResponses() {
	m.responses = responseView{list: viewport.New(1, 1), output: viewport.New(1, 1), following: true}
	m.older = false
	m.detailGeneration++
	m.detailPending = false
	m.detailItems = nil
	m.detailUnseeded = nil
	m.detailTurns = nil
	m.detailStatus = false
}

// Group by assistant message, not turn: a single turn can contain many responses.
func responseBlocks(thread codex.Thread) []responseBlock {
	var blocks []responseBlock
	for ti, turn := range thread.Turns {
		for ii, item := range turn.Items {
			if item.Type == "agentMessage" {
				key := turn.ID + "/" + item.ID
				if item.ID == "" {
					key = fmt.Sprintf("%s/%d/%d", turn.ID, ti, ii)
				}
				blocks = append(blocks, responseBlock{key: key, text: item.Text})
			} else if len(blocks) > 0 && isToolItem(item.Type) {
				last := &blocks[len(blocks)-1]
				last.tools = append(last.tools, item)
			}
		}
	}
	return blocks
}

func isToolItem(kind string) bool {
	switch kind {
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "functionCallOutput", "collabAgentToolCall", "webSearch", "imageView", "imageGeneration", "sleep":
		return true
	}
	return false
}

func (m *Model) updateResponses() {
	r := &m.responses
	if r.list.Width == 0 {
		m.resetResponses()
	}
	width := max(20, m.width-5)
	r.list.Width = max(10, width*2/5)
	r.output.Width = max(10, width-r.list.Width)
	r.list.Height = max(1, m.height-16)
	r.output.Height = r.list.Height
	r.blocks = responseBlocks(m.thread)
	oldKey := r.key
	if r.following {
		r.selected = max(0, len(r.blocks)-1)
	} else {
		r.selected = min(r.selected, max(0, len(r.blocks)-1))
		for i, block := range r.blocks {
			if block.key == r.key {
				r.selected = i
				break
			}
		}
	}
	r.key = ""
	if len(r.blocks) > 0 {
		r.key = r.blocks[r.selected].key
	}
	changed := oldKey != r.key
	var list strings.Builder
	r.starts = nil
	line := 0
	for i, block := range r.blocks {
		r.starts = append(r.starts, line)
		prefix := fmt.Sprintf("  %d  ", i+1)
		if i == r.selected {
			prefix = fmt.Sprintf("› %d  ", i+1)
		}
		text := block.text
		if text == "" {
			text = "Receiving response…"
		}
		text = ansi.Wrap(safe(prefix+text), r.list.Width, "")
		if i == r.selected {
			text = selectedStyle.Render(text)
		}
		list.WriteString(text + "\n\n")
		line += strings.Count(text, "\n") + 2
	}
	if len(r.blocks) == 0 {
		list.WriteString("Waiting for an assistant response.")
	}
	r.list.SetContent(strings.TrimSuffix(list.String(), "\n\n"))
	if r.following {
		r.list.GotoBottom()
	} else if changed && len(r.starts) > 0 {
		r.list.SetYOffset(r.starts[r.selected])
	}
	var output strings.Builder
	if len(r.blocks) > 0 {
		for _, item := range r.blocks[r.selected].tools {
			body := item.Body()
			if len(body) > 64<<10 {
				body = "[earlier output truncated]\n" + body[len(body)-(64<<10):]
			}
			output.WriteString(strings.ToUpper(item.Type))
			if item.Status != "" {
				output.WriteString(" · " + item.Status)
			}
			output.WriteString("\n" + body + "\n\n")
		}
	}
	if output.Len() == 0 {
		output.WriteString("No tool output after this response yet.")
	}
	r.output.SetContent(wrap(strings.TrimSpace(output.String()), r.output.Width))
	if r.following {
		r.output.GotoBottom()
	} else if changed {
		r.output.GotoTop()
	}
}

func (m *Model) responsesView() string {
	r := &m.responses
	mode := "PAUSED · f follow"
	if r.following {
		mode = "CURRENT"
		if m.demo || m.homes[m.selected.Home].Attached[m.thread.ID] {
			mode = "LIVE"
		}
	}
	listTitle := fmt.Sprintf("Responses %d/%d", min(r.selected+1, len(r.blocks)), len(r.blocks))
	if m.older {
		listTitle += " · recent 20 turns"
	}
	outputTitle := "Tool output · " + mode
	pane := func(title string, v viewport.Model, focused bool) string {
		style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(v.Width)
		if focused {
			style = style.BorderForeground(lipgloss.Color("#67E8C0"))
			title = "› " + title
		}
		return style.Render(fit(title, v.Width) + "\n" + v.View())
	}
	left := pane(listTitle, r.list, !r.focusOutput)
	right := pane(outputTitle, r.output, r.focusOutput)
	if r.swapped {
		left, right = right, left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m *Model) responseKey(key tea.KeyMsg) bool {
	r := &m.responses
	switch key.String() {
	case "tab", "shift+tab", "enter":
		r.focusOutput = !r.focusOutput
	case "s":
		r.swapped = !r.swapped
	case "f":
		r.following = true
		m.updateResponses()
	case "up", "k", "down", "j":
		r.following = false
		if r.focusOutput {
			if key.String() == "up" || key.String() == "k" {
				r.output.ScrollUp(1)
			} else {
				r.output.ScrollDown(1)
			}
		} else if len(r.blocks) > 0 {
			next := r.selected + 1
			if key.String() == "up" || key.String() == "k" {
				next = r.selected - 1
			}
			next = max(0, min(next, len(r.blocks)-1))
			r.selected = next
			// Clear the key so the new index takes precedence over retained selection.
			r.key = ""
			m.updateResponses()
		}
	case "pgup", "pgdown", "home", "end":
		r.following = false
		v := &r.list
		if r.focusOutput {
			v = &r.output
		}
		switch key.String() {
		case "pgup":
			v.HalfPageUp()
		case "pgdown":
			v.HalfPageDown()
		case "home":
			v.GotoTop()
		case "end":
			v.GotoBottom()
		}
	default:
		return false
	}
	return true
}
