package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const createSchoolsTable = `CREATE TABLE IF NOT EXISTS schools (
	id 		SERIAL 		PRIMARY KEY,
	name 		VARCHAR(255) 	NOT NULL,
	status 		VARCHAR(50) 	NOT NULL DEFAULT '',
	created_at 	TIMESTAMP 	NOT NULL DEFAULT now(),
	updated_at 	TIMESTAMP 	NOT NULL DEFAULT now()
);`

func createTableSchools() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createSchoolsTable)

			return err
		},
	}
}
