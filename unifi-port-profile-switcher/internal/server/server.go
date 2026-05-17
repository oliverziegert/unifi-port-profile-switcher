// Package server implements the HTTP serve subcommand's handler and routing layer.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/switcher"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// ClientFactory returns a fresh ControllerClient each call.
type ClientFactory func() (switcher.ControllerClient, error)

// Server is the HTTP server wrapping the switcher operations.
type Server struct {
	cfg     *config.Config
	factory ClientFactory
	log     *slog.Logger
	srv     *http.Server
}

// New creates a Server.
func New(cfg *config.Config, factory ClientFactory, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, factory: factory, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /presets", s.auth(s.handleList))
	mux.HandleFunc("GET /presets/{name}/status", s.auth(s.handleStatus))
	mux.HandleFunc("POST /presets/{name}/apply", s.auth(s.handleApply))
	mux.HandleFunc("GET /ports/{switch}/{port}/active", s.auth(s.handleActive))
	s.srv = &http.Server{
		Addr:    cfg.Server.Bind,
		Handler: s.logging(mux),
	}
	return s
}

// Handler returns the HTTP handler for use in tests (e.g. httptest.NewServer).
func (s *Server) Handler() http.Handler {
	return s.srv.Handler
}

// Run starts the server and blocks until ctx is cancelled, then drains with a 10s grace period.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listening", "addr", s.cfg.Server.Bind)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(shutCtx); err != nil {
		return err
	}
	return <-errCh
}

// auth wraps a handler behind bearer-token authentication.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Server.AuthToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// logging wraps a handler with per-request structured logging.
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = newRequestID()
		}
		w.Header().Set("X-Request-Id", reqID)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", reqID,
		)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	names := make([]string, 0, len(s.cfg.Presets))
	for n := range s.cfg.Presets {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		p := s.cfg.Presets[n]
		out = append(out, map[string]any{
			"preset":  n,
			"switch":  p.Switch,
			"port":    p.Port,
			"profile": p.Profile,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	preset, ok := s.cfg.Presets[name]
	if !ok {
		writeError(w, http.StatusNotFound, "preset not found: "+name)
		return
	}
	cli, err := s.factory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := switcher.Status(r.Context(), cli, name, preset)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	preset, ok := s.cfg.Presets[name]
	if !ok {
		writeError(w, http.StatusNotFound, "preset not found: "+name)
		return
	}

	q := r.URL.Query().Get("dry_run")
	dryRun := q == "1" || q == "true"

	cli, err := s.factory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := switcher.Apply(r.Context(), cli, name, preset, switcher.Options{DryRun: dryRun})
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleActive(w http.ResponseWriter, r *http.Request) {
	switchRef := r.PathValue("switch")
	portStr := r.PathValue("port")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid port: "+portStr)
		return
	}
	if port < 1 || port > 52 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("port out of range: %d (must be 1-52)", port))
		return
	}

	cli, err := s.factory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := switcher.ActivePreset(r.Context(), cli, s.cfg.Presets, switchRef, port)
	if err != nil {
		writeHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func writeHTTPError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, unifi.ErrAuth):
		writeError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, unifi.ErrProfileNotFound),
		errors.Is(err, unifi.ErrDeviceNotFound),
		errors.Is(err, switcher.ErrPortOutOfRange):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		var apiErr *unifi.APIError
		if errors.As(err, &apiErr) {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
