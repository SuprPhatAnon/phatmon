// Package install installs the executable and per-home user systemd services.
package install

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"phatmon/internal/config"
)

type Options struct {
	Binary, Codex, BinDir, UnitDir, ConfigPath string
	WorkName, WorkAddr, PersonalAddr           string
	NoStart                                    bool
	NoHomeEnv                                  bool
	EndpointEnv                                string
}

func Run(opts Options, out io.Writer) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("user systemd installation requires Linux")
	}
	for _, path := range []*string{&opts.Binary, &opts.BinDir, &opts.UnitDir, &opts.ConfigPath} {
		var err error
		*path, err = config.Expand(*path)
		if err != nil {
			return err
		}
	}
	conf, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	codex, err := exec.LookPath(opts.Codex)
	if err != nil {
		return fmt.Errorf("find Codex: %w", err)
	}
	codex, err = filepath.Abs(codex)
	if err != nil {
		return err
	}
	if legacyLauncher(codex) {
		codex = conf.Settings.CodexBinary
		if !filepath.IsAbs(codex) || legacyLauncher(codex) {
			return fmt.Errorf("set CODEX to the original Codex executable to replace a legacy Phatmon launcher")
		}
		if _, err := exec.LookPath(codex); err != nil {
			return err
		}
	}
	conf.Settings.CodexBinary = codex
	binary, err := os.ReadFile(opts.Binary)
	if err != nil {
		return err
	}
	if len(binary) == 0 {
		return fmt.Errorf("Phatmon binary is empty")
	}
	var systemctl string
	if !opts.NoStart {
		systemctl, err = exec.LookPath("systemctl")
		if err != nil {
			return err
		}
		if err := command(systemctl, out, "--user", "show-environment"); err != nil {
			return fmt.Errorf("user systemd manager unavailable; use --no-start to install files only: %w", err)
		}
	}
	homes, err := config.ResolveHomes(conf)
	if err != nil {
		return err
	}
	selected, err := assignEndpoints(homes, opts)
	if err != nil {
		return err
	}
	envFiles, err := prepareEnvironment(opts, selected)
	if err != nil {
		return err
	}
	services := make([]string, len(selected))
	units := make([][]byte, len(selected))
	for i, h := range selected {
		services[i] = serviceName(h, opts)
		unit, err := renderUnit(h, codex, os.Getenv("PATH"))
		if err != nil {
			return err
		}
		units[i] = []byte(unit)
		updated := false
		for j := range conf.Homes {
			if conf.Homes[j].Name == h.Name {
				conf.Homes[j] = h
				updated = true
			}
		}
		if !updated {
			conf.Homes = append(conf.Homes, h)
		}
	}
	if err := conf.Validate(); err != nil {
		return err
	}
	for i, unit := range units {
		if err := writeFile(filepath.Join(opts.UnitDir, services[i]), unit, 0644); err != nil {
			return err
		}
	}
	target := filepath.Join(opts.BinDir, "phatmon")
	if err := writeFile(target, binary, 0755); err != nil {
		return err
	}
	if err := config.Save(opts.ConfigPath, conf); err != nil {
		return err
	}
	for _, file := range envFiles {
		if err := writeFile(file.path, file.data, file.mode); err != nil {
			return err
		}
	}
	if err := removeLegacyClients(opts, selected); err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s\nConfigured %d endpoints in %s\nUser units: %s\n", target, len(selected), opts.ConfigPath, opts.UnitDir)
	for i, h := range selected {
		fmt.Fprintf(out, "  %s: %s (%s)\n", h.Name, h.Endpoint, services[i])
	}
	if !opts.NoHomeEnv {
		fmt.Fprintln(out, "Wrote phatmon.env in each home for direnv or shell sourcing.")
	}
	if opts.NoStart {
		fmt.Fprintln(out, "Files installed; services were not enabled or started.")
		return nil
	}
	if err := command(systemctl, out, "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := command(systemctl, out, append([]string{"--user", "enable", "--now"}, services...)...); err != nil {
		return fmt.Errorf("files installed, but enabling/starting services failed: %w", err)
	}
	fmt.Fprintln(out, "All services enabled and started. Already-running services are left running; restart them after changing their settings.")
	return nil
}

// Reserve all saved endpoints before allocating new ones, so adding homes does
// not change ports assigned by an earlier installation.
func assignEndpoints(homes []config.Home, opts Options) ([]config.Home, error) {
	selected := append([]config.Home(nil), homes...)
	used := map[int]string{}
	for i := range selected {
		h := &selected[i]
		info, err := os.Stat(h.Path)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("Codex home is not a directory: %s", h.Path)
		}
		switch h.Name {
		case "personal":
			h.Endpoint = opts.PersonalAddr
		case opts.WorkName:
			h.Endpoint = opts.WorkAddr
		}
		h.Discovered = false
		if h.Endpoint == "" {
			continue
		}
		if err := h.Validate(); err != nil {
			return nil, err
		}
		u, err := url.Parse(h.Endpoint)
		if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Path != "" {
			return nil, fmt.Errorf("home %q: server address must be ws://127.0.0.1:PORT or ws://[::1]:PORT", h.Name)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("home %q: server address needs a port between 1 and 65535", h.Name)
		}
		if other, ok := used[port]; ok {
			return nil, fmt.Errorf("homes %q and %q use the same server port %d", other, h.Name, port)
		}
		used[port] = h.Name
	}
	port := 4502
	for i := range selected {
		if selected[i].Endpoint != "" {
			continue
		}
		for used[port] != "" && port <= 65535 {
			port++
		}
		if port > 65535 {
			return nil, fmt.Errorf("no free server ports in the installation configuration")
		}
		selected[i].Endpoint = fmt.Sprintf("ws://127.0.0.1:%d", port)
		used[port] = selected[i].Name
	}
	return selected, config.Validate(selected)
}

func serviceName(home config.Home, opts Options) string {
	if home.Name == "personal" {
		return "phatmon-codex-personal.service"
	}
	if home.Name == opts.WorkName {
		return "phatmon-codex-work.service"
	}
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, home.Name)
	// The path hash disambiguates names that sanitize to the same unit name.
	hash := sha256.Sum256([]byte(home.Path))
	return fmt.Sprintf("phatmon-codex-%s-%x.service", name, hash[:4])
}

func command(binary string, out io.Writer, args ...string) error {
	cmd := exec.Command(binary, args...)
	// show-environment is only a manager availability probe; never print its data.
	if len(args) < 2 || args[1] != "show-environment" {
		cmd.Stdout = out
	}
	cmd.Stderr = out
	return cmd.Run()
}

func renderUnit(home config.Home, codex, path string) (string, error) {
	for _, value := range []string{home.Path, home.Endpoint, codex, path} {
		if strings.ContainsAny(value, "\x00\r\n") {
			return "", fmt.Errorf("service paths and environment must not contain NUL or newlines")
		}
	}
	quote := func(value string) string {
		value = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%", "\t", "\\t").Replace(value)
		return "\"" + value + "\""
	}
	// ExecStart expands dollars; Environment does not.
	arg := func(value string) string { return quote(strings.ReplaceAll(value, "$", "$$")) }
	return fmt.Sprintf(`[Unit]
Description=Phatmon Codex app server

[Service]
Type=exec
WorkingDirectory=%%h
Environment=%s
Environment=%s
ExecStart=%s app-server --listen %s
Restart=on-failure
RestartSec=5
KillSignal=SIGINT
TimeoutStopSec=30
UMask=0077

[Install]
WantedBy=default.target
`, quote("CODEX_HOME="+home.Path), quote("PATH="+path), arg(codex), arg(home.Endpoint)), nil
}

// Rename replaces an installed binary without truncating an executing inode.
func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".phatmon-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(mode)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
