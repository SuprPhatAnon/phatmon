// Package codex implements the local Codex app-server protocol. It never reads auth.json.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"phatmon/internal/config"
)

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

type Event struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}
type envelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

type Client struct {
	Home    config.Home
	Events  chan Event
	ctx     context.Context
	cancel  context.CancelFunc
	process *exec.Cmd
	stdin   io.WriteCloser
	socket  *websocket.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan envelope
	serial  atomic.Uint64
	alive   atomic.Bool
	once    sync.Once
	done    chan struct{}
}

func Connect(ctx context.Context, home config.Home, binary string) (*Client, error) {
	if err := home.Validate(); err != nil {
		return nil, err
	}
	info, err := os.Stat(home.Path)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("Codex home is not a directory: %s", home.Path)
	}
	life, cancel := context.WithCancel(ctx)
	c := &Client{Home: home, Events: make(chan Event, 256), ctx: life, cancel: cancel, pending: make(map[uint64]chan envelope), done: make(chan struct{})}
	if home.Endpoint != "" {
		dialCtx, end := context.WithTimeout(life, 10*time.Second)
		c.socket, _, err = websocket.Dial(dialCtx, home.Endpoint, nil)
		end()
		if err == nil {
			c.socket.SetReadLimit(32 << 20)
		}
	} else {
		c.process = exec.CommandContext(life, binary, "app-server", "--listen", "stdio://")
		c.process.Dir = home.Path
		c.process.Env = append(os.Environ(), "CODEX_HOME="+home.Path)
		c.process.WaitDelay = 2 * time.Second
		c.process.Cancel = func() error {
			if err := c.process.Process.Signal(os.Interrupt); err != nil {
				return c.process.Process.Kill()
			}
			return nil
		}
		c.stdin, err = c.process.StdinPipe()
		var output io.ReadCloser
		if err == nil {
			output, err = c.process.StdoutPipe()
		}
		// Keep server diagnostics out of the alternate terminal screen and avoid retaining secrets.
		c.process.Stderr = io.Discard
		if err == nil {
			err = c.process.Start()
		}
		if err == nil {
			go c.readStdio(output)
		}
	}
	if err != nil {
		cancel()
		return nil, err
	}
	c.alive.Store(true)
	if c.socket != nil {
		go c.readSocket()
	}
	var initialized struct {
		CodexHome string `json:"codexHome"`
	}
	err = c.Call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "phatmon", "title": "Phatmon", "version": "0.1.0"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}, &initialized)
	if err == nil && home.Endpoint != "" {
		actual, e := config.Expand(initialized.CodexHome)
		if e != nil || actual != home.Path {
			err = errors.New("endpoint CODEX_HOME does not match this registry entry (or server cannot report it)")
		}
	}
	if err == nil {
		err = c.send(ctx, map[string]any{"method": "initialized", "params": map[string]any{}})
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Connected() bool { return c.alive.Load() }

func (c *Client) dispatch(data []byte) error {
	var msg envelope
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("invalid app-server JSON: %w", err)
	}
	if msg.Method != "" {
		select {
		case c.Events <- Event{msg.ID, msg.Method, msg.Params}:
		case <-c.ctx.Done():
		}
	} else {
		var id uint64
		if json.Unmarshal(msg.ID, &id) != nil {
			return nil
		}
		c.mu.Lock()
		channel := c.pending[id]
		c.mu.Unlock()
		if channel != nil {
			select {
			case channel <- msg:
			default:
			}
		}
	}
	return nil
}

func (c *Client) readStdio(output io.Reader) {
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	var err error
	for scanner.Scan() {
		if err = c.dispatch(scanner.Bytes()); err != nil {
			break
		}
	}
	if err == nil {
		err = scanner.Err()
	}
	c.disconnected(err)
}
func (c *Client) readSocket() {
	var err error
	for {
		var data []byte
		_, data, err = c.socket.Read(c.ctx)
		if err != nil {
			break
		}
		if err = c.dispatch(data); err != nil {
			break
		}
	}
	c.disconnected(err)
}
func (c *Client) disconnected(err error) {
	c.alive.Store(false)
	message := "App server disconnected"
	if err != nil {
		message += ": " + err.Error()
	}
	params, _ := json.Marshal(map[string]string{"message": message})
	select {
	case c.Events <- Event{Method: "phatmon/disconnected", Params: params}:
	case <-c.ctx.Done():
	}
	c.cancel()
	close(c.done)
}

func (c *Client) send(ctx context.Context, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.socket != nil {
		return c.socket.Write(ctx, websocket.MessageText, data)
	}
	if pipe, ok := c.stdin.(interface{ SetWriteDeadline(time.Time) error }); ok {
		deadline, hasDeadline := ctx.Deadline()
		if !hasDeadline {
			deadline = time.Now().Add(25 * time.Second)
		}
		_ = pipe.SetWriteDeadline(deadline)
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	id := c.serial.Add(1)
	response := make(chan envelope, 1)
	c.mu.Lock()
	c.pending[id] = response
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if params == nil {
		params = map[string]any{}
	}
	if err := c.send(ctx, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case msg := <-response:
		if msg.Error != nil {
			return msg.Error
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(msg.Result, result)
	case <-ctx.Done():
		return fmt.Errorf("%s: %w; refresh before retrying a write", method, ctx.Err())
	case <-c.ctx.Done():
		return errors.New("app server disconnected")
	}
}
func (c *Client) Reply(ctx context.Context, id json.RawMessage, result any) error {
	return c.send(ctx, map[string]any{"id": id, "result": result})
}
func (c *Client) Reject(ctx context.Context, id json.RawMessage) error {
	return c.send(ctx, map[string]any{"id": id, "error": RPCError{-32601, "Phatmon does not support this request"}})
}
func (c *Client) Close() {
	c.once.Do(func() {
		c.alive.Store(false)
		c.cancel()
		if c.socket != nil {
			_ = c.socket.CloseNow()
		}
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.process != nil && c.process.Process != nil {
			_ = c.process.Wait()
		}
	})
}
