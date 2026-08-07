package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

// dropHMAdminAccountIDSQL removes admin_account_id from h/m. The column
// exists on the live DB but was never in a tracked migration (h/m pointing
// at accounts while accounts already points at h/m via h_id/m_id is a cyclic
// relationship); accounts.h_admin/m_admin (see addAccountsAdminFlags) is the
// admin marker now.
const dropHMAdminAccountIDSQL = `ALTER TABLE h DROP COLUMN IF EXISTS admin_account_id;
	ALTER TABLE m DROP COLUMN IF EXISTS admin_account_id;`

func dropHMAdminAccountID() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(dropHMAdminAccountIDSQL)

			return err
		},
	}
}
