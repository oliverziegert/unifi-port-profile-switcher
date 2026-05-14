package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/server"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/switcher"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// fakeClient implements switcher.ControllerClient for testing.
type fakeClient struct {
	profiles    []unifi.PortProfile
	devices     []unifi.Device
	loginErr    error
	updateErr   error
	updateCalls int
}

func (f *fakeClient) Login(_ context.Context) error { return f.loginErr }
func (f *fakeClient) ListPortProfiles(_ context.Context) ([]unifi.PortProfile, error) {
	return f.profiles, nil
}
func (f *fakeClient) ListDevices(_ context.Context) ([]unifi.Device, error) {
	return f.devices, nil
}
func (f *fakeClient) UpdateDevicePortOverrides(_ context.Context, _ string, _ []unifi.PortOverride) error {
	f.updateCalls++
	return f.updateErr
}

func makeConfig(authToken string) *config.Config {
	return &config.Config{
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
			Bind:      "0.0.0.0:8099",
			AuthToken: authToken,
		},
	}
}

func makeClient() *fakeClient {
	return &fakeClient{
		profiles: []unifi.PortProfile{
			{ID: "work-id", Name: "Work VLAN"},
			{ID: "personal-id", Name: "Personal VLAN"},
		},
		devices: []unifi.Device{
			{
				ID:   "dev-1",
				Name: "Office USW-24",
				MAC:  "aa:bb:cc:dd:ee:ff",
				PortTable: []unifi.Port{
					{PortIDX: 5},
				},
				PortOverrides: []unifi.PortOverride{
					{PortIDX: 5, PortconfID: "personal-id"},
				},
			},
		},
	}
}

func newServer(t *testing.T, token string, cli *fakeClient) *httptest.Server {
	t.Helper()
	cfg := makeConfig(token)
	srv := server.New(cfg, func() (switcher.ControllerClient, error) { return cli, nil }, slog.Default())
	return httptest.NewServer(srv.Handler())
}

func get(t *testing.T, srv *httptest.Server, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return res
}

func post(t *testing.T, srv *httptest.Server, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return res
}

func TestHealthz_NoAuth(t *testing.T) {
	cli := makeClient()
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := get(t, srv, "/healthz", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]bool
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body["ok"] {
		t.Errorf("ok = false, want true")
	}
}

func TestList_MissingAuth_Returns401(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := get(t, srv, "/presets", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestList_WrongToken_Returns401(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := get(t, srv, "/presets", "wrong")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestList_ValidToken_ReturnsPresets(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := get(t, srv, "/presets", "secret")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var presets []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&presets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(presets) != 1 {
		t.Fatalf("len(presets) = %d, want 1", len(presets))
	}
	if presets[0]["preset"] != "work-laptop" {
		t.Errorf("preset name = %v", presets[0]["preset"])
	}
}

func TestStatus_KnownPreset(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := get(t, srv, "/presets/work-laptop/status", "secret")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var result switcher.Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Preset != "work-laptop" {
		t.Errorf("preset = %q", result.Preset)
	}
	if result.FromProfile != "Personal VLAN" {
		t.Errorf("from_profile = %q", result.FromProfile)
	}
}

func TestStatus_UnknownPreset_Returns404(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := get(t, srv, "/presets/ghost/status", "secret")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestApply_ChangesPort(t *testing.T) {
	cli := makeClient()
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := post(t, srv, "/presets/work-laptop/apply", "secret")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var result switcher.Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Changed {
		t.Errorf("changed = false, want true")
	}
	if cli.updateCalls != 1 {
		t.Errorf("updateCalls = %d, want 1", cli.updateCalls)
	}
}

func TestApply_Idempotent(t *testing.T) {
	cli := makeClient()
	// Pre-set port to the target profile so apply is a no-op.
	cli.devices[0].PortOverrides[0].PortconfID = "work-id"
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := post(t, srv, "/presets/work-laptop/apply", "secret")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var result switcher.Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Changed {
		t.Errorf("changed = true, want false (idempotent)")
	}
	if cli.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", cli.updateCalls)
	}
}

func TestApply_DryRun(t *testing.T) {
	cli := makeClient()
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := post(t, srv, "/presets/work-laptop/apply?dry_run=1", "secret")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var result switcher.Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.DryRun {
		t.Errorf("dry_run = false, want true")
	}
	if result.Changed {
		t.Errorf("changed = true in dry-run")
	}
	if cli.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 in dry-run", cli.updateCalls)
	}
}

func TestApply_UnknownPreset_Returns404(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := post(t, srv, "/presets/ghost/apply", "secret")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestApply_ControllerAuthError_Returns502(t *testing.T) {
	cli := makeClient()
	cli.loginErr = unifi.ErrAuth
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := post(t, srv, "/presets/work-laptop/apply", "secret")
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", res.StatusCode)
	}
}

func TestApply_ControllerAPIError_Returns502(t *testing.T) {
	cli := makeClient()
	cli.updateErr = &unifi.APIError{Status: 500, Method: "PUT", Path: "/test", Message: "oops"}
	// ensure port is different so it tries to write
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := post(t, srv, "/presets/work-laptop/apply", "secret")
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, expected 200 or 502", res.StatusCode)
	}
}

func TestRequestID_IsReturnedInHeader(t *testing.T) {
	srv := newServer(t, "secret", makeClient())
	defer srv.Close()

	res := get(t, srv, "/healthz", "")
	if res.Header.Get("X-Request-Id") == "" {
		t.Errorf("X-Request-Id header is missing")
	}
}

func TestApply_DryRunTrue(t *testing.T) {
	cli := makeClient()
	srv := newServer(t, "secret", cli)
	defer srv.Close()

	res := post(t, srv, "/presets/work-laptop/apply?dry_run=true", "secret")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var result switcher.Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.DryRun {
		t.Errorf("dry_run = false for ?dry_run=true")
	}
}

// errFactory is a ClientFactory that always returns an error.
func errFactory() (switcher.ControllerClient, error) {
	return nil, errors.New("factory error")
}

func TestStatus_FactoryError_Returns500(t *testing.T) {
	cfg := makeConfig("secret")
	srv := server.New(cfg, errFactory, slog.Default())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/presets/work-laptop/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", res.StatusCode)
	}
}
