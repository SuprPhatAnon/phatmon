package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"phatmon/internal/config"
)

func (c *Client) storedThreads(ctx context.Context) ([]Thread, bool, error) {
	threads := []Thread{}
	cursor := ""
	seen := map[string]bool{}
	sources := []string{"cli", "vscode", "exec", "appServer", "unknown"}
	for range 20 {
		var page struct {
			Data       []Thread `json:"data"`
			NextCursor string   `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100, "sortKey": "updated_at", "modelProviders": []string{}, "sourceKinds": sources}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.Call(ctx, "thread/list", params, &page); err != nil {
			return nil, false, err
		}
		threads = append(threads, TopLevelThreads(page.Data)...)
		cursor = page.NextCursor
		if cursor == "" {
			return threads, false, nil
		}
		if seen[cursor] {
			return threads, true, nil
		}
		seen[cursor] = true
	}
	return threads, true, nil
}

// Threads lists top-level sessions, including loaded sessions without a persisted rollout.
func (c *Client) Threads(ctx context.Context) ([]Thread, bool, error) {
	threads, truncated, err := c.storedThreads(ctx)
	if err != nil {
		return nil, false, err
	}
	seen := map[string]bool{}
	for _, thread := range threads {
		seen[thread.ID] = true
	}
	cursor := ""
	cursors := map[string]bool{}
	for range 20 {
		var page struct {
			Data       []string `json:"data"`
			NextCursor string   `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		err = c.Call(ctx, "thread/loaded/list", params, &page)
		if err != nil {
			var rpcErr *RPCError
			if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
				return threads, truncated, nil
			}
			return nil, false, err
		}
		for _, id := range page.Data {
			if !seen[id] {
				thread, e := c.ReadThread(ctx, id)
				if e == nil {
					if !thread.IsChildAgent() {
						threads = append(threads, thread)
					}
					seen[id] = true
				}
			}
		}
		cursor = page.NextCursor
		if cursor == "" {
			return threads, truncated, nil
		}
		if cursors[cursor] {
			return threads, true, nil
		}
		cursors[cursor] = true
	}
	return threads, true, nil
}
func (c *Client) ReadThread(ctx context.Context, id string) (Thread, error) {
	var response struct {
		Thread Thread `json:"thread"`
	}
	err := c.Call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": false}, &response)
	return response.Thread, err
}
func (c *Client) Detail(ctx context.Context, id string) (Thread, bool, error) {
	thread, err := c.ReadThread(ctx, id)
	if err != nil {
		return thread, false, err
	}
	var page struct {
		Data       []Turn `json:"data"`
		NextCursor string `json:"nextCursor"`
	}
	err = c.Call(ctx, "thread/turns/list", map[string]any{"threadId": id, "limit": 20, "sortDirection": "desc", "itemsView": "full"}, &page)
	if err != nil {
		var rpcErr *RPCError
		if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
			return thread, false, err
		}
		var response struct {
			Thread Thread `json:"thread"`
		}
		err = c.Call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": true}, &response)
		return response.Thread, false, err
	}
	for i := len(page.Data) - 1; i >= 0; i-- {
		thread.Turns = append(thread.Turns, page.Data[i])
	}
	return thread, page.NextCursor != "", nil
}
func (c *Client) Resume(ctx context.Context, id string) (Thread, error) {
	var response struct {
		Thread Thread `json:"thread"`
	}
	err := c.Call(ctx, "thread/resume", map[string]any{"threadId": id}, &response)
	return response.Thread, err
}
func (c *Client) NewThread(ctx context.Context, cwd string) (Thread, error) {
	var response struct {
		Thread Thread `json:"thread"`
	}
	path, err := config.Expand(cwd)
	if err != nil {
		return response.Thread, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return response.Thread, errors.New("working directory does not exist")
	}
	err = c.Call(ctx, "thread/start", map[string]any{"cwd": path}, &response)
	return response.Thread, err
}
func (c *Client) Message(ctx context.Context, id, activeTurn, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("message is empty")
	}
	thread, err := c.ReadThread(ctx, id)
	if err != nil {
		return "", err
	}
	if thread.CanAcceptDirectInput != nil && !*thread.CanAcceptDirectInput {
		return "", errors.New("thread does not accept direct input; message its parent")
	}
	params := map[string]any{"threadId": id, "input": []map[string]string{{"type": "text", "text": text}}}
	if thread.Status.Type == "active" {
		if activeTurn == "" {
			return "", errors.New("active turn ID unavailable; refresh details before steering")
		}
		params["expectedTurnId"] = activeTurn
		err = c.Call(ctx, "turn/steer", params, nil)
		return activeTurn, err
	}
	var response struct {
		Turn Turn `json:"turn"`
	}
	err = c.Call(ctx, "turn/start", params, &response)
	return response.Turn.ID, err
}
func (c *Client) Interrupt(ctx context.Context, id, turn string) error {
	if turn == "" {
		return errors.New("no active turn")
	}
	return c.Call(ctx, "turn/interrupt", map[string]any{"threadId": id, "turnId": turn}, nil)
}
func (c *Client) Quotas(ctx context.Context) (Quotas, error) {
	var response Quotas
	err := c.Call(ctx, "account/rateLimits/read", nil, &response)
	return response, err
}
func (c *Client) Skills(ctx context.Context, cwd string) ([]Skill, error) {
	var response struct {
		Data []struct {
			Skills []Skill `json:"skills"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := c.Call(ctx, "skills/list", map[string]any{"cwds": []string{cwd}, "forceReload": true}, &response); err != nil {
		return nil, err
	}
	var skills []Skill
	var messages []string
	for _, entry := range response.Data {
		skills = append(skills, entry.Skills...)
		for _, e := range entry.Errors {
			messages = append(messages, e.Message)
		}
	}
	if len(messages) > 0 {
		return skills, fmt.Errorf("skill discovery: %s", strings.Join(messages, "; "))
	}
	return skills, nil
}
func (c *Client) ToggleSkill(ctx context.Context, skill Skill) error {
	return c.Call(ctx, "skills/config/write", map[string]any{"path": skill.Path, "enabled": !skill.Enabled}, nil)
}
func (c *Client) MCP(ctx context.Context) (MCPConfig, error) {
	var response struct {
		Layers []struct {
			Name struct {
				Type    string `json:"type"`
				File    string `json:"file"`
				Profile string `json:"profile"`
			} `json:"name"`
			Version string `json:"version"`
			Config  struct {
				Servers map[string]map[string]any `json:"mcp_servers"`
			} `json:"config"`
		} `json:"layers"`
	}
	err := c.Call(ctx, "config/read", map[string]bool{"includeLayers": true}, &response)
	if err != nil {
		return MCPConfig{}, err
	}
	expected := filepath.Join(c.Home.Path, "config.toml")
	for _, layer := range response.Layers {
		path, _ := config.Expand(layer.Name.File)
		if layer.Name.Type == "user" && layer.Name.Profile == "" && path == expected {
			if layer.Config.Servers == nil {
				layer.Config.Servers = map[string]map[string]any{}
			}
			return MCPConfig{Servers: layer.Config.Servers, Version: layer.Version, File: path}, nil
		}
	}
	return MCPConfig{}, errors.New("user configuration layer unavailable; MCP edits disabled")
}
func (c *Client) WriteMCP(ctx context.Context, config MCPConfig) error {
	if config.Version == "" {
		return errors.New("config version is required")
	}
	if err := c.Call(ctx, "config/value/write", map[string]any{"keyPath": "mcp_servers", "value": config.Servers, "mergeStrategy": "replace", "expectedVersion": config.Version, "filePath": config.File}, nil); err != nil {
		return err
	}
	if err := c.Call(ctx, "config/mcpServer/reload", nil, nil); err != nil {
		return fmt.Errorf("configuration saved, but runtime reload failed: %w", err)
	}
	return nil
}
