package config

import (
	"os"
	"path/filepath"
	"testing"
)

func discoveryFixture(t *testing.T) (string, Config) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", "")
	conf, err := Defaults()
	if err != nil {
		t.Fatal(err)
	}
	return root, conf
}
func makeDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverDefaultAndDirectoryChildren(t *testing.T) {
	root, conf := discoveryFixture(t)
	makeDir(t, filepath.Join(root, ".codex"))
	makeDir(t, filepath.Join(root, ".codex-homes", "consumable", "sessions"))
	makeDir(t, filepath.Join(root, ".codex-homes", "other"))
	os.WriteFile(filepath.Join(root, ".codex-homes", "notes.txt"), []byte("ignored"), 0600)
	homes, err := ResolveHomes(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(homes) != 3 || homes[0].Name != "personal" || homes[1].Name != "consumable" || homes[2].Name != "other" {
		t.Fatalf("wrong discovery: %#v", homes)
	}
	if homes[1].Path != filepath.Join(root, ".codex-homes", "consumable") {
		t.Fatal("must use child directory itself as CODEX_HOME")
	}
	for _, home := range homes {
		if !home.Discovered {
			t.Fatal("discovery provenance missing")
		}
	}
}

func TestConfiguredOverridesAndSymlinkDeduplication(t *testing.T) {
	root, conf := discoveryFixture(t)
	personal := filepath.Join(root, ".codex")
	work := filepath.Join(root, ".codex-homes", "consumable")
	makeDir(t, personal)
	makeDir(t, work)
	if err := os.Symlink(work, filepath.Join(root, ".codex-homes", "alias")); err != nil {
		t.Fatal(err)
	}
	os.Symlink(personal, filepath.Join(root, ".codex-homes", "personal"))
	os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, ".codex-homes", "broken"))
	conf.Homes = []Home{{Name: "work", Path: work, Endpoint: "ws://127.0.0.1:4500"}}
	homes, err := ResolveHomes(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(homes) != 2 || homes[0].Name != "work" || homes[0].Endpoint == "" || homes[0].Discovered {
		t.Fatalf("lost override / duplicate path: %#v", homes)
	}
}

func TestDiscoveryTracksDirectoryChangesWithoutPersistingThem(t *testing.T) {
	root, conf := discoveryFixture(t)
	path := filepath.Join(root, "config", "config.json")
	if err := Save(path, conf); err != nil {
		t.Fatal(err)
	}
	makeDir(t, filepath.Join(root, ".codex-homes", "work"))
	homes, err := ResolveHomes(conf)
	if err != nil {
		t.Fatal(err)
	}
	homes = append(homes, Home{Name: "manual", Path: filepath.Join(root, "outside")})
	if err := SaveHomes(path, homes); err != nil {
		t.Fatal(err)
	}
	saved, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Homes) != 1 || saved.Homes[0].Name != "manual" {
		t.Fatalf("persisted discovered homes: %#v", saved.Homes)
	}
	os.Remove(filepath.Join(root, ".codex-homes", "work"))
	makeDir(t, filepath.Join(root, ".codex-homes", "new"))
	found, err := ResolveHomes(saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 3 || found[2].Name != "new" {
		t.Fatalf("stale discovery: %#v", found)
	}
}

func TestDiscoveryMissingRootAndNameCollisions(t *testing.T) {
	root, conf := discoveryFixture(t)
	homes, err := ResolveHomes(conf)
	if err != nil || len(homes) != 1 {
		t.Fatalf("missing discovery root: %#v %v", homes, err)
	}
	makeDir(t, filepath.Join(root, ".codex-homes", "personal"))
	makeDir(t, filepath.Join(root, ".codex-homes", "work"))
	conf.Homes = []Home{{Name: "work", Path: filepath.Join(root, "elsewhere")}}
	homes, err = ResolveHomes(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(homes) != 4 || homes[2].Name != "personal-2" || homes[3].Name != "work-2" {
		t.Fatalf("unstable collisions: %#v", homes)
	}
}

func TestCustomDiscoveryDirectory(t *testing.T) {
	root, conf := discoveryFixture(t)
	conf.Settings.HomesDirectory = filepath.Join(root, "custom")
	makeDir(t, filepath.Join(root, "custom", "lab"))
	homes, err := ResolveHomes(conf)
	if err != nil || len(homes) != 2 || homes[1].Name != "lab" {
		t.Fatalf("custom root: %#v %v", homes, err)
	}
}
