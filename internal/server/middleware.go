// SPDX-License-Identifier: AGPL-3.0-or-later
package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/server/authz"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
)

// Middleware defines a HTTP middleware component.
type Middleware func(http.Handler) http.Handler

// chainMiddleware applies the supplied middlewares in order to the provided handler.
func chainMiddleware(h http.Handler, chain ...Middleware) http.Handler {
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i] == nil {
			continue
		}
		h = chain[i](h)
	}
	return h
}

// loggingMiddleware records request metadata using slog.
func loggingMiddleware(cfg Config) Middleware {
	logger := newLogger(cfg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			reqLogger := logger.With(
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			meta := &requestctx.Metadata{}
			ctx := requestctx.WithMetadata(r.Context(), meta)
			ctx = requestctx.WithLogger(ctx, reqLogger)
			next.ServeHTTP(recorder, r.WithContext(ctx))
			effective, ok := requestctx.EffectiveProfile(ctx)
			if !ok || effective == "" {
				effective = cfg.Profile
			}
			runtime, _ := requestctx.Runtime(ctx)
			route, _ := requestctx.Route(ctx)
			attrs := []any{
				slog.Int("status", recorder.status),
				slog.String("profile.config", cfg.Profile),
				slog.String("profile.effective", effective),
				slog.Duration("duration", time.Since(start)),
			}
			if route != "" {
				attrs = append(attrs, slog.String("route", route))
			}
			if runtime != "" {
				attrs = append(attrs, slog.String("runtime.effective", runtime))
			}
			reqLogger.Info("request", attrs...)
		})
	}
}

// corsMiddleware is a no-op placeholder until dev-mode CORS support is implemented.
func corsMiddleware(cfg Config) Middleware {
	if !cfg.Dev {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1")) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// authMiddleware is stubbed; it will enforce JWT bearer scopes in later tasks.
func authMiddleware(cfg Config) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.MetricsEnabled && cfg.MetricsAllowUnauthenticated && r.Method == http.MethodGet && r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}
			required := authz.RequiredScopes(r.Method, r.URL.Path)
			info, err := resolveAuthInfo(r, cfg)
			if err != nil {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"flowd\"")
				response.Write(w, response.New(http.StatusUnauthorized, "unauthorized", response.WithDetail(err.Error())))
				return
			}
			if info == nil {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"flowd\"")
				response.Write(w, response.New(http.StatusUnauthorized, "unauthorized"))
				return
			}
			if len(required) > 0 && !info.hasScopes(required) {
				response.Write(w, response.New(http.StatusForbidden, "forbidden", response.WithDetail("missing required scope")))
				return
			}
			ctx := withAuth(r.Context(), info)
			ctx = requestctx.WithPrincipal(ctx, info.principal())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func metricsMiddleware(cfg Config) Middleware {
	if !cfg.MetricsEnabled {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := templateRoute(r.URL.Path)
			ctx := requestctx.WithRoute(r.Context(), route)
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(recorder, r.WithContext(ctx))
			duration := time.Since(start)
			metrics.Default.RecordHTTP(route, r.Method, recorder.status, duration)
		})
	}
}

func templateRoute(path string) string {
	switch {
	case path == "":
		return "/"
	case path == "/metrics":
		return "/metrics"
	case path == "/healthz":
		return "/healthz"
	case path == "/health/storage":
		return "/health/storage"
	case path == "/plans":
		return "/plans"
	case path == "/runs":
		return "/runs"
	case strings.HasPrefix(path, "/runs/"):
		switch {
		case strings.HasSuffix(path, ":cancel"):
			return "/runs/{id}:cancel"
		case strings.HasSuffix(path, "/events.ndjson"):
			return "/runs/{id}/events.ndjson"
		case strings.HasSuffix(path, "/events"):
			return "/runs/{id}/events"
		default:
			return "/runs/{id}"
		}
	case path == "/jobs":
		return "/jobs"
	case path == "/sources":
		return "/sources"
	case strings.HasPrefix(path, "/sources/"):
		return "/sources/{name}"
	case path == "/events":
		return "/events"
	default:
		return path
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func newLogger(cfg Config) *slog.Logger {
	var handler slog.Handler
	switch strings.ToLower(cfg.Log) {
	case "json":
		handler = slog.NewJSONHandler(cfg.StdOut, nil)
	default:
		handler = slog.NewTextHandler(cfg.StdOut, nil)
	}
	return slog.New(handler)
}
