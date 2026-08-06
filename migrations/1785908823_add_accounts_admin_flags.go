package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const addAccountsAdminFlagsSQL = `ALTER TABLE accounts
	ADD COLUMN IF NOT EXISTS h_admin boolean NOT NULL DEFAULT false,
	ADD COLUMN IF NOT EXISTS m_admin boolean NOT NULL DEFAULT false;`

func addAccountsAdminFlags() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addAccountsAdminFlagsSQL)

			return err
		},
	}
}
