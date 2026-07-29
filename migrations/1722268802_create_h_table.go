package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const createHTable = `CREATE TABLE IF NOT EXISTS h (
	id 		SERIAL 		PRIMARY KEY,
	name 		VARCHAR(255) 	NOT NULL,
	status 		VARCHAR(50) 	NOT NULL DEFAULT '',
	created_at 	TIMESTAMP 	NOT NULL DEFAULT now(),
	updated_at 	TIMESTAMP 	NOT NULL DEFAULT now()
);`

func createTableH() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createHTable)

			return err
		},
	}
}
