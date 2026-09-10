package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"phatmon/internal/config"
	"phatmon/internal/install"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var opts install.Options
	flag.StringVar(&opts.Binary, "binary", "bin/phatmon", "Built Phatmon executable")
	flag.StringVar(&opts.Codex, "codex", "codex", "Codex executable")
	flag.StringVar(&opts.BinDir, "bin-dir", filepath.Join(home, "bin"), "Executable destination directory")
	flag.StringVar(&opts.UnitDir, "unit-dir", filepath.Join(configDir, "systemd", "user"), "User systemd unit directory")
	flag.StringVar(&opts.ConfigPath, "config", config.DefaultPath(), "Phatmon configuration to update")
	flag.StringVar(&opts.WorkName, "work-name", "consumable", "Discovered/configured work home name")
	flag.StringVar(&opts.WorkAddr, "work-addr", "ws://127.0.0.1:4500", "Work server address")
	flag.StringVar(&opts.PersonalAddr, "personal-addr", "ws://127.0.0.1:4501", "Personal server address")
	flag.BoolVar(&opts.NoStart, "no-start", false, "Install files only; skip all systemctl commands")
	flag.BoolVar(&opts.NoHomeEnv, "no-home-env", false, "Skip per-home environment files (for staged installs)")
	flag.StringVar(&opts.EndpointEnv, "endpoint-env", "CODEX_REMOTE", "Environment variable for the app-server address")
	flag.Parse()
	if err := install.Run(opts, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "install:", err)
		os.Exit(1)
	}
}
