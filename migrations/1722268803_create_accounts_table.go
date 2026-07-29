package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const createAccountsTable = `CREATE TABLE IF NOT EXISTS accounts (
	id 		SERIAL 		PRIMARY KEY,
	email 		VARCHAR(255) 	NOT NULL UNIQUE,
	password_hash 	VARCHAR(255) 	NOT NULL,
	name 		VARCHAR(255) 	NOT NULL DEFAULT '',
	phone_number 	VARCHAR(20) 	NOT NULL DEFAULT '',
	status 		VARCHAR(50) 	NOT NULL DEFAULT '',
	created_at 	TIMESTAMP 	NOT NULL DEFAULT now(),
	updated_at 	TIMESTAMP 	NOT NULL DEFAULT now()
);`

func createTableAccounts() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createAccountsTable)

			return err
		},
	}
}
