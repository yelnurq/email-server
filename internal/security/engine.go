// Package security implements the deterministic risk baseline for local
// mail: sender block lists and explicit content markers. It deliberately
// contains no ML and no external engines — Rspamd/ClamAV integrate behind
// this same Verdict contract in a later phase.
package security

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Actions ordered by severity.
const (
	ActionAllow      = "allow"
	ActionSpam       = "spam"
	ActionQuarantine = "quarantine"
)

// Thresholds mirror the platform risk bands (configurable later):
// 0-40 allow, 41-60 spam, 61+ quarantine.
const (
	spamThreshold       = 41
	quarantineThreshold = 61
)

// Verdict is the outcome of evaluating one message.
type Verdict struct {
	Score   int      `json:"score"`
	Action  string   `json:"action"`
	Signals []string `json:"signals"`
}

// Evaluate scores a message inside the delivery transaction.
func Evaluate(ctx context.Context, tx pgx.Tx, tenantID, fromAddress, subject, body string) (Verdict, error) {
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

	// Rule 2: explicit deterministic content markers. These double as the
	// test hooks for the pipeline until real engines (Rspamd) plug in.
	upper := strings.ToUpper(subject + " " + body)
	if strings.Contains(upper, "[QUARANTINE-TEST]") {
		v.Score += 70
		v.Signals = append(v.Signals, "marker-quarantine")
	} else if strings.Contains(upper, "[SPAM-TEST]") {
		v.Score += 50
		v.Signals = append(v.Signals, "marker-spam")
	}

	switch {
	case v.Score >= quarantineThreshold:
		v.Action = ActionQuarantine
	case v.Score >= spamThreshold:
		v.Action = ActionSpam
	}
	return v, nil
}
