package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexus/backend/internal/config"
)

func routesWithEnv(env string) http.Handler {
	return (&Server{Cfg: config.Config{Env: env}}).Routes()
}

func preflight(t *testing.T, h http.Handler, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/posts", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDevCORSAllowsLocalOrigins(t *testing.T) {
	h := routesWithEnv(config.EnvDevelopment)
	for _, origin := range []string{
		"http://localhost:8090",
		"http://127.0.0.1:8090",
	} {
		rec := preflight(t, h, origin)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s: status = %d, want %d", origin, rec.Code, http.StatusNoContent)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("%s: allow-origin = %q", origin, got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("%s: vary = %q", origin, got)
		}
		if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
			t.Fatalf("%s: authorization header not allowed", origin)
		}
	}
}

func TestDevCORSRejectsForeignOrigins(t *testing.T) {
	h := routesWithEnv(config.EnvDevelopment)
	for _, origin := range []string{
		"https://evil.example",
		"http://localhost.evil.example",
		"null",
		"",
	} {
		rec := preflight(t, h, origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("%q: allow-origin = %q, want none", origin, got)
		}
	}
}

func TestCORSDisabledOutsideDevelopment(t *testing.T) {
	for _, env := range []string{config.EnvStaging, config.EnvProduction} {
		rec := preflight(t, routesWithEnv(env), "http://localhost:8090")
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("%s: allow-origin = %q, want none", env, got)
		}
	}
}
