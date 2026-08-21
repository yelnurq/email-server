package mailcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pemMarker never may appear in anything a caller can see: error strings
// flow into logs, provisioning_error columns and audit entries.
const pemMarker = "-----BEGIN RSA PRIVATE KEY-----"

// TestDKIMErrorNeverCarriesPrivateKey reproduces the leak found live in V4:
// Stalwart's fieldAlreadyExists envelope echoes the private-key PEM in its
// "value" field. The client boundary must redact it (§120/§121).
func TestDKIMErrorNeverCarriesPrivateKey(t *testing.T) {
	leakBody := `{"error":"fieldAlreadyExists","field":"signature.rsa-x.test.private-key",` +
		`"value":"` + pemMarker + `\nSEKRETKEYMATERIAL\n-----END RSA PRIVATE KEY-----\n"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/dkim" {
			w.Write([]byte(leakBody))
			return
		}
		w.Write([]byte(`{"data":null}`))
	}))
	defer srv.Close()

	s := &Stalwart{BaseURL: srv.URL, AdminUser: "a", AdminPass: "b"}
	_, err := s.EnsureDKIMKey(context.Background(), "x.test", "s1", "rsa")
	if err == nil {
		t.Fatal("expected an error from the mail core")
	}
	msg := err.Error()
	for _, needle := range []string{pemMarker, "SEKRETKEYMATERIAL", "PRIVATE KEY-----"} {
		if strings.Contains(msg, needle) {
			t.Fatalf("error string leaks key material (%q): %s", needle, msg)
		}
	}
	// The useful diagnostic parts must survive redaction.
	if !strings.Contains(msg, "fieldAlreadyExists") || !strings.Contains(msg, "signature.rsa-x.test.private-key") {
		t.Fatalf("redaction destroyed the diagnostic context: %s", msg)
	}
}

// TestUnstructuredErrorBodiesAreRedacted covers non-JSON upstream bodies
// that carry key or app-password material.
func TestUnstructuredErrorBodiesAreRedacted(t *testing.T) {
	for _, body := range []string{
		"internal error: " + pemMarker + "\nSEKRET\n-----END RSA PRIVATE KEY-----",
		"could not store secret $app$imap$hunter2",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, body, http.StatusInternalServerError)
		}))
		s := &Stalwart{BaseURL: srv.URL, AdminUser: "a", AdminPass: "b"}
		err := s.EnsureDomain(context.Background(), "x.test")
		srv.Close()
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "SEKRET") || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("error string leaks secret material: %s", err.Error())
		}
	}
}

// TestLongBodiesAreCapped keeps stored errors short even for huge upstream
// responses.
func TestLongBodiesAreCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("x", 10_000), http.StatusBadGateway)
	}))
	defer srv.Close()
	s := &Stalwart{BaseURL: srv.URL, AdminUser: "a", AdminPass: "b"}
	err := s.EnsureDomain(context.Background(), "x.test")
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(err.Error()) > 500 {
		t.Fatalf("error string too long (%d chars)", len(err.Error()))
	}
}
