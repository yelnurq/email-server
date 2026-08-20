package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestHandler(h *HealthChecker) http.Handler {
	log := slog.New(slog.NewTextHandler(nopWriter{}, nil))
	return New(Deps{Log: log, Health: h})
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestHealthLive(t *testing.T) {
	handler := newTestHandler(&HealthChecker{})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("live: got status %d, want %d", rec.Code, http.StatusOK)
	}
	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("live: invalid JSON response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("live: got status %q, want \"ok\"", body.Status)
	}
}

func TestHealthReadyNoDependencies(t *testing.T) {
	// With no dependencies wired, readiness reports ok with zero checks.
	handler := newTestHandler(&HealthChecker{})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ready: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthReadyFailingDependency(t *testing.T) {
	// An unreachable S3 health endpoint must flip readiness to 503.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	handler := newTestHandler(&HealthChecker{S3HealthURL: srv.URL})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready: got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ready: invalid JSON response: %v", err)
	}
	if body.Status != "degraded" {
		t.Fatalf("ready: got status %q, want \"degraded\"", body.Status)
	}
	if _, present := body.Checks["minio"]; !present {
		t.Fatalf("ready: expected a minio check entry, got %v", body.Checks)
	}
}
