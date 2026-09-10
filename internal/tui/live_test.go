package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coder/websocket"
	"phatmon/internal/codex"
	"phatmon/internal/config"
)

func TestOpeningDetailsAutomaticallyAttachesOnlyLoadedSessions(t *testing.T) {
	for _, status := range []string{"active", "idle", "notLoaded", ""} {
		t.Run(status, func(t *testing.T) {
			home := config.Home{Name: "test", Path: t.TempDir()}
			var resumed atomic.Int32
			thread := codex.Thread{ID: "thread", Cwd: home.Path, Status: codex.Status{Type: status}, Turns: []codex.Turn{{ID: "turn", Status: "inProgress", Items: []codex.Item{{ID: "response", Type: "agentMessage", Text: "Live response"}}}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					_, data, err := conn.Read(r.Context())
					if err != nil {
						return
					}
					var req struct {
						ID     json.RawMessage
						Method string
					}
					if json.Unmarshal(data, &req) != nil {
						return
					}
					if len(req.ID) == 0 {
						continue
					}
					var result any = map[string]any{}
					switch req.Method {
					case "initialize":
						result = map[string]any{"codexHome": home.Path}
					case "thread/read":
						metadata := thread
						metadata.Turns = nil
						result = map[string]any{"thread": metadata}
					case "thread/turns/list":
						result = map[string]any{"data": thread.Turns}
					case "thread/resume":
						resumed.Add(1)
						result = map[string]any{"thread": thread}
					}
					reply, _ := json.Marshal(map[string]any{"id": req.ID, "result": result})
					if conn.Write(r.Context(), websocket.MessageText, reply) != nil {
						return
					}
				}
			}))
			defer server.Close()
			home.Endpoint = "ws" + strings.TrimPrefix(server.URL, "http")
			client, err := codex.Connect(context.Background(), home, "")
			if err != nil {
				t.Fatal(err)
			}
			m := New([]config.Home{home}, "", config.DefaultSettings(), false)
			defer m.Close()
			m.homes[home.Name].Client = client
			m.homes[home.Name].Threads = []codex.Thread{thread}
			m.rebuildRows()
			cmd := m.key(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("no detail request")
			}
			m.Update(cmd())
			want := status == "active" || status == "idle"
			if m.homes[home.Name].Attached[thread.ID] != want || (resumed.Load() == 1) != want {
				t.Fatalf("status %q: attached=%v resumes=%d", status, m.homes[home.Name].Attached[thread.ID], resumed.Load())
			}
			if m.tab != 1 || m.responses.key != "turn/response" {
				t.Fatal("did not open current response")
			}
			if want {
				m.Update(m.readDetail()())
				if resumed.Load() != 1 {
					t.Fatal("refresh repeated attachment")
				}
			}
		})
	}
}
