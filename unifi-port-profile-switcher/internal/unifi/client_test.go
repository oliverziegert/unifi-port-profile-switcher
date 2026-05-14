package unifi_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// envelopeOK wraps a JSON data payload in the UniFi `meta.rc=ok` envelope.
func envelopeOK(data string) string {
	if data == "" {
		data = "[]"
	}
	return `{"meta":{"rc":"ok"},"data":` + data + `}`
}

func newClient(t *testing.T, server *httptest.Server) *unifi.Client {
	t.Helper()
	c, err := unifi.New(server.URL, "default", "u", "p", true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestLogin_CapturesCSRFToken(t *testing.T) {
	const token = "csrf-abc-123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/login" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["username"] != "u" || body["password"] != "p" {
			t.Errorf("body = %v", body)
		}
		http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: "session-xyz"})
		w.Header().Set("X-CSRF-Token", token)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	if err := c.Login(t.Context()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Indirectly verify the token was stored by issuing a write and checking
	// the server saw X-CSRF-Token.
	var seenCSRF string
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCSRF = r.Header.Get("X-CSRF-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, envelopeOK("[]"))
	}))
	defer srv2.Close()

	// Re-point client to the second server, keeping the captured csrfToken.
	c.BaseURL = srv2.URL
	if err := c.Do(t.Context(), http.MethodPut, "/api/s/default/rest/device/x", map[string]string{"k": "v"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if seenCSRF != token {
		t.Errorf("X-CSRF-Token = %q, want %q", seenCSRF, token)
	}
}

func TestLogin_RejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad creds", http.StatusForbidden)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	err := c.Login(t.Context())
	if !errors.Is(err, unifi.ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
}

func TestDo_RelogsOn401(t *testing.T) {
	var calls atomic.Int32
	var loginCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			loginCalls.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: "tok"})
			w.Header().Set("X-CSRF-Token", "csrf")
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/rest/portconf":
			n := calls.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, envelopeOK(`[{"_id":"x","name":"Work"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newClient(t, srv)
	if err := c.Login(t.Context()); err != nil {
		t.Fatalf("initial Login: %v", err)
	}
	profiles, err := c.ListPortProfiles(t.Context())
	if err != nil {
		t.Fatalf("ListPortProfiles: %v", err)
	}
	if loginCalls.Load() != 2 {
		t.Errorf("login calls = %d, want 2 (initial + after 401)", loginCalls.Load())
	}
	if len(profiles) != 1 || profiles[0].Name != "Work" {
		t.Errorf("profiles = %+v", profiles)
	}
}

func TestDo_ReportsEnvelopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/auth/login" {
			w.Header().Set("X-CSRF-Token", "t")
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = io.WriteString(w, `{"meta":{"rc":"error","msg":"nope"},"data":[]}`)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	if err := c.Login(t.Context()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	_, err := c.ListPortProfiles(t.Context())
	var apiErr *unifi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want APIError", err)
	}
	if apiErr.Message != "nope" {
		t.Errorf("message = %q, want nope", apiErr.Message)
	}
}

func TestDo_ReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			w.Header().Set("X-CSRF-Token", "t")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "server boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newClient(t, srv)
	if err := c.Login(t.Context()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	_, err := c.ListPortProfiles(t.Context())
	var apiErr *unifi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want APIError", err)
	}
	if apiErr.Status != http.StatusInternalServerError {
		t.Errorf("status = %d", apiErr.Status)
	}
	if !strings.Contains(apiErr.Body, "boom") {
		t.Errorf("body = %q", apiErr.Body)
	}
}

func TestUpdateDevicePortOverrides_PutsBodyAndHeaders(t *testing.T) {
	var seen struct {
		method string
		path   string
		csrf   string
		body   string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			w.Header().Set("X-CSRF-Token", "csrf-tok")
			w.WriteHeader(http.StatusOK)
			return
		}
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.csrf = r.Header.Get("X-CSRF-Token")
		b, _ := io.ReadAll(r.Body)
		seen.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, envelopeOK("[]"))
	}))
	defer srv.Close()

	c := newClient(t, srv)
	if err := c.Login(t.Context()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	overrides := []unifi.PortOverride{{PortIDX: 5, PortconfID: "abc"}}
	if err := c.UpdateDevicePortOverrides(t.Context(), "dev-1", overrides); err != nil {
		t.Fatalf("UpdateDevicePortOverrides: %v", err)
	}
	if seen.method != http.MethodPut {
		t.Errorf("method = %q", seen.method)
	}
	if seen.path != "/proxy/network/api/s/default/rest/device/dev-1" {
		t.Errorf("path = %q", seen.path)
	}
	if seen.csrf != "csrf-tok" {
		t.Errorf("CSRF = %q", seen.csrf)
	}
	if !strings.Contains(seen.body, `"port_idx":5`) || !strings.Contains(seen.body, `"portconf_id":"abc"`) {
		t.Errorf("body = %s", seen.body)
	}
}
