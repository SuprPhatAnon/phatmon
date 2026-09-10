package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"phatmon/internal/config"
	"phatmon/internal/tui"
)

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ", ") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }
func run() error {
	var homeArgs, endpointArgs repeated
	var registry, binary string
	var demo, version, initConfig, listHomes bool
	flag.Var(&homeArgs, "home", "Codex home NAME=PATH (repeatable; overrides saved registry)")
	flag.Var(&endpointArgs, "connect", "Shared local app server NAME=ws://127.0.0.1:PORT (repeatable)")
	flag.StringVar(&registry, "config", config.DefaultPath(), "Phatmon configuration file (created if missing)")
	flag.StringVar(&binary, "codex", "", "Override configured Codex executable")
	flag.BoolVar(&listHomes, "list-homes", false, "Print resolved homes without starting Codex or the TUI")
	flag.BoolVar(&initConfig, "init-config", false, "Create configuration if missing, print its path, and exit")
	flag.BoolVar(&demo, "demo", false, "Show synthetic sessions without account or process access")
	flag.BoolVar(&version, "version", false, "Print version")
	flag.Parse()
	if version {
		fmt.Println("phatmon 0.1.0")
		return nil
	}
	registry, err := config.Expand(registry)
	if err != nil {
		return err
	}
	conf, err := config.Load(registry)
	if err != nil {
		return err
	}
	if initConfig {
		fmt.Println(registry)
		return nil
	}
	if binary != "" {
		conf.Settings.CodexBinary = binary
	}
	var homes []config.Home
	if !demo {
		if len(homeArgs) > 0 {
			for _, entry := range homeArgs {
				name, path, ok := strings.Cut(entry, "=")
				if !ok {
					return fmt.Errorf("--home requires NAME=PATH")
				}
				homes = append(homes, config.Home{Name: name, Path: path})
			}
		} else {
			homes, err = config.ResolveHomes(conf)
			if err != nil {
				return err
			}
		}
		for _, entry := range endpointArgs {
			name, url, ok := strings.Cut(entry, "=")
			if !ok {
				return fmt.Errorf("--connect requires NAME=URL")
			}
			found := false
			for i := range homes {
				if homes[i].Name == name {
					homes[i].Endpoint = url
					found = true
				}
			}
			if !found {
				return fmt.Errorf("--connect references unknown home %q", name)
			}
		}
		if err = config.Validate(homes); err != nil {
			return err
		}
	}
	if listHomes {
		data, err := json.MarshalIndent(homes, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	model := tui.New(homes, registry, conf.Settings, demo)
	defer model.Close()
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "phatmon:", err)
		os.Exit(1)
	}
}
