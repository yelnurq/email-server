package mailcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeStalwart mimics the v0.13 management API shapes verified against a
// live server: application errors arrive as HTTP 200 with an "error" field
// ({"error":"notFound","item":"..."}), successes as {"data":...}.
type fakeStalwart struct {
	principals map[string]map[string]any
	patches    []patchOp
}

func (f *fakeStalwart) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/principal", func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		f.principals[p["name"].(string)] = p
		_ = json.NewEncoder(w).Encode(map[string]any{"data": len(f.principals)})
	})
	mux.HandleFunc("/api/principal/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len("/api/principal/"):]
		switch r.Method {
		case http.MethodGet:
			if p, ok := f.principals[name]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": p})
			} else {
				// Live-verified: missing principals are HTTP 200 + error body.
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "notFound", "item": name})
			}
		case http.MethodPatch:
			if _, ok := f.principals[name]; !ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "notFound", "item": name})
				return
			}
			var ops []patchOp
			_ = json.NewDecoder(r.Body).Decode(&ops)
			f.patches = append(f.patches, ops...)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		case http.MethodDelete:
			delete(f.principals, name)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		}
	})
	return mux
}

func newFake(t *testing.T) (*fakeStalwart, *Stalwart, func()) {
	t.Helper()
	f := &fakeStalwart{principals: map[string]map[string]any{}}
	srv := httptest.NewServer(f.handler(t))
	s := &Stalwart{BaseURL: srv.URL, AdminUser: "admin", AdminPass: "secret", HTTP: srv.Client()}
	return f, s, srv.Close
}

func TestEnsureDomainCreatesOnce(t *testing.T) {
	f, s, done := newFake(t)
	defer done()
	ctx := context.Background()

	if err := s.EnsureDomain(ctx, "example.test"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if _, ok := f.principals["example.test"]; !ok {
		t.Fatal("domain principal not created")
	}
	// Second call must be idempotent (no error, no duplicate create).
	if err := s.EnsureDomain(ctx, "example.test"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
}

func TestEnsureAccountCreateThenUpdate(t *testing.T) {
	f, s, done := newFake(t)
	defer done()
	ctx := context.Background()

	a := Account{Email: "u@example.test", DisplayName: "U", QuotaBytes: 42}
	if err := s.EnsureAccount(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}
	p := f.principals["u@example.test"]
	if p["type"] != "individual" {
		t.Fatalf("wrong type: %v", p["type"])
	}
	// Existing account: update path uses PATCH set ops.
	a.DisplayName = "U2"
	if err := s.EnsureAccount(ctx, a); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(f.patches) == 0 || f.patches[0].Action != "set" {
		t.Fatalf("expected set patches, got %+v", f.patches)
	}
}

func TestAppPasswordLifecycle(t *testing.T) {
	f, s, done := newFake(t)
	defer done()
	ctx := context.Background()

	if err := s.EnsureAccount(ctx, Account{Email: "u@example.test"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAppPassword(ctx, "u@example.test", "smtp-abc", "pw123"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.RemoveAppPassword(ctx, "u@example.test", "smtp-abc"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	var add, remove *patchOp
	for i := range f.patches {
		switch f.patches[i].Action {
		case "addItem":
			add = &f.patches[i]
		case "removeItem":
			remove = &f.patches[i]
		}
	}
	if add == nil || add.Value != "$app$smtp-abc$pw123" {
		t.Fatalf("bad add op: %+v", add)
	}
	// Removal matches by label prefix — the password value is not needed.
	if remove == nil || remove.Value != "$app$smtp-abc$" {
		t.Fatalf("bad remove op: %+v", remove)
	}
}

func TestNotFoundBodyIsError(t *testing.T) {
	_, s, done := newFake(t)
	defer done()
	// Patching a missing principal must fail (not silently succeed).
	err := s.SetAccountQuota(context.Background(), "ghost@example.test", 1)
	if err == nil {
		t.Fatal("expected error for missing principal")
	}
	if !notFound(err) {
		t.Fatalf("expected notFound classification, got %v", err)
	}
}

func TestUnavailableWrapsErr(t *testing.T) {
	s := &Stalwart{BaseURL: "http://127.0.0.1:1", AdminUser: "a", AdminPass: "b"}
	err := s.EnsureDomain(context.Background(), "example.test")
	if err == nil {
		t.Fatal("expected transport error")
	}
	st := s.Health(context.Background())
	if st.Available {
		t.Fatal("health must be unavailable")
	}
}

func TestDisabledProvider(t *testing.T) {
	var p Provider = Disabled{}
	if p.Enabled() {
		t.Fatal("Disabled must report Enabled()==false")
	}
	if err := p.EnsureDomain(context.Background(), "x"); err != nil {
		t.Fatal("Disabled mutations must be no-op successes")
	}
	if p.Health(context.Background()).Available {
		t.Fatal("Disabled health must not report available")
	}
}
