package domains

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailcore"
)

// retirePrevious is how long a rotated-out key stays published in guidance
// before its DNS record can be dropped: in-flight mail signed with the old
// key must remain verifiable.
const retirePrevious = 30 * 24 * time.Hour

type dkimKeyView struct {
	ID          string  `json:"id"`
	Selector    string  `json:"selector"`
	Algorithm   string  `json:"algorithm"`
	Status      string  `json:"status"`
	PublicKey   string  `json:"public_key"`
	DNSHost     string  `json:"dns_host"`
	DNSValue    string  `json:"dns_value"`
	CreatedAt   string  `json:"created_at"`
	ActivatedAt *string `json:"activated_at,omitempty"`
	RetireAfter *string `json:"retire_after,omitempty"`
}

// DKIMKeys lists the domain's signing keys — public material only; there is
// no endpoint anywhere that returns private keys (V4 §22).
func (h *Handlers) DKIMKeys(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	domainID := chi.URLParam(r, "id")
	name, _, _, _, ok := h.domainForDNS(r.Context(), id, domainID)
	if !ok {
		httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Domain not found")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, selector, algorithm, status, public_key,
		       created_at::text, activated_at::text, retire_after::text
		FROM dkim_keys WHERE domain_id = $1
		ORDER BY created_at DESC`, domainID)
	if err != nil {
		h.Log.Error("dkim list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []dkimKeyView{}
	for rows.Next() {
		var v dkimKeyView
		if err := rows.Scan(&v.ID, &v.Selector, &v.Algorithm, &v.Status, &v.PublicKey,
			&v.CreatedAt, &v.ActivatedAt, &v.RetireAfter); err != nil {
			httpx.Internal(w, r)
			return
		}
		v.DNSHost = v.Selector + "._domainkey." + name
		v.DNSValue = "v=DKIM1; k=" + v.Algorithm + "; p=" + v.PublicKey
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domain": name, "keys": out})
}

// RotateDKIM issues a fresh signing key for the domain (V4 §28): the mail
// core starts signing with the new selector immediately; the old key moves
// to 'previous' with a retire-after date so its DNS record is kept long
// enough to verify in-flight mail. Also serves as the initial "generate"
// action when the domain has no key yet.
func (h *Handlers) RotateDKIM(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	domainID := chi.URLParam(r, "id")
	name, _, _, _, ok := h.domainForDNS(r.Context(), id, domainID)
	if !ok {
		httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Domain not found")
		return
	}
	if !h.Provisioner.Provider.Enabled() {
		httpx.Error(w, r, http.StatusConflict, "MAIL_CORE_DISABLED",
			"No mail core is configured; DKIM keys are managed by the mail core")
		return
	}

	selector := mailcore.NewDKIMSelector()
	publicKey, err := h.Provisioner.Provider.EnsureDKIMKey(r.Context(), name, selector, "rsa")
	if err != nil {
		if errors.Is(err, mailcore.ErrUnavailable) {
			httpx.Error(w, r, http.StatusServiceUnavailable, "MAIL_CORE_UNAVAILABLE",
				"The mail core is unavailable; try again")
			return
		}
		h.Log.Error("dkim rotate failed", slog.String("domain", name), slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	hadActive := false
	tag, err := tx.Exec(r.Context(), `
		UPDATE dkim_keys SET status = 'previous', retire_after = now() + $2::interval
		WHERE domain_id = $1 AND status = 'active'`, domainID, retirePrevious.String())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	hadActive = tag.RowsAffected() > 0
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO dkim_keys (tenant_id, domain_id, selector, algorithm, public_key, status, activated_at)
		VALUES ($1, $2, $3, 'rsa', $4, 'active', now())`,
		id.TenantID, domainID, selector, publicKey); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}

	action := "dkim.generate"
	if hadActive {
		action = "dkim.rotate"
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: action,
		ResourceType: "domain", ResourceID: domainID,
		Detail: map[string]any{"domain": name, "selector": selector, "algorithm": "rsa"},
	})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"selector":  selector,
		"dns_host":  selector + "._domainkey." + name,
		"dns_value": "v=DKIM1; k=rsa; p=" + publicKey,
		"rotated":   hadActive,
		"note":      "Publish the new TXT record; keep the previous record until its retire-after date so in-flight mail still verifies.",
	})
}
