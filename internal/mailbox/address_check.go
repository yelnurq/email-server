package mailbox

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// AddressInUse reports whether an address is already claimed by a mailbox,
// alias or group. The address namespace is shared across all three.
func AddressInUse(ctx context.Context, tx pgx.Tx, address string) (bool, error) {
	var taken bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM mailboxes WHERE address = $1)
		    OR EXISTS (SELECT 1 FROM mailbox_aliases WHERE address = $1)
		    OR EXISTS (SELECT 1 FROM mail_groups WHERE address = $1)`,
		address).Scan(&taken)
	return taken, err
}
