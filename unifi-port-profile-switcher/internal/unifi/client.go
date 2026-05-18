// Package unifi is a thin client for the UniFi OS controller's internal REST API.
//
// It targets the /proxy/network/api/s/<site>/... surface used by the controller's
// web UI. The official Integrations API (X-API-Key, /proxy/network/integration/v1/...)
// does not expose port-override endpoints, so this client authenticates with
// session cookie + X-CSRF-Token obtained from POST /api/auth/login.
package unifi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// Client talks to a UniFi OS controller.
type Client struct {
	BaseURL   string // e.g. https://192.168.1.1 (no trailing slash)
	Site      string // typically "default"
	Username  string
	Password  string
	HTTP      *http.Client
	csrfToken string
}

// New returns a Client configured for the given controller. The returned client
// has a cookie jar and an http.Transport whose TLS verification is governed by
// insecureTLS.
func New(baseURL, site, username, password string, insecureTLS bool) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("unifi: new cookiejar: %w", err)
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureTLS}, //nolint:gosec // controlled by config flag
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Site:     site,
		Username: username,
		Password: password,
		HTTP: &http.Client{
			Jar:       jar,
			Transport: transport,
		},
	}, nil
}

// ErrAuth is returned when authentication fails.
var ErrAuth = errors.New("unifi: authentication failed")

// APIError represents a controller-side rejection of a request.
type APIError struct {
	Status  int
	Method  string
	Path    string
	Body    string // truncated response body
	Message string // controller error message, if extractable
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("unifi: %s %s: %d: %s", e.Method, e.Path, e.Status, e.Message)
	}
	return fmt.Sprintf("unifi: %s %s: %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// Login performs POST /api/auth/login and captures the CSRF token.
// The TOKEN cookie is captured by the client's cookie jar automatically.
func (c *Client) Login(ctx context.Context) error {
	body, err := json.Marshal(map[string]any{
		"username": c.Username,
		"password": c.Password,
		"remember": false,
	})
	if err != nil {
		return fmt.Errorf("unifi: marshal login body: %w", err)
	}

	loginURL := c.BaseURL + "/api/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unifi: new login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("unifi: login: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("%w: status %d: %s", ErrAuth, res.StatusCode, string(b))
	}

	c.csrfToken = res.Header.Get("X-CSRF-Token")
	if c.csrfToken == "" {
		// Some firmware spells the header differently; try the X- variants.
		c.csrfToken = res.Header.Get("x-csrf-token")
	}
	return nil
}

// envelope is the shape the controller wraps its responses in.
type envelope struct {
	Meta struct {
		RC      string `json:"rc"`
		Message string `json:"msg"`
	} `json:"meta"`
	Data json.RawMessage `json:"data"`
}

// Do issues an authenticated request against /proxy/network<path>. The path
// argument must start with a slash (for example "/api/s/default/rest/portconf").
// On a 401 it performs a single re-login and retries once. The "data" field of
// the standard UniFi envelope is decoded into out, when out is non-nil.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	if err := c.do(ctx, method, path, body, out); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			if err2 := c.Login(ctx); err2 != nil {
				return err2
			}
			return c.do(ctx, method, path, body, out)
		}
		return err
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	fullURL := c.BaseURL + "/proxy/network" + path

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("unifi: marshal %s %s body: %w", method, path, err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("unifi: new request %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && c.csrfToken != "" {
		req.Header.Set("X-CSRF-Token", c.csrfToken)
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("unifi: %s %s: %w", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()

	// Refresh CSRF token if the response provides a new one.
	if t := res.Header.Get("X-CSRF-Token"); t != "" {
		c.csrfToken = t
	}

	if res.StatusCode == http.StatusUnauthorized {
		return &APIError{Status: res.StatusCode, Method: method, Path: path, Message: "unauthorized"}
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("unifi: read %s %s response: %w", method, path, err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &APIError{
			Status:  res.StatusCode,
			Method:  method,
			Path:    path,
			Body:    truncate(string(raw), 512),
			Message: extractErrorMessage(raw),
		}
	}

	if len(raw) == 0 {
		return nil
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Meta.RC != "" {
		if env.Meta.RC != "ok" {
			return &APIError{
				Status:  res.StatusCode,
				Method:  method,
				Path:    path,
				Body:    truncate(string(raw), 512),
				Message: env.Meta.Message,
			}
		}
		if out != nil && len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("unifi: decode %s %s data: %w", method, path, err)
			}
		}
		return nil
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("unifi: decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func extractErrorMessage(raw []byte) string {
	var env envelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Meta.Message != "" {
		return env.Meta.Message
	}
	return ""
}

// sitePath returns "/api/s/<site><suffix>" with the suffix appended verbatim.
func (c *Client) sitePath(suffix string) string {
	return "/api/s/" + url.PathEscape(c.Site) + suffix
}
