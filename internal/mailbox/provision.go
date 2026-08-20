// Package mailbox manages mailboxes and their folders.
package mailbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/mailaddr"
)

// SystemFolders are provisioned for every mailbox, in this order.
var SystemFolders = []struct{ Name, Type string }{
	{"Inbox", "inbox"},
	{"Sent", "sent"},
	{"Drafts", "drafts"},
	{"Spam", "spam"},
	{"Trash", "trash"},
}

// ErrAddressTaken is returned when the address is already used by a mailbox
// or an alias.
var ErrAddressTaken = errors.New("address already in use")

// Provision creates a mailbox with its system folders inside the caller's
// transaction. The domain must already be validated as belonging to the
// tenant. userID may be empty for shared/service mailboxes.
func Provision(ctx context.Context, tx pgx.Tx, tenantID, orgID, domainID, domainName, userID, localPart string) (mailboxID string, err error) {
	local, err := mailaddr.NormalizeLocalPart(localPart)
	if err != nil {
		return "", err
	}
	address := mailaddr.Join(local, domainName)

	// The address namespace is shared between mailboxes and aliases.
	var taken bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM mailbox_aliases WHERE address = $1)`, address).Scan(&taken); err != nil {
		return "", err
	}
	if taken {
		return "", ErrAddressTaken
	}

	var uid *string
	if userID != "" {
		uid = &userID
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO mailboxes (tenant_id, organization_id, domain_id, user_id, local_part, address)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (domain_id, local_part) DO NOTHING
		RETURNING id`,
		tenantID, orgID, domainID, uid, local, address).Scan(&mailboxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAddressTaken
	}
	if err != nil {
		return "", err
	}

	for _, f := range SystemFolders {
		if _, err := tx.Exec(ctx, `
			INSERT INTO folders (mailbox_id, name, type) VALUES ($1, $2, $3)`,
			mailboxID, f.Name, f.Type); err != nil {
			return "", fmt.Errorf("provision folder %s: %w", f.Type, err)
		}
	}
	return mailboxID, nil
}
