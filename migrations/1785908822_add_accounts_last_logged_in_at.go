package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const addAccountsLastLoggedInAtSQL = `ALTER TABLE accounts
	ADD COLUMN IF NOT EXISTS last_logged_in_at TIMESTAMP;`

func addAccountsLastLoggedInAt() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addAccountsLastLoggedInAtSQL)

			return err
		},
	}
}
