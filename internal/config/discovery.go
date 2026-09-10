package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ResolveHomes adds the default home and immediate directory children of the
// discovery root to configured homes. It reads directory metadata only, never
// credentials. Explicit names and endpoints win when paths refer to the same home.
func ResolveHomes(conf Config) ([]Home, error) {
	homes := append([]Home{}, conf.Homes...)
	if err := Validate(homes); err != nil {
		return nil, err
	}
	names, paths := map[string]bool{}, map[string]bool{}
	for _, home := range homes {
		names[home.Name] = true
		paths[home.Path] = true
	}
	add := func(name, path string) error {
		resolved, err := Expand(path)
		if err != nil {
			return err
		}
		if paths[resolved] {
			return nil
		}
		base := discoveryName(name)
		name = base
		for suffix := 2; names[name]; suffix++ {
			name = fmt.Sprintf("%s-%d", base, suffix)
		}
		home := Home{Name: name, Path: resolved, Discovered: true}
		if err := home.Validate(); err != nil {
			return err
		}
		homes = append(homes, home)
		names[name] = true
		paths[resolved] = true
		return nil
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if err := add("personal", filepath.Join(user, ".codex")); err != nil {
		return nil, err
	}
	if environment := os.Getenv("CODEX_HOME"); environment != "" {
		if err := add("environment", environment); err != nil {
			return nil, err
		}
	}
	root, err := Expand(conf.Settings.HomesDirectory)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return homes, nil
	}
	if err != nil {
		return nil, fmt.Errorf("discover Codex homes in %s: %w", root, err)
	}
	// os.ReadDir sorts by name, making both order and collision suffixes stable.
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := os.Stat(path) // Follow directory symlinks; Expand deduplicates their targets.
		if os.IsNotExist(err) {
			continue
		} // Broken symlinks or an entry removed during the scan.
		if err != nil {
			return nil, fmt.Errorf("discover Codex home %s: %w", path, err)
		}
		if !info.IsDir() {
			continue
		}
		if err := add(entry.Name(), path); err != nil {
			return nil, err
		}
	}
	return homes, nil
}

func discoveryName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' {
			return '-'
		}
		return r
	}, name))
	for len(name) > 64 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		return "home"
	}
	return name
}
