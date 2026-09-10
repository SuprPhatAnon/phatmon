// Package config owns Phatmon settings, separate from Codex configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Home struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Endpoint   string `json:"endpoint,omitempty"`
	Discovered bool   `json:"-"`
}

type Settings struct {
	HomesDirectory              string `json:"homes_directory"`
	CodexBinary                 string `json:"codex_binary"`
	RefreshIntervalSeconds      int    `json:"refresh_interval_seconds"`
	QuotaRefreshIntervalSeconds int    `json:"quota_refresh_interval_seconds"`
	LiveOnly                    bool   `json:"live_only"`
}

type Config struct {
	Version  int      `json:"version"`
	Homes    []Home   `json:"homes"`
	Settings Settings `json:"settings"`
}

func DefaultSettings() Settings {
	return Settings{HomesDirectory: "~/.codex-homes", CodexBinary: "codex", RefreshIntervalSeconds: 5, QuotaRefreshIntervalSeconds: 60}
}

func (s Settings) RefreshInterval() time.Duration {
	return time.Duration(s.RefreshIntervalSeconds) * time.Second
}
func (s Settings) QuotaRefreshInterval() time.Duration {
	return time.Duration(s.QuotaRefreshIntervalSeconds) * time.Second
}

func (c *Config) Validate() error {
	if c.Version != 1 {
		return errors.New("unsupported config version (expected 1)")
	}
	if strings.TrimSpace(c.Settings.HomesDirectory) == "" {
		return errors.New("settings.homes_directory must not be empty")
	}
	if strings.TrimSpace(c.Settings.CodexBinary) == "" {
		return errors.New("settings.codex_binary must not be empty")
	}
	if c.Settings.RefreshIntervalSeconds < 1 || c.Settings.RefreshIntervalSeconds > 86400 {
		return errors.New("settings.refresh_interval_seconds must be between 1 and 86400")
	}
	if c.Settings.QuotaRefreshIntervalSeconds < 1 || c.Settings.QuotaRefreshIntervalSeconds > 86400 {
		return errors.New("settings.quota_refresh_interval_seconds must be between 1 and 86400")
	}
	return Validate(c.Homes)
}

func Defaults() (Config, error) {
	conf := Config{Version: 1, Homes: []Home{}, Settings: DefaultSettings()}
	return conf, conf.Validate()
}

func Expand(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	if path == "" {
		return "", errors.New("path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	return absolute, nil
}

func (h *Home) Validate() error {
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" || len(h.Name) > 80 || strings.ContainsAny(h.Name, "/\n\r\t\x1b") {
		return errors.New("home name must be 1–80 printable characters")
	}
	var err error
	h.Path, err = Expand(h.Path)
	if err != nil {
		return err
	}
	if h.Endpoint != "" {
		u, err := url.Parse(h.Endpoint)
		if err != nil {
			return err
		}
		if u.Scheme != "ws" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("endpoint must be ws://127.0.0.1:PORT (loopback only, no credentials)")
		}
	}
	return nil
}

func Validate(homes []Home) error {
	names, paths, endpoints := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range homes {
		if err := homes[i].Validate(); err != nil {
			return err
		}
		h := homes[i]
		if names[h.Name] || paths[h.Path] || (h.Endpoint != "" && endpoints[h.Endpoint]) {
			return errors.New("home names, paths, and endpoints must be unique")
		}
		names[h.Name], paths[h.Path] = true, true
		if h.Endpoint != "" {
			endpoints[h.Endpoint] = true
		}
	}
	return nil
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "phatmon", "config.json")
}

func decode(data []byte) (Config, error) {
	conf := Config{Settings: DefaultSettings()}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&conf); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, errors.New("config must contain exactly one JSON object")
	}
	return conf, conf.Validate()
}

// Load creates a missing configuration. Legacy homes.json is copied into config.json
// with default settings; the original file is left intact. Existing files are never
// overwritten on startup, including when two instances start at the same time.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return decode(data)
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}
	// A dangling symlink is an existing configuration entry, not a missing file.
	if _, err := os.Lstat(path); err == nil {
		// Another startup may have just published the file.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return Config{}, readErr
		}
		return decode(data)
	} else if !os.IsNotExist(err) {
		return Config{}, err
	}
	conf, err := Defaults()
	if err != nil {
		return Config{}, err
	}
	if filepath.Base(path) == "config.json" {
		legacy := filepath.Join(filepath.Dir(path), "homes.json")
		if data, readErr := os.ReadFile(legacy); readErr == nil {
			conf, err = decode(data)
			if err != nil {
				return Config{}, fmt.Errorf("migrate %s: %w", legacy, err)
			}
		} else if !os.IsNotExist(readErr) {
			return Config{}, readErr
		}
	}
	if err = write(path, conf, false); os.IsExist(err) {
		return Load(path)
	} else if err != nil {
		return Config{}, err
	}
	return conf, nil
}

// SaveHomes preserves on-disk settings and persists only explicitly registered homes.
func SaveHomes(path string, homes []Home) error {
	conf, err := Load(path)
	if err != nil {
		return err
	}
	conf.Homes = []Home{}
	for _, home := range homes {
		if !home.Discovered {
			conf.Homes = append(conf.Homes, home)
		}
	}
	return Save(path, conf)
}

// Save atomically writes owner-only configuration after validation.
func Save(path string, conf Config) error { return write(path, conf, true) }

func write(path string, conf Config, replace bool) error {
	if err := conf.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if replace {
		return os.Rename(file.Name(), path)
	}
	return os.Link(file.Name(), path)
}
