package install

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"phatmon/internal/config"
)

func fixture(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", "")
	opts := Options{
		Binary: filepath.Join(root, "build", "phatmon"), Codex: filepath.Join(root, "tools", "codex"),
		BinDir: filepath.Join(root, "bin"), UnitDir: filepath.Join(root, "units"),
		ConfigPath: filepath.Join(root, "config", "config.json"),
		WorkName:   "consumable", WorkAddr: "ws://127.0.0.1:4500", PersonalAddr: "ws://127.0.0.1:4501", NoStart: true,
	}
	for _, dir := range []string{filepath.Join(root, ".codex"), filepath.Join(root, ".codex-homes", "consumable")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{opts.Binary, opts.Codex} {
		if err := writeFile(file, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return opts
}

func TestInstallPreservesConfigAndCanUpdate(t *testing.T) {
	opts := fixture(t)
	conf, err := config.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	conf.Settings.RefreshIntervalSeconds = 17
	conf.Homes = []config.Home{{Name: "lab", Path: t.TempDir(), Endpoint: "ws://127.0.0.1:4700"}}
	if err := config.Save(opts.ConfigPath, conf); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Run(opts, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := config.Load(opts.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	conf.Settings.CodexBinary = opts.Codex
	if !reflect.DeepEqual(saved.Settings, conf.Settings) || len(saved.Homes) != 3 || saved.Homes[0] != conf.Homes[0] {
		t.Fatalf("settings/unrelated homes lost or duplicate homes added: %#v", saved)
	}
	for _, h := range saved.Homes {
		addr := map[string]string{"consumable": opts.WorkAddr, "personal": opts.PersonalAddr, "lab": "ws://127.0.0.1:4700"}[h.Name]
		if h.Endpoint != addr {
			t.Fatalf("endpoint not saved: %#v", saved.Homes)
		}
		unit, err := os.ReadFile(filepath.Join(opts.UnitDir, serviceName(h, opts)))
		if err != nil || !strings.Contains(string(unit), addr) || !strings.Contains(string(unit), h.Path) {
			t.Fatalf("wrong unit: %s, %v", unit, err)
		}
	}
	installed := filepath.Join(opts.BinDir, "phatmon")
	info, err := os.Stat(installed)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("executable permissions: %v, %v", info, err)
	}
	want, _ := os.ReadFile(opts.Binary)
	got, _ := os.ReadFile(installed)
	if !bytes.Equal(got, want) {
		t.Fatal("installed binary differs")
	}
}

func TestInstallRejectsInvalidServersBeforeInstallingFiles(t *testing.T) {
	for _, addr := range []string{"ws://127.0.0.1:4501", "ws://0.0.0.0:4500", "ws://127.0.0.1:0", "ws://127.0.0.1:70000", "ws://127.0.0.1:4500/path"} {
		t.Run(addr, func(t *testing.T) {
			opts := fixture(t)
			opts.WorkAddr = addr
			if err := Run(opts, &bytes.Buffer{}); err == nil {
				t.Fatal("invalid/conflicting endpoint accepted")
			}
			if _, err := os.Stat(opts.BinDir); !os.IsNotExist(err) {
				t.Fatal("installed binary before rejecting endpoint")
			}
		})
	}
}

func TestServiceEscaping(t *testing.T) {
	home := config.Home{Path: "/tmp/home %h $HOME \"quoted\"", Endpoint: "ws://127.0.0.1:4500"}
	unit, err := renderUnit(home, "/tmp/a $bin %h/codex", "/tmp/tools:/usr/bin")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`WorkingDirectory=%h`, `Environment="CODEX_HOME=/tmp/home %%h $HOME \"quoted\""`, `ExecStart="/tmp/a $$bin %%h/codex"`, `Environment="PATH=/tmp/tools:/usr/bin"`} {
		if !strings.Contains(unit, want) {
			t.Fatalf("missing %s in unit:\n%s", want, unit)
		}
	}
	if _, err := renderUnit(home, "/bin/codex\nExecStart=/bin/false", ""); err == nil {
		t.Fatal("newline accepted")
	}
}

func TestInstallEnablesBothWithoutRestartingExistingWork(t *testing.T) {
	opts := fixture(t)
	opts.NoStart = false
	log := filepath.Join(t.TempDir(), "commands")
	t.Setenv("PHATMON_INSTALL_TEST_LOG", log)
	stub := filepath.Join(t.TempDir(), "systemctl")
	if err := writeFile(stub, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PHATMON_INSTALL_TEST_LOG\"\nif [ \"$2\" = show-environment ]; then printf 'PRIVATE=value\\n'; fi\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(stub)+":"+os.Getenv("PATH"))
	var output bytes.Buffer
	if err := Run(opts, &output); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(log)
	want := "--user show-environment\n--user daemon-reload\n--user enable --now phatmon-codex-personal.service phatmon-codex-work.service\n"
	if string(got) != want || strings.Contains(output.String(), "PRIVATE") {
		t.Fatalf("incorrect systemctl calls/output: %s / %s", got, output.String())
	}
}

func TestAllDiscoveredHomesGetStableMatchingServices(t *testing.T) {
	opts := fixture(t)
	root := filepath.Join(os.Getenv("HOME"), ".codex-homes")
	for _, name := range []string{"lab one", "lab-one"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "lab one"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := Run(opts, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	first, err := config.Load(opts.ConfigPath)
	if err != nil || len(first.Homes) != 4 {
		t.Fatalf("missing or duplicate homes: %#v, %v", first, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "aaa-new"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Run(opts, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	second, err := config.Load(opts.ConfigPath)
	if err != nil || len(second.Homes) != 5 {
		t.Fatalf("new home not installed: %#v, %v", second, err)
	}
	seen := map[string]bool{}
	for _, h := range second.Homes {
		for _, previous := range first.Homes {
			if h.Path == previous.Path && h.Endpoint != previous.Endpoint {
				t.Fatalf("existing endpoint changed: %s", h.Name)
			}
		}
		unitName := serviceName(h, opts)
		if seen[unitName] {
			t.Fatalf("unit name collision: %s", unitName)
		}
		seen[unitName] = true
		unit, err := os.ReadFile(filepath.Join(opts.UnitDir, unitName))
		if err != nil || !strings.Contains(string(unit), "CODEX_HOME="+h.Path) || !strings.Contains(string(unit), h.Endpoint) {
			t.Fatalf("home points to wrong service: %#v, %s, %v", h, unit, err)
		}
	}
}

func TestInstallDoesNotRequireConsumableHome(t *testing.T) {
	opts := fixture(t)
	if err := os.Remove(filepath.Join(os.Getenv("HOME"), ".codex-homes", "consumable")); err != nil {
		t.Fatal(err)
	}
	if err := Run(opts, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	conf, err := config.Load(opts.ConfigPath)
	if err != nil || len(conf.Homes) != 1 || conf.Homes[0].Endpoint != opts.PersonalAddr {
		t.Fatalf("default home installation failed: %#v, %v", conf, err)
	}
}
