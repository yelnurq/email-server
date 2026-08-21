package mailservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The master secret opens every mailbox in the deployment. It must never
// leave the backend: not through an error string, not through a log line,
// not through anything a handler could serialise to a client.
const masterSecret = "s3cr3t-master-passphrase"

func newProbeService(t *testing.T, handler http.HandlerFunc) (*Service, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	return &Service{
		JMAP: &JMAP{
			BaseURL:      srv.URL,
			MasterUser:   "master",
			MasterSecret: masterSecret,
			HTTP:         srv.Client(),
		},
	}, srv
}

// TestErrorsNeverCarryMasterSecret drives every failure path that produces
// an error string and asserts the secret is absent from all of them.
func TestErrorsNeverCarryMasterSecret(t *testing.T) {
	// A server that fails in the noisiest way possible: echo the request
	// back, including the Authorization header, in the error body.
	svc, srv := newProbeService(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","echo":"` + auth + `"}`))
	})
	defer srv.Close()
	ctx := context.Background()

	var errs []string
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	_, err := svc.Summary(ctx, "user@example.test")
	collect(err)
	_, _, err = svc.List(ctx, "user@example.test", ListQuery{FolderType: "inbox"})
	collect(err)
	_, err = svc.GetMessage(ctx, "user@example.test", "abc")
	collect(err)
	collect(svc.SetFlags(ctx, "user@example.test", "abc", boolPtr(true), nil))
	collect(svc.Move(ctx, "user@example.test", "abc", "trash"))
	collect(svc.Destroy(ctx, "user@example.test", "abc"))
	_, err = svc.Import(ctx, "user@example.test", "inbox", []byte("raw"), false)
	collect(err)

	if len(errs) == 0 {
		t.Fatal("expected the failing mail store to produce errors")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("master:" + masterSecret))
	for _, msg := range errs {
		if strings.Contains(msg, masterSecret) {
			t.Fatalf("error message leaks the master secret: %s", msg)
		}
		if strings.Contains(msg, encoded) {
			t.Fatalf("error message leaks the encoded credential: %s", msg)
		}
		// An echoed Authorization header is only acceptable redacted.
		if i := strings.Index(msg, "Basic "); i >= 0 {
			if !strings.HasPrefix(msg[i:], "Basic [redacted]") {
				t.Fatalf("error message carries an unredacted Authorization header: %s", msg)
			}
		}
	}
}

// TestLogOutputNeverCarriesMasterSecret runs the same failures with a
// structured logger attached and scans everything written.
func TestLogOutputNeverCarriesMasterSecret(t *testing.T) {
	svc, srv := newProbeService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream"}`))
	})
	defer srv.Close()

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := context.Background()

	if _, err := svc.Summary(ctx, "user@example.test"); err != nil {
		// Log the way handlers do: the error string, nothing else.
		log.Error("mail store operation failed", slog.String("error", err.Error()))
	}
	if err := svc.Move(ctx, "user@example.test", "abc", "trash"); err != nil {
		log.Error("move failed", slog.String("error", err.Error()))
	}
	if strings.Contains(buf.String(), masterSecret) {
		t.Fatalf("log output leaks the master secret: %s", buf.String())
	}
}

// TestRequestsSendSecretOnlyAsAuthHeader proves the secret travels in the
// HTTP Authorization header and never in a URL or request body, where it
// would land in access logs and proxies.
func TestRequestsSendSecretOnlyAsAuthHeader(t *testing.T) {
	var sawAuthHeader bool
	svc, srv := newProbeService(t, func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			sawAuthHeader = true
		}
		if strings.Contains(r.URL.String(), masterSecret) {
			t.Errorf("secret leaked into the URL: %s", r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(masterSecret)) {
			t.Errorf("secret leaked into the request body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"methodResponses":[["Mailbox/get",{"list":[]},"0"]]}`))
	})
	defer srv.Close()

	// Any call that reaches the wire is enough.
	_, _ = svc.Summary(context.Background(), "user@example.test")
	if !sawAuthHeader {
		t.Fatal("expected the client to authenticate with a Basic auth header")
	}
}

// TestMessageJSONHasNoInternalIdentifiers guards the webmail payload: what
// is serialised to a browser must not carry mail-core account handles or
// credentials.
func TestMessageJSONHasNoInternalIdentifiers(t *testing.T) {
	msg := Message{
		ID: "abc", MessageID: "<id@example.test>", From: "a@example.test",
		Subject: "hello", BodyText: "body",
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{masterSecret, "master", "Basic ", "accountId"} {
		if bytes.Contains(bytes.ToLower(raw), bytes.ToLower([]byte(forbidden))) {
			t.Fatalf("message JSON exposes %q: %s", forbidden, raw)
		}
	}
}
