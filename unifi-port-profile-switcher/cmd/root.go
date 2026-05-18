// Package cmd implements the CLI surface of unifi-port-profile-switcher.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/server"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/switcher"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// Exit codes used by the CLI. Documented in the spec; downstream automations
// (Home Assistant shell_command in particular) rely on these.
const (
	ExitOK            = 0
	ExitGeneric       = 1
	ExitPresetMissing = 2
	ExitAuth          = 3
	ExitNotFound      = 4
	ExitAPI           = 5
)

// ErrPresetNotFound is the typed error mapped to ExitPresetMissing.
var ErrPresetNotFound = errors.New("preset not found")

const usage = `unifi-port-profile-switcher: flip a UniFi switch port to a named preset profile

usage:
  unifi-port-profile-switcher [flags] <preset>          apply the named preset
  unifi-port-profile-switcher [flags] list              list configured presets
  unifi-port-profile-switcher [flags] status <preset>   show current vs target profile
  unifi-port-profile-switcher [flags] serve             start the HTTP API server

flags:
  --config PATH    path to config.yaml (default: $XDG_CONFIG_HOME/unifi-port-profile-switcher/config.yaml or /etc/unifi-port-profile-switcher/config.yaml)
  --dry-run        compute the change but do not write to the controller
  --json           emit a single JSON object on stdout
  -v, --verbose    enable debug logging

exit codes:
  0  success / graceful shutdown (serve)
  1  generic / unexpected error (also serve startup failure)
  2  preset not found in config
  3  controller authentication failure
  4  switch, port, or profile not found
  5  controller API error
`

// IO captures the streams Run writes to. main wires in os.Stdout / os.Stderr.
type IO struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Run parses args and runs the requested subcommand. The returned int is the
// process exit code, defined by the constants above.
func Run(args []string, io IO) int {
	fs := flag.NewFlagSet("unifi-port-profile-switcher", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(io.Stderr, usage) }

	var (
		configPath string
		dryRun     bool
		jsonOut    bool
		verbose    bool
	)
	fs.StringVar(&configPath, "config", "", "path to config.yaml")
	fs.BoolVar(&dryRun, "dry-run", false, "do not write changes")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output on stdout")
	fs.BoolVar(&verbose, "v", false, "verbose logging")
	fs.BoolVar(&verbose, "verbose", false, "verbose logging")

	if err := fs.Parse(args); err != nil {
		return ExitGeneric
	}
	rest := fs.Args()
	if len(rest) == 0 {
		_, _ = fmt.Fprint(io.Stderr, usage)
		return ExitGeneric
	}

	setupLogger(io.Stderr, verbose, jsonOut)

	path := resolveConfigPath(configPath)
	cfg, err := config.Load(path)
	if err != nil {
		_, _ = fmt.Fprintf(io.Stderr, "error: %v\n", err)
		return ExitGeneric
	}

	switch rest[0] {
	case "list":
		return runList(cfg, jsonOut, io)
	case "status":
		if len(rest) < 2 {
			_, _ = fmt.Fprint(io.Stderr, "error: status requires a preset name\n")
			return ExitGeneric
		}
		return runStatus(cfg, rest[1], jsonOut, io)
	case "serve":
		return runServe(cfg, io)
	default:
		// bare <preset> = apply
		return runApply(cfg, rest[0], dryRun, jsonOut, io)
	}
}

func setupLogger(w io.Writer, verbose, jsonOut bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if jsonOut {
		// In --json mode we still want logs on stderr; stdout is reserved for the result JSON.
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	slog.SetDefault(slog.New(h))
}

func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "unifi-port-profile-switcher", "config.yaml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".config", "unifi-port-profile-switcher", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/etc/unifi-port-profile-switcher/config.yaml"
}

func runList(cfg *config.Config, jsonOut bool, io IO) int {
	names := make([]string, 0, len(cfg.Presets))
	for n := range cfg.Presets {
		names = append(names, n)
	}
	sort.Strings(names)

	if jsonOut {
		out := make([]map[string]any, 0, len(names))
		for _, n := range names {
			p := cfg.Presets[n]
			out = append(out, map[string]any{
				"preset":  n,
				"switch":  p.Switch,
				"port":    p.Port,
				"profile": p.Profile,
			})
		}
		enc := json.NewEncoder(io.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			_, _ = fmt.Fprintf(io.Stderr, "error: encode json: %v\n", err)
			return ExitGeneric
		}
		return ExitOK
	}

	for _, n := range names {
		p := cfg.Presets[n]
		_, _ = fmt.Fprintf(io.Stdout, "%s\tswitch=%s\tport=%d\tprofile=%s\n", n, p.Switch, p.Port, p.Profile)
	}
	return ExitOK
}

func runStatus(cfg *config.Config, name string, jsonOut bool, io IO) int {
	preset, ok := cfg.Presets[name]
	if !ok {
		_, _ = fmt.Fprintf(io.Stderr, "error: %s: %s\n", ErrPresetNotFound, name)
		return ExitPresetMissing
	}
	cli, err := newClient(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(io.Stderr, "error: %v\n", err)
		return ExitGeneric
	}
	res, err := switcher.Status(context.Background(), cli, name, preset)
	if err != nil {
		return reportApplyError(io.Stderr, err)
	}
	return writeResult(res, jsonOut, io)
}

func runApply(cfg *config.Config, name string, dryRun, jsonOut bool, io IO) int {
	preset, ok := cfg.Presets[name]
	if !ok {
		_, _ = fmt.Fprintf(io.Stderr, "error: %s: %s\n", ErrPresetNotFound, name)
		return ExitPresetMissing
	}
	cli, err := newClient(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(io.Stderr, "error: %v\n", err)
		return ExitGeneric
	}
	res, err := switcher.Apply(context.Background(), cli, name, preset, switcher.Options{DryRun: dryRun})
	if err != nil {
		return reportApplyError(io.Stderr, err)
	}
	return writeResult(res, jsonOut, io)
}

func runServe(cfg *config.Config, io IO) int {
	if tok := os.Getenv("AUTH_TOKEN"); tok != "" {
		cfg.Server.AuthToken = tok
	}
	if cfg.Server.AuthToken == "" {
		_, _ = fmt.Fprint(io.Stderr, "error: serve requires an auth token (set server.auth_token in config or AUTH_TOKEN env var)\n")
		return ExitGeneric
	}

	factory := func() (switcher.ControllerClient, error) {
		return newClient(cfg)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return runServeContext(ctx, cfg, factory, io)
}

// RunServeContext starts the HTTP server with the given context and factory. Exported for testing.
func RunServeContext(ctx context.Context, cfg *config.Config, factory server.ClientFactory, io IO) int {
	return runServeContext(ctx, cfg, factory, io)
}

func runServeContext(ctx context.Context, cfg *config.Config, factory server.ClientFactory, io IO) int {
	srv := server.New(cfg, factory, slog.Default())
	if err := srv.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(io.Stderr, "error: serve: %v\n", err)
		return ExitGeneric
	}
	return ExitOK
}

func newClient(cfg *config.Config) (*unifi.Client, error) {
	return unifi.New(cfg.Controller.URL, cfg.Controller.Site, cfg.Controller.Username, cfg.Controller.Password, cfg.Controller.InsecureTLS)
}

func writeResult(res switcher.Result, jsonOut bool, io IO) int {
	if jsonOut {
		enc := json.NewEncoder(io.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			_, _ = fmt.Fprintf(io.Stderr, "error: encode json: %v\n", err)
			return ExitGeneric
		}
		return ExitOK
	}

	switch {
	case res.DryRun && res.FromProfile != res.ToProfile:
		_, _ = fmt.Fprintf(io.Stdout, "[dry-run] %s: switch=%s port=%d would change %q -> %q\n",
			res.Preset, res.Switch, res.Port, res.FromProfile, res.ToProfile)
	case res.Changed:
		_, _ = fmt.Fprintf(io.Stdout, "%s: switch=%s port=%d changed %q -> %q\n",
			res.Preset, res.Switch, res.Port, res.FromProfile, res.ToProfile)
	case !res.Changed && res.FromProfile == res.ToProfile:
		_, _ = fmt.Fprintf(io.Stdout, "%s: switch=%s port=%d already on %q (no-op)\n",
			res.Preset, res.Switch, res.Port, res.ToProfile)
	default:
		_, _ = fmt.Fprintf(io.Stdout, "%s: switch=%s port=%d current=%q target=%q\n",
			res.Preset, res.Switch, res.Port, res.FromProfile, res.ToProfile)
	}
	return ExitOK
}

func reportApplyError(w io.Writer, err error) int {
	_, _ = fmt.Fprintf(w, "error: %v\n", err)
	switch {
	case errors.Is(err, unifi.ErrAuth):
		return ExitAuth
	case errors.Is(err, unifi.ErrProfileNotFound),
		errors.Is(err, unifi.ErrDeviceNotFound),
		errors.Is(err, switcher.ErrPortOutOfRange):
		return ExitNotFound
	}
	var apiErr *unifi.APIError
	if errors.As(err, &apiErr) {
		return ExitAPI
	}
	return ExitGeneric
}
