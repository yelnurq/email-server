// Package security implements the risk evaluation for platform-accepted
// mail: a deterministic baseline (sender blocks, explicit markers) plus the
// external engines — Rspamd for spam scoring and ClamAV for malware — per
// the policy fixed in docs/adr/ADR-004. Inbound SMTP mail is scanned by the
// same engines inside the mail core's milter path; this package covers the
// control-plane acceptance path (webmail, Email API, broadcast).
package security

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/scanner"
)

// Actions ordered by severity.
const (
	ActionAllow      = "allow"
	ActionSpam       = "spam"
	ActionQuarantine = "quarantine"
)

// Quarantine reasons (V4 §49).
const (
	ReasonSpam    = "spam"
	ReasonMalware = "malware"
	ReasonPolicy  = "policy"
)

// Thresholds mirror the platform risk bands (configurable later):
// 0-40 allow, 41-60 spam, 61+ quarantine.
const (
	spamThreshold       = 41
	quarantineThreshold = 61
)

// scanTimeout bounds one external engine call inside the delivery path.
const scanTimeout = 15 * time.Second

// ErrScanUnavailable means the message carries attachments and no malware
// scanner could examine them. Per ADR-004 the delivery is deferred (the
// worker retries with backoff) — unscanned attachments are never delivered
// or relayed (§47).
var ErrScanUnavailable = errors.New("malware scanning unavailable; delivery deferred")

// Verdict is the outcome of evaluating one message.
type Verdict struct {
	Score   int      `json:"score"`
	Action  string   `json:"action"`
	Signals []string `json:"signals"`
	// Reason is the quarantine classification (§49); empty unless the
	// action is quarantine.
	Reason string `json:"reason,omitempty"`

	// External engine results (§41), persisted as security metadata.
	RspamdScore  float64  `json:"rspamd_score,omitempty"`
	RspamdAction string   `json:"rspamd_action,omitempty"`
	Symbols      []string `json:"symbols,omitempty"`
	Virus        string   `json:"virus,omitempty"`
}

// Engine evaluates messages. Zero-value scanners disable the respective
// integration (deterministic rules always run).
type Engine struct {
	Rspamd *scanner.Rspamd
	Clamd  *scanner.Clamd
	Log    *slog.Logger
}

// maxRecordedSymbols keeps stored signal lists bounded (§42: no hundreds of
// obscure symbols).
const maxRecordedSymbols = 40

// Evaluate scores a message inside the delivery transaction. raw is the
// rendered RFC822 message handed to the external engines.
func (e *Engine) Evaluate(ctx context.Context, tx pgx.Tx, tenantID, fromAddress, subject, body string, raw []byte) (Verdict, error) {
	v := Verdict{Action: ActionAllow, Signals: []string{}}

	// Rule 1: tenant sender block list (exact address or sender domain).
	domain := ""
	if at := strings.LastIndexByte(fromAddress, '@'); at > 0 {
		domain = fromAddress[at+1:]
	}
	var blocked bool
	var blockReason string
	err := tx.QueryRow(ctx, `
		SELECT true, COALESCE(reason, '') FROM sender_blocks
		WHERE tenant_id = $1 AND (
			(kind = 'address' AND pattern = $2) OR
			(kind = 'domain' AND pattern = $3)
		)
		LIMIT 1`, tenantID, fromAddress, domain).Scan(&blocked, &blockReason)
	if err != nil && err != pgx.ErrNoRows {
		return v, err
	}
	if blocked {
		v.Score += 100
		v.Signals = append(v.Signals, "sender-blocked")
	}

	// Rule 2: explicit deterministic content markers (test hooks and
	// operator overrides; they work with every engine down).
	upper := strings.ToUpper(subject + " " + body)
	if strings.Contains(upper, "[QUARANTINE-TEST]") {
		v.Score += 70
		v.Signals = append(v.Signals, "marker-quarantine")
	} else if strings.Contains(upper, "[SPAM-TEST]") {
		v.Score += 50
		v.Signals = append(v.Signals, "marker-spam")
	}

	// Rule 3: external engines (ADR-004). Rspamd's /checkv2 shares engine
	// and config with the inbound milter path — including the antivirus
	// module, so one call covers spam scoring and ClamAV.
	if err := e.scanExternal(ctx, &v, fromAddress, raw); err != nil {
		return v, err
	}

	switch {
	case v.Score >= quarantineThreshold:
		v.Action = ActionQuarantine
		if v.Reason == "" {
			if v.Virus != "" {
				v.Reason = ReasonMalware
			} else if blocked {
				v.Reason = ReasonPolicy
			} else {
				v.Reason = ReasonSpam
			}
		}
	case v.Score >= spamThreshold:
		v.Action = ActionSpam
	}
	return v, nil
}

// scanExternal runs Rspamd (with ClamAV behind it) and falls back to a
// direct ClamAV scan when Rspamd is down. Failure policy per ADR-004:
// spam scoring fails open; malware scanning of attachments fails closed as
// a deferral (ErrScanUnavailable).
func (e *Engine) scanExternal(ctx context.Context, v *Verdict, fromAddress string, raw []byte) error {
	hasAttachment := bytes.Contains(raw, []byte("Content-Disposition: attachment"))
	if !e.Rspamd.Enabled() && !e.Clamd.Enabled() {
		return nil // integration disabled (local dev without scanners)
	}

	scanCtx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	if e.Rspamd.Enabled() {
		res, err := e.Rspamd.Check(scanCtx, raw, scanner.CheckMeta{
			From: fromAddress, User: fromAddress,
		})
		if err == nil {
			e.applyRspamd(v, res)
			// CLAM_VIRUS_FAIL means rspamd answered but clamd was down: the
			// spam score is valid, malware coverage is not.
			if hasAttachment && hasSymbol(res, "CLAM_VIRUS_FAIL") {
				return ErrScanUnavailable
			}
			return nil
		}
		if e.Log != nil {
			e.Log.Warn("rspamd scan unavailable, falling back",
				slog.String("error", err.Error()))
		}
		v.Signals = append(v.Signals, "rspamd-unavailable")
	}

	// Rspamd down (or disabled): malware coverage via clamd directly.
	if hasAttachment {
		if !e.Clamd.Enabled() {
			return ErrScanUnavailable
		}
		res, err := e.Clamd.Scan(scanCtx, bytes.NewReader(raw))
		if err != nil {
			if e.Log != nil {
				e.Log.Warn("clamd scan unavailable", slog.String("error", err.Error()))
			}
			return ErrScanUnavailable
		}
		if res.Infected {
			v.Score += 100
			v.Virus = res.Virus
			v.Reason = ReasonMalware
			v.Signals = append(v.Signals, "malware:"+res.Virus)
		}
	}
	return nil
}

// applyRspamd folds a scan result into the verdict (§41).
func (e *Engine) applyRspamd(v *Verdict, res *scanner.CheckResult) {
	v.RspamdScore = res.Score
	v.RspamdAction = res.Action

	symbols := make([]scanner.Symbol, len(res.Symbols))
	copy(symbols, res.Symbols)
	// Highest-impact symbols first; the stored list is bounded (§42).
	sort.Slice(symbols, func(i, j int) bool { return abs(symbols[i].Score) > abs(symbols[j].Score) })
	for i, s := range symbols {
		if i >= maxRecordedSymbols {
			break
		}
		v.Symbols = append(v.Symbols, fmt.Sprintf("%s(%.1f)", s.Name, s.Score))
		if s.Name == "CLAM_VIRUS" {
			v.Virus = strings.Join(s.Options, ",")
			if v.Virus == "" {
				v.Virus = "detected"
			}
			v.Signals = append(v.Signals, "malware:"+v.Virus)
		}
	}

	switch res.Action {
	case "reject":
		v.Score += 70
		v.Signals = append(v.Signals, "rspamd-reject")
	case "add header", "rewrite subject", "soft reject":
		v.Score += 50
		v.Signals = append(v.Signals, "rspamd-spam")
	}
	if v.Virus != "" {
		v.Score += 100
		v.Reason = ReasonMalware
	}
}

func hasSymbol(res *scanner.CheckResult, name string) bool {
	for _, s := range res.Symbols {
		if s.Name == name {
			return true
		}
	}
	return false
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
