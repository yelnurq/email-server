// Package deliverability serves the Admin Deliverability Center (V4
// §67-76): real counts from the platform's own delivery records, never
// invented metrics. Terminology is deliberate (§69): "delivered" is used
// only for local mailbox delivery the platform itself performed; mail
// handed to the outbound queue is "relayed" — remote acceptance is visible
// in the queue/trace, end-user delivery is unknowable from here.
package deliverability

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/dnscheck"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailcore"
)

type Handlers struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
	// Queue is nil when no mail core is configured; the live snapshot is
	// omitted then.
	Queue mailcore.QueueManager
	// Resolver powers provider detection by MX (§73); nil disables it.
	Resolver dnscheck.Resolver

	mxCache sync.Map // domain → mxCacheEntry
}

type mxCacheEntry struct {
	provider string
	expires  time.Time
}

// ranges maps the API range parameter to a window (§76). Buckets are daily
// except for 24h, which uses hours.
var ranges = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

type overview struct {
	Range  string `json:"range"`
	Totals struct {
		Accepted       int `json:"accepted"`
		DeliveredLocal int `json:"delivered_local"`
		Relayed        int `json:"relayed"`
		Failed         int `json:"failed"`
		Quarantined    int `json:"quarantined"`
	} `json:"totals"`
	TopFailures []failureRow  `json:"top_failures"`
	Providers   []providerRow `json:"providers,omitempty"`
	Series      []seriesRow   `json:"series"`
	Queue       *queueNow     `json:"queue,omitempty"`
	// Explanations render as tooltips (§70): the definitions are part of
	// the API so every surface says the same thing.
	Definitions map[string]string `json:"definitions"`
}

type failureRow struct {
	Error string `json:"error"`
	Count int    `json:"count"`
}

type providerRow struct {
	Provider string `json:"provider"`
	Domains  int    `json:"domains"`
	Relayed  int    `json:"relayed"`
	Failed   int    `json:"failed"`
}

type seriesRow struct {
	Bucket         string `json:"bucket"`
	Accepted       int    `json:"accepted"`
	DeliveredLocal int    `json:"delivered_local"`
	Relayed        int    `json:"relayed"`
	Failed         int    `json:"failed"`
}

type queueNow struct {
	Total    int `json:"total"`
	Deferred int `json:"deferred"`
}

var definitions = map[string]string{
	"accepted":        "The platform accepted the message for processing.",
	"delivered_local": "Placed into a recipient mailbox on this platform.",
	"relayed":         "Handed to the mail core's outbound queue for a remote domain. Remote acceptance shows in the queue and trace; end-user delivery cannot be observed from here.",
	"failed":          "Terminally failed — no mailbox, policy refusal, or exhausted retries.",
	"quarantined":     "Withheld by security policy pending admin review.",
}

// Overview is GET /admin/deliverability.
func (h *Handlers) Overview(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rng := r.URL.Query().Get("range")
	window, ok := ranges[rng]
	if !ok {
		rng, window = "7d", ranges["7d"]
	}
	since := time.Now().UTC().Add(-window)
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))

	out := overview{Range: rng, Definitions: definitions}

	// Event totals for the window, tenant-scoped through messages.
	domainFilter := ""
	args := []any{id.TenantID, since}
	if domain != "" {
		domainFilter = ` AND m.from_address::text LIKE '%@' || $3`
		args = append(args, domain)
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT e.type, count(*)
		FROM message_events e
		JOIN messages m ON m.id = e.message_id
		WHERE m.tenant_id = $1 AND e.created_at >= $2`+domainFilter+`
		GROUP BY e.type`, args...)
	if err != nil {
		h.Log.Error("deliverability totals failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			rows.Close()
			httpx.Internal(w, r)
			return
		}
		switch typ {
		case "email.accepted":
			out.Totals.Accepted = n
		case "email.delivered_local":
			out.Totals.DeliveredLocal = n
		case "email.relayed":
			out.Totals.Relayed = n
		case "email.failed":
			out.Totals.Failed = n
		case "email.quarantined":
			out.Totals.Quarantined = n
		}
	}
	rows.Close()

	// Top terminal failure reasons (§71-72 as far as the data really goes:
	// local refusals and stored remote replies).
	frows, err := h.Pool.Query(r.Context(), `
		SELECT r.error, count(*)
		FROM message_recipients r
		JOIN messages m ON m.id = r.message_id
		WHERE m.tenant_id = $1 AND r.created_at >= $2 AND r.status = 'failed' AND r.error <> ''`+domainFilter+`
		GROUP BY r.error ORDER BY count(*) DESC LIMIT 10`, args...)
	if err == nil {
		for frows.Next() {
			var f failureRow
			if frows.Scan(&f.Error, &f.Count) == nil {
				out.TopFailures = append(out.TopFailures, f)
			}
		}
		frows.Close()
	}

	// Provider breakdown for remote mail (§73-74): recipient domain → MX →
	// provider, cached. Only when there is data and a resolver.
	out.Providers = h.providerBreakdown(r.Context(), id.TenantID, since, domain)

	// Time series for the chart.
	bucket := "day"
	if rng == "24h" {
		bucket = "hour"
	}
	srows, err := h.Pool.Query(r.Context(), `
		SELECT date_trunc('`+bucket+`', e.created_at)::text, e.type, count(*)
		FROM message_events e
		JOIN messages m ON m.id = e.message_id
		WHERE m.tenant_id = $1 AND e.created_at >= $2
		  AND e.type IN ('email.accepted','email.delivered_local','email.relayed','email.failed')`+domainFilter+`
		GROUP BY 1, 2 ORDER BY 1`, args...)
	if err == nil {
		byBucket := map[string]*seriesRow{}
		var order []string
		for srows.Next() {
			var b, typ string
			var n int
			if srows.Scan(&b, &typ, &n) != nil {
				continue
			}
			row, ok := byBucket[b]
			if !ok {
				row = &seriesRow{Bucket: b}
				byBucket[b] = row
				order = append(order, b)
			}
			switch typ {
			case "email.accepted":
				row.Accepted = n
			case "email.delivered_local":
				row.DeliveredLocal = n
			case "email.relayed":
				row.Relayed = n
			case "email.failed":
				row.Failed = n
			}
		}
		srows.Close()
		for _, b := range order {
			out.Series = append(out.Series, *byBucket[b])
		}
	}

	// Live queue snapshot (not a historical metric).
	if h.Queue != nil && id.TenantWide() {
		if msgs, total, err := h.Queue.ListQueue(r.Context(), 50); err == nil {
			q := queueNow{Total: total}
			for _, m := range msgs {
				for _, rc := range m.Recipients {
					if rc.Status == "temp_fail" {
						q.Deferred++
					}
				}
			}
			out.Queue = &q
		}
	}

	httpx.JSON(w, http.StatusOK, out)
}

// providerBreakdown groups relayed/failed remote recipients by mail
// provider, detected through MX records (§73), with a 1h cache. Domains
// beyond the per-request resolution cap are grouped as "Unresolved" so the
// response never silently under-reports (no-silent-caps).
func (h *Handlers) providerBreakdown(ctx context.Context, tenantID string, since time.Time, senderDomain string) []providerRow {
	if h.Resolver == nil {
		return nil
	}
	domainFilter := ""
	args := []any{tenantID, since}
	if senderDomain != "" {
		domainFilter = ` AND m.from_address::text LIKE '%@' || $3`
		args = append(args, senderDomain)
	}
	rows, err := h.Pool.Query(ctx, `
		SELECT split_part(r.address::text, '@', 2), r.status, count(*)
		FROM message_recipients r
		JOIN messages m ON m.id = r.message_id
		WHERE m.tenant_id = $1 AND r.created_at >= $2
		  AND r.status IN ('relayed', 'failed') AND position('@' IN r.address::text) > 0`+domainFilter+`
		GROUP BY 1, 2`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type domStat struct{ relayed, failed int }
	stats := map[string]*domStat{}
	for rows.Next() {
		var dom, status string
		var n int
		if rows.Scan(&dom, &status, &n) != nil {
			continue
		}
		d, ok := stats[dom]
		if !ok {
			d = &domStat{}
			stats[dom] = d
		}
		if status == "relayed" {
			d.relayed += n
		} else {
			d.failed += n
		}
	}
	if len(stats) == 0 {
		return nil
	}

	const resolveCap = 20
	byProvider := map[string]*providerRow{}
	resolved := 0
	for dom, st := range stats {
		provider := "Unresolved"
		if resolved < resolveCap {
			provider = h.providerFor(ctx, dom)
			resolved++
		}
		p, ok := byProvider[provider]
		if !ok {
			p = &providerRow{Provider: provider}
			byProvider[provider] = p
		}
		p.Domains++
		p.Relayed += st.relayed
		p.Failed += st.failed
	}
	out := make([]providerRow, 0, len(byProvider))
	for _, p := range byProvider {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Relayed > out[j].Relayed })
	return out
}

// providerFor detects the mail provider by the domain's MX targets (§73 —
// never by the address domain alone).
func (h *Handlers) providerFor(ctx context.Context, domain string) string {
	if e, ok := h.mxCache.Load(domain); ok {
		entry := e.(mxCacheEntry)
		if time.Now().Before(entry.expires) {
			return entry.provider
		}
	}
	provider := "Other"
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	mxs, err := h.Resolver.LookupMX(lookupCtx, domain)
	cancel()
	if err != nil {
		provider = "Unknown"
	} else {
		for _, mx := range mxs {
			host := strings.ToLower(strings.TrimSuffix(mx.Host, "."))
			switch {
			case strings.HasSuffix(host, "google.com") || strings.HasSuffix(host, "googlemail.com"):
				provider = "Google"
			case strings.HasSuffix(host, "protection.outlook.com") || strings.HasSuffix(host, "olc.protection.outlook.com"):
				provider = "Microsoft"
			case strings.HasSuffix(host, "yahoodns.net"):
				provider = "Yahoo"
			}
			if provider != "Other" {
				break
			}
		}
	}
	h.mxCache.Store(domain, mxCacheEntry{provider: provider, expires: time.Now().Add(time.Hour)})
	return provider
}
