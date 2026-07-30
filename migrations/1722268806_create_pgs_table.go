package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const createPgsTable = `CREATE TABLE IF NOT EXISTS pgs (
	id 		SERIAL 		PRIMARY KEY,
	name 		VARCHAR(255) 	NOT NULL,
	status 		VARCHAR(50) 	NOT NULL DEFAULT '',
	created_at 	TIMESTAMP 	NOT NULL DEFAULT now(),
	updated_at 	TIMESTAMP 	NOT NULL DEFAULT now()
);`

func createTablePgs() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createPgsTable)

			return err
		},
	}
}
