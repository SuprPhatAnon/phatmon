package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryRoundTripAndPermissions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "homes.json")
	homes := []Home{{Name: "personal", Path: root}, {Name: "work", Path: filepath.Join(root, "work"), Endpoint: "ws://127.0.0.1:4500"}}
	if err := Save(path, Config{Version: 1, Homes: homes, Settings: DefaultSettings()}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Homes) != 2 || got.Homes[1].Endpoint != homes[1].Endpoint {
		t.Fatalf("bad roundtrip: %#v", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("registry permissions = %o", info.Mode().Perm())
	}
}
func TestRejectDuplicateAndRemoteHomes(t *testing.T) {
	for _, homes := range [][]Home{
		{{Name: "same", Path: "/a"}, {Name: "same", Path: "/b"}},
		{{Name: "a", Path: "/same"}, {Name: "b", Path: "/same"}},
		{{Name: "a", Path: "/a", Endpoint: "ws://example.com:4500"}},
		{{Name: "a", Path: "/a", Endpoint: "ws://token@127.0.0.1:4500"}},
		{{Name: "a", Path: "/a", Endpoint: "ws://127.0.0.1:4500?token=secret"}},
		{{Name: "a/b", Path: "/a"}},
	} {
		if Validate(homes) == nil {
			t.Fatalf("accepted invalid homes: %#v", homes)
		}
	}
}
func TestDefaultRespectsCodexHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(root, "environment"))
	conf, err := Load(filepath.Join(root, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	homes, err := ResolveHomes(conf)
	if err != nil || len(homes) != 2 || homes[0].Path != filepath.Join(root, ".codex") || homes[1].Path != filepath.Join(root, "environment") {
		t.Fatalf("%#v %v", homes, err)
	}
}

func TestStartupCreatesPrivateConfigAndPreservesEdits(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex-home"))
	path := filepath.Join(root, "phatmon", "config.json")
	conf, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.Settings != DefaultSettings() {
		t.Fatalf("unexpected defaults: %#v", conf.Settings)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file: %v %v", info, err)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || dir.Mode().Perm() != 0700 {
		t.Fatalf("directory: %v %v", dir, err)
	}
	conf.Settings.CodexBinary = "/opt/codex"
	conf.Settings.RefreshIntervalSeconds = 12
	conf.Settings.QuotaRefreshIntervalSeconds = 180
	conf.Settings.LiveOnly = true
	if err := Save(path, conf); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || got.Settings != conf.Settings {
		t.Fatal("startup overwrote existing settings")
	}
	if err := SaveHomes(path, []Home{{Name: "work", Path: root}}); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got.Settings != conf.Settings || len(got.Homes) != 1 || got.Homes[0].Name != "work" {
		t.Fatalf("home edit lost settings: %#v %v", got, err)
	}
}

func TestLegacyMigration(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "homes.json")
	data := []byte(`{"version":1,"homes":[{"name":"work","path":"/tmp/codex-work"}]}`)
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	conf, err := Load(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if conf.Homes[0].Name != "work" || conf.Settings != DefaultSettings() {
		t.Fatalf("bad migration: %#v", conf)
	}
	original, _ := os.ReadFile(legacy)
	if string(original) != string(data) {
		t.Fatal("legacy registry changed")
	}
}

func TestRejectBadSettingsWithoutChangingFile(t *testing.T) {
	for _, settings := range []string{
		`{"refresh_interval_seconds":0}`,
		`{"quota_refresh_interval_seconds":-1}`,
		`{"codex_binary":""}`,
		`{"quota_refresh_interval_seconds":86401}`,
		`{"refresh_intervl_seconds":10}`,
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		content := []byte(`{"version":1,"homes":[],"settings":` + settings + `}`)
		os.WriteFile(path, content, 0600)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted invalid settings %s", settings)
		}
		got, _ := os.ReadFile(path)
		if string(got) != string(content) {
			t.Fatal("invalid file was overwritten")
		}
	}
}

func TestConcurrentFirstStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	results := make(chan error, 8)
	for range 8 {
		go func() { _, err := Load(path); results <- err }()
	}
	for range 8 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}
