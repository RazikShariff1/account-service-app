package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const addAccountsVerificationStatusSQL = `ALTER TABLE accounts
	ADD COLUMN IF NOT EXISTS email_status boolean NOT NULL DEFAULT false,
	ADD COLUMN IF NOT EXISTS phone_status boolean NOT NULL DEFAULT false;`

func addAccountsVerificationStatus() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addAccountsVerificationStatusSQL)

			return err
		},
	}
}
