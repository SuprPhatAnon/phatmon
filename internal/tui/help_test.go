package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestHotkeyHelpPreservesSessionAndScroll(t *testing.T) {
	m := responseModel(t)
	press(m, "tab")
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	key, offset, focus := m.responses.key, m.responses.output.YOffset, m.responses.focusOutput
	press(m, "?")
	if !m.showHelp || !strings.Contains(m.View(), "ALL HOTKEYS") || !strings.Contains(m.View(), "Ctrl+S") {
		t.Fatal("help did not open with messaging keys")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.helpViewport.YOffset == 0 {
		t.Fatal("help did not scroll")
	}
	press(m, "m")
	if m.composing {
		t.Fatal("help key leaked into composer action")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.helpViewport.View(), "Typing ?") {
		t.Fatal("end of reference unreachable")
	}
	press(m, "esc")
	if m.showHelp || m.selected == nil || m.responses.key != key || m.responses.output.YOffset != offset || m.responses.focusOutput != focus {
		t.Fatal("closing help changed underlying view")
	}
	press(m, "esc")
	press(m, "?")
	if !m.showHelp {
		t.Fatal("help unavailable on dashboard")
	}
	press(m, "?")
	if m.showHelp {
		t.Fatal("? did not close help")
	}
}

func TestQuestionMarkRemainsTextInput(t *testing.T) {
	m := responseModel(t)
	m.homes[m.selected.Home].Attached[m.thread.ID] = true
	press(m, "m")
	m.composer.SetValue("What now")
	press(m, "?")
	if m.showHelp || !strings.Contains(m.composer.Value(), "?") {
		t.Fatal("question mark hijacked message input")
	}
	if !strings.Contains(m.View(), "Ctrl+S send") {
		t.Fatal("send instruction not visible")
	}
	draft := m.composer.Value()
	press(m, "esc")
	press(m, "?")
	press(m, "esc")
	if m.composer.Value() != draft {
		t.Fatal("help lost draft")
	}
	m.openForm("filter", "Search", "", []string{"Query"}, []string{"test"})
	press(m, "?")
	if m.showHelp || !strings.Contains(m.form.Inputs[0].Value(), "?") {
		t.Fatal("question mark hijacked form input")
	}
}

func TestMessagingKeysAndHelpFitSmallTerminal(t *testing.T) {
	m := responseModel(t)
	for _, size := range [][2]int{{65, 18}, {80, 24}, {120, 40}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, tab := range []string{"1", "2", "5", "6", "7"} {
			press(m, tab)
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			footer := lines[len(lines)-1]
			if !strings.Contains(footer, "m new message") || !strings.Contains(footer, "? all hotkeys") {
				t.Fatalf("primary keys hidden at %v, tab %s: %s", size, tab, footer)
			}
		}
		press(m, "?")
		view := m.View()
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("help exceeds height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("help exceeds width")
			}
		}
		press(m, "?")
	}
}
