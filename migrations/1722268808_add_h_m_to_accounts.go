package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const addHMToAccounts = `ALTER TABLE accounts
	ADD COLUMN IF NOT EXISTS h_id INT REFERENCES h(id),
	ADD COLUMN IF NOT EXISTS m_id INT REFERENCES m(id);`

func addAccountsHMColumns() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addHMToAccounts)

			return err
		},
	}
}
