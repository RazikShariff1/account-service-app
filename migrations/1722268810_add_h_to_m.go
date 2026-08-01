package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const addHToM = `ALTER TABLE m
	ADD COLUMN IF NOT EXISTS h_id INT REFERENCES h(id);`

func addMHColumn() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addHToM)

			return err
		},
	}
}
