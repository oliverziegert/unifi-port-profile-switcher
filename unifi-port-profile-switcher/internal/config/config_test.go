package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
)

func writeTemp(t *testing.T, name, body string, mode os.FileMode) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

const okConfig = `controller:
  url: https://192.168.1.1
  site: default
  username: port-switcher
  password: hunter2
  insecure_tls: true
presets:
  work-laptop:
    switch: "Office USW-24"
    port: 5
    profile: "Work VLAN"
`

func TestLoad_HappyPath(t *testing.T) {
	path := writeTemp(t, "ok.yaml", okConfig, 0o600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Controller.URL != "https://192.168.1.1" {
		t.Errorf("URL = %q", cfg.Controller.URL)
	}
	if cfg.Controller.Site != "default" {
		t.Errorf("Site = %q", cfg.Controller.Site)
	}
	if !cfg.Controller.InsecureTLS {
		t.Errorf("InsecureTLS = false, want true")
	}
	p, ok := cfg.Presets["work-laptop"]
	if !ok {
		t.Fatalf("preset work-laptop missing")
	}
	if p.Switch != "Office USW-24" || p.Port != 5 || p.Profile != "Work VLAN" {
		t.Errorf("preset = %+v", p)
	}
}

func TestLoad_DefaultsSiteWhenOmitted(t *testing.T) {
	body := strings.Replace(okConfig, "site: default\n  ", "", 1)
	path := writeTemp(t, "no-site.yaml", body, 0o600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Controller.Site != "default" {
		t.Errorf("Site = %q, want default", cfg.Controller.Site)
	}
}

func TestLoad_MissingFields(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		field string
	}{
		{
			name:  "missing url",
			body:  strings.Replace(okConfig, "url: https://192.168.1.1\n  ", "", 1),
			field: "controller.url",
		},
		{
			name:  "missing username",
			body:  strings.Replace(okConfig, "username: port-switcher\n  ", "", 1),
			field: "controller.username",
		},
		{
			name:  "missing password",
			body:  strings.Replace(okConfig, "password: hunter2\n  ", "", 1),
			field: "controller.password",
		},
		{
			name: "no presets",
			body: `controller:
  url: https://x
  username: u
  password: p
presets: {}
`,
			field: "presets",
		},
		{
			name: "preset missing switch",
			body: `controller:
  url: https://x
  username: u
  password: p
presets:
  p1:
    port: 1
    profile: q
`,
			field: "presets.p1.switch",
		},
		{
			name: "preset missing port",
			body: `controller:
  url: https://x
  username: u
  password: p
presets:
  p1:
    switch: s
    profile: q
`,
			field: "presets.p1.port",
		},
		{
			name: "preset missing profile",
			body: `controller:
  url: https://x
  username: u
  password: p
presets:
  p1:
    switch: s
    port: 1
`,
			field: "presets.p1.profile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, "bad.yaml", tt.body, 0o600)
			_, err := config.Load(path)
			var mfe *config.MissingFieldError
			if !errors.As(err, &mfe) {
				t.Fatalf("err = %v, want MissingFieldError", err)
			}
			if mfe.Field != tt.field {
				t.Errorf("field = %q, want %q", mfe.Field, tt.field)
			}
		})
	}
}

func TestLoad_RejectsInsecurePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check is POSIX-only")
	}
	path := writeTemp(t, "perms.yaml", okConfig, 0o644)
	_, err := config.Load(path)
	if !errors.Is(err, config.ErrInsecurePerms) {
		t.Fatalf("err = %v, want ErrInsecurePerms", err)
	}
}

func TestLoad_AcceptsReadOnlyPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check is POSIX-only")
	}
	path := writeTemp(t, "ro.yaml", okConfig, 0o400)
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load with 0o400: %v", err)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	path := writeTemp(t, "bad.yaml", "controller: [unterminated", 0o600)
	_, err := config.Load(path)
	if err == nil {
		t.Fatalf("Load returned nil error for malformed YAML")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatalf("Load returned nil for missing file")
	}
}

func TestLoad_ServerDefaults(t *testing.T) {
	path := writeTemp(t, "ok.yaml", okConfig, 0o600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Bind != "0.0.0.0:8099" {
		t.Errorf("Server.Bind = %q, want 0.0.0.0:8099", cfg.Server.Bind)
	}
	if cfg.Server.AuthToken != "" {
		t.Errorf("Server.AuthToken = %q, want empty", cfg.Server.AuthToken)
	}
}

func TestLoad_ServerBlock(t *testing.T) {
	body := okConfig + `server:
  bind: "127.0.0.1:9000"
  auth_token: "secret123"
`
	path := writeTemp(t, "server.yaml", body, 0o600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Bind != "127.0.0.1:9000" {
		t.Errorf("Server.Bind = %q, want 127.0.0.1:9000", cfg.Server.Bind)
	}
	if cfg.Server.AuthToken != "secret123" {
		t.Errorf("Server.AuthToken = %q, want secret123", cfg.Server.AuthToken)
	}
}

func TestLoad_MACFormatSwitch(t *testing.T) {
	body := `controller:
  url: https://x
  username: u
  password: p
presets:
  p1:
    switch: "aa:bb:cc:dd:ee:ff"
    port: 1
    profile: q
`
	path := writeTemp(t, "mac.yaml", body, 0o600)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Presets["p1"].Switch != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("Switch = %q", cfg.Presets["p1"].Switch)
	}
}
