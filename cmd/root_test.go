package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oliverziegert/unifi-port-profile-switcher/cmd"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/server"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/switcher"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

const okConfig = `controller:
  url: https://192.168.1.1
  site: default
  username: u
  password: p
  insecure_tls: true
presets:
  work-laptop:
    switch: "Office USW-24"
    port: 5
    profile: "Work VLAN"
  personal-laptop:
    switch: "Office USW-24"
    port: 5
    profile: "Personal VLAN"
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func runWith(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cmd.Run(args, cmd.IO{Stdout: &stdout, Stderr: &stderr})
	return code, stdout.String(), stderr.String()
}

func TestRun_List(t *testing.T) {
	path := writeConfig(t, okConfig)
	code, stdout, _ := runWith(t, "--config", path, "list")
	if code != cmd.ExitOK {
		t.Fatalf("exit = %d, want %d", code, cmd.ExitOK)
	}
	if !strings.Contains(stdout, "work-laptop") || !strings.Contains(stdout, "personal-laptop") {
		t.Errorf("stdout missing presets: %s", stdout)
	}
}

func TestRun_ListJSON(t *testing.T) {
	path := writeConfig(t, okConfig)
	code, stdout, _ := runWith(t, "--config", path, "--json", "list")
	if code != cmd.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var out []map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(out) != 2 {
		t.Errorf("len = %d, want 2", len(out))
	}
}

func TestRun_UnknownPresetForStatus(t *testing.T) {
	path := writeConfig(t, okConfig)
	code, _, stderr := runWith(t, "--config", path, "status", "nope")
	if code != cmd.ExitPresetMissing {
		t.Errorf("exit = %d, want %d", code, cmd.ExitPresetMissing)
	}
	if !strings.Contains(stderr, "nope") {
		t.Errorf("stderr should mention preset name: %s", stderr)
	}
}

func TestRun_UnknownPresetForApply(t *testing.T) {
	path := writeConfig(t, okConfig)
	code, _, stderr := runWith(t, "--config", path, "ghost")
	if code != cmd.ExitPresetMissing {
		t.Errorf("exit = %d, want %d", code, cmd.ExitPresetMissing)
	}
	if !strings.Contains(stderr, "ghost") {
		t.Errorf("stderr should mention preset name: %s", stderr)
	}
}

func TestRun_MissingConfigFile(t *testing.T) {
	code, _, stderr := runWith(t, "--config", "/nope/missing.yaml", "list")
	if code != cmd.ExitGeneric {
		t.Errorf("exit = %d, want %d", code, cmd.ExitGeneric)
	}
	if stderr == "" {
		t.Errorf("expected error on stderr")
	}
}

func TestRun_NoArgsPrintsUsage(t *testing.T) {
	code, _, stderr := runWith(t)
	if code != cmd.ExitGeneric {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(stderr, "usage:") {
		t.Errorf("stderr should include usage: %s", stderr)
	}
}

func TestServe_ExitsNonZeroWithoutToken(t *testing.T) {
	// Config has no server.auth_token, no env var.
	path := writeConfig(t, okConfig)
	t.Setenv("AUTH_TOKEN", "")
	code, _, stderr := runWith(t, "--config", path, "serve")
	if code == cmd.ExitOK {
		t.Errorf("serve should exit non-zero when token is missing, got 0")
	}
	if !strings.Contains(stderr, "auth token") {
		t.Errorf("stderr should mention auth token, got: %s", stderr)
	}
}

func TestServe_AcceptsRequestAndHealthz(t *testing.T) {
	// Build config with a server block so RunServeContext gets a valid token.
	cfg := &config.Config{
		Controller: config.ControllerConfig{
			URL:      "https://192.168.1.1",
			Site:     "default",
			Username: "u",
			Password: "p",
		},
		Presets: map[string]config.Preset{
			"work-laptop": {Switch: "Office USW-24", Port: 5, Profile: "Work VLAN"},
		},
		Server: config.ServerConfig{
			Bind:      "127.0.0.1:18099",
			AuthToken: "testtoken",
		},
	}

	fakeFactory := func() (switcher.ControllerClient, error) {
		return &fakeCmdClient{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan int, 1)
	go func() {
		var buf bytes.Buffer
		code := cmd.RunServeContext(ctx, cfg, server.ClientFactory(fakeFactory), cmd.IO{Stdout: &buf, Stderr: &buf})
		done <- code
	}()

	// Wait for server to be ready.
	deadline := time.Now().Add(3 * time.Second)
	for {
		res, err := http.Get("http://127.0.0.1:18099/healthz") //nolint:noctx
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not become ready in 3s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()

	select {
	case code := <-done:
		if code != cmd.ExitOK {
			t.Errorf("serve exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

// fakeCmdClient satisfies switcher.ControllerClient with no-ops.
type fakeCmdClient struct{}

func (f *fakeCmdClient) Login(_ context.Context) error { return nil }
func (f *fakeCmdClient) ListPortProfiles(_ context.Context) ([]unifi.PortProfile, error) {
	return []unifi.PortProfile{{ID: "id1", Name: "Work VLAN"}}, nil
}
func (f *fakeCmdClient) ListDevices(_ context.Context) ([]unifi.Device, error) {
	return []unifi.Device{
		{ID: "d1", Name: "Office USW-24", MAC: "aa:bb:cc:dd:ee:ff",
			PortTable:     []unifi.Port{{PortIDX: 5}},
			PortOverrides: []unifi.PortOverride{{PortIDX: 5, PortconfID: "id1"}}},
	}, nil
}
func (f *fakeCmdClient) UpdateDevicePortOverrides(_ context.Context, _ string, _ []unifi.PortOverride) error {
	return nil
}

func TestRun_StatusRequiresPresetArg(t *testing.T) {
	path := writeConfig(t, okConfig)
	code, _, stderr := runWith(t, "--config", path, "status")
	if code != cmd.ExitGeneric {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(stderr, "status requires a preset") {
		t.Errorf("stderr = %s", stderr)
	}
}
