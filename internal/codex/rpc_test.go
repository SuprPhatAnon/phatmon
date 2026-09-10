package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"phatmon/internal/config"
)

type fakeServer struct {
	mu          sync.Mutex
	methods     []string
	params      map[string]json.RawMessage
	active      bool
	wrongHome   bool
	childAgents bool
}

func fake(t *testing.T, home string, state *fakeServer) *Client {
	t.Helper()
	state.params = map[string]json.RawMessage{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var req envelope
			if json.Unmarshal(data, &req) != nil {
				return
			}
			state.mu.Lock()
			state.methods = append(state.methods, req.Method)
			state.params[req.Method] = req.Params
			active, wrong, reviews := state.active, state.wrongHome, state.childAgents
			state.mu.Unlock()
			if len(req.ID) == 0 {
				continue
			}
			var result any = map[string]any{}
			var rpcError *RPCError
			switch req.Method {
			case "initialize":
				path := home
				if wrong {
					path = "/wrong/home"
				}
				result = map[string]string{"codexHome": path}
			case "thread/list":
				var p struct{ Cursor string }
				json.Unmarshal(req.Params, &p)
				if p.Cursor == "" {
					threads := []Thread{{ID: "one", Cwd: home, Status: Status{Type: "idle"}}}
					if reviews {
						threads = append(threads, Thread{ID: "stored-review", Source: json.RawMessage(`{"subAgent":"review"}`)}, Thread{ID: "stored-request", ParentThreadID: "one", Source: json.RawMessage(`"unknown"`)})
					}
					result = map[string]any{"data": threads, "nextCursor": "page-two"}
				} else {
					result = map[string]any{"data": []Thread{{ID: "two", Cwd: home}}, "nextCursor": nil}
				}
			case "thread/loaded/list":
				if reviews {
					result = map[string]any{"data": []string{"loaded-review", "loaded-other", "loaded-request"}}
				}
			case "thread/read":
				if reviews {
					var params struct {
						ThreadID string `json:"threadId"`
					}
					json.Unmarshal(req.Params, &params)
					if params.ThreadID == "loaded-review" {
						result = map[string]any{"thread": Thread{ID: params.ThreadID, Source: json.RawMessage(`{"subAgent":"review"}`)}}
						break
					}
					if params.ThreadID == "loaded-request" {
						result = map[string]any{"thread": Thread{ID: params.ThreadID, ParentThreadID: "one", Source: json.RawMessage(`"exec"`)}}
						break
					}
					if params.ThreadID == "loaded-other" {
						result = map[string]any{"thread": Thread{ID: params.ThreadID, Source: json.RawMessage(`{"subAgent":{"thread_spawn":{"parent_thread_id":"one","depth":1}}}`)}}
						break
					}
				}
				status := "idle"
				if active {
					status = "active"
				}
				result = map[string]any{"thread": Thread{ID: "one", Cwd: home, Status: Status{Type: status}}}
			case "thread/resume":
				result = map[string]any{"thread": Thread{ID: "one", Status: Status{Type: "active"}, Turns: []Turn{{ID: "turn-active", Status: "inProgress"}}}}
			case "thread/turns/list":
				result = map[string]any{"data": []Turn{{ID: "newer", Status: "completed"}, {ID: "older", Status: "completed"}}, "nextCursor": "more"}
			case "turn/start":
				result = map[string]any{"turn": Turn{ID: "new-turn", Status: "inProgress"}}
			case "turn/steer":
				result = map[string]string{"turnId": "turn-active"}
			case "config/read":
				result = map[string]any{"layers": []any{map[string]any{"name": map[string]any{"type": "user", "file": filepath.Join(home, "config.toml")}, "version": "version-one", "config": map[string]any{"model": "example", "mcp_servers": map[string]any{"existing": map[string]any{"command": "server", "env": map[string]string{"TOKEN": "hidden"}}}}}}}
			case "config/value/write":
				var p struct{ ExpectedVersion string }
				json.Unmarshal(req.Params, &p)
				if p.ExpectedVersion != "version-one" {
					rpcError = &RPCError{-32000, "configuration changed"}
				}
			case "account/rateLimits/read":
				result = map[string]any{"rateLimits": map[string]any{"primary": map[string]any{"usedPercent": 20, "windowDurationMins": 300}, "secondary": map[string]any{"usedPercent": 65, "windowDurationMins": 10080}}}
			case "test/error":
				rpcError = &RPCError{-32601, "unsupported"}
			case "test/event":
				event, _ := json.Marshal(Event{ID: json.RawMessage(`"approval-1"`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"one","command":"git status"}`)})
				if conn.Write(ctx, websocket.MessageText, event) != nil {
					return
				}
			case "test/slow":
				continue
			}
			response := map[string]any{"id": req.ID, "result": result}
			if rpcError != nil {
				response = map[string]any{"id": req.ID, "error": rpcError}
			}
			encoded, _ := json.Marshal(response)
			if conn.Write(ctx, websocket.MessageText, encoded) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	c, err := Connect(context.Background(), config.Home{Name: "test", Path: home, Endpoint: "ws" + strings.TrimPrefix(server.URL, "http")}, "unused")
	if state.wrongHome {
		if err == nil {
			c.Close()
			t.Fatal("accepted wrong home")
		}
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}
func TestProtocolPaginationEventsAndControl(t *testing.T) {
	state := &fakeServer{}
	c := fake(t, t.TempDir(), state)
	ctx := context.Background()
	threads, truncated, err := c.Threads(ctx)
	if err != nil || truncated || len(threads) != 2 {
		t.Fatalf("%#v %v %v", threads, truncated, err)
	}
	thread, older, err := c.Detail(ctx, "one")
	if err != nil || !older || thread.Turns[0].ID != "older" {
		t.Fatalf("%#v %v", thread, err)
	}
	turn, err := c.Message(ctx, "one", "", "hello")
	if err != nil || turn != "new-turn" {
		t.Fatalf("%s %v", turn, err)
	}
	state.mu.Lock()
	state.active = true
	state.mu.Unlock()
	if _, err = c.Message(ctx, "one", "", "hello"); err == nil {
		t.Fatal("steered without turn ID")
	}
	turn, err = c.Message(ctx, "one", "turn-active", "adjust")
	if err != nil || turn != "turn-active" {
		t.Fatalf("%s %v", turn, err)
	}
	state.mu.Lock()
	steer := string(state.params["turn/steer"])
	state.mu.Unlock()
	if !strings.Contains(steer, `"expectedTurnId":"turn-active"`) {
		t.Fatal(steer)
	}
	if err = c.Call(ctx, "test/event", nil, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-c.Events:
		if event.Method != "item/commandExecution/requestApproval" || string(event.ID) != `"approval-1"` {
			t.Fatalf("%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("event lost")
	}
	var rpcErr *RPCError
	if err = c.Call(ctx, "test/error", nil, nil); !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("bad error: %v", err)
	}
	timeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err = c.Call(timeout, "test/slow", nil, nil); err == nil {
		t.Fatal("no timeout")
	}
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Fatal("pending request leak")
	}
}
func TestHomeMismatch(t *testing.T) { fake(t, t.TempDir(), &fakeServer{wrongHome: true}) }
func TestMCPVersionedWritePreservesOtherServers(t *testing.T) {
	state := &fakeServer{}
	c := fake(t, t.TempDir(), state)
	ctx := context.Background()
	conf, err := c.MCP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conf.Servers["new"] = map[string]any{"url": "http://localhost:8080/mcp"}
	if err = c.WriteMCP(ctx, conf); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	params := string(state.params["config/value/write"])
	methods := strings.Join(state.methods, ",")
	state.mu.Unlock()
	if !strings.Contains(params, `"existing"`) || !strings.Contains(params, `"expectedVersion":"version-one"`) || !strings.Contains(methods, "config/mcpServer/reload") {
		t.Fatal(params, methods)
	}
	conf.Version = "stale"
	if err = c.WriteMCP(ctx, conf); err == nil {
		t.Fatal("stale write succeeded")
	}
}
func TestConcurrentRPC(t *testing.T) {
	c := fake(t, t.TempDir(), &fakeServer{})
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := c.Quotas(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
}

// Opt-in smoke test: creates only a temporary Codex home, never sends a model prompt.
func TestInstalledCodex(t *testing.T) {
	if os.Getenv("PHATMON_TEST_CODEX") != "1" {
		t.Skip("set PHATMON_TEST_CODEX=1 to check the installed CLI with an isolated home")
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skip(err)
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[history]\npersistence = \"save-all\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(home, "skills", "phatmon-test")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: phatmon-test\ndescription: Isolated test fixture.\n---\nFixture only.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := Connect(ctx, config.Home{Name: "isolated", Path: home}, binary)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	threads, _, err := c.Threads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 0 {
		t.Fatal("isolated home unexpectedly has threads")
	}
	conf, err := c.MCP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Connected; user config version available: %v", conf.Version != "")
	conf.Servers["phatmon-test"] = map[string]any{"command": "unused-disabled-test-server", "enabled": false}
	if err := c.WriteMCP(ctx, conf); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil || !strings.Contains(string(content), "save-all") {
		t.Fatalf("unrelated config lost: %s %v", content, err)
	}
	skills, err := c.Skills(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, skill := range skills {
		if skill.Path == skillPath {
			found = true
			if err := c.ToggleSkill(ctx, skill); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("fixture skill not discovered")
	}
	t.Log("MCP write/reload and skill toggle succeeded in isolated home")
	if _, err = c.Skills(ctx, home); err != nil {
		t.Logf("Skill discovery diagnostic: %v", err)
	}
	// An empty new thread exercises the permission/config schema without invoking a model.
	thread, err := c.NewThread(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	if thread.ID == "" {
		t.Fatal("thread ID empty")
	}
	t.Log(fmt.Sprintf("Created empty isolated thread %s (no prompt sent)", thread.ID))
	threads, _, err = c.Threads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, candidate := range threads {
		if candidate.ID == thread.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("new loaded session absent from dashboard listing")
	}
}

func TestChildAgentsExcludedFromStoredAndLoadedSessions(t *testing.T) {
	state := &fakeServer{childAgents: true}
	c := fake(t, t.TempDir(), state)
	threads, truncated, err := c.Threads(context.Background())
	if err != nil || truncated {
		t.Fatalf("%v truncated=%v", err, truncated)
	}
	if len(threads) != 2 || threads[0].ID != "one" || threads[1].ID != "two" {
		t.Fatalf("incorrect session filtering: %#v", threads)
	}
	state.mu.Lock()
	params := state.params["thread/list"]
	state.mu.Unlock()
	var request struct {
		SourceKinds []string `json:"sourceKinds"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		t.Fatal(err)
	}
	for _, source := range request.SourceKinds {
		if strings.HasPrefix(source, "subAgent") {
			t.Fatal("requested child agents from server")
		}
	}
}
