package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

// Case-insensitive uniqueness: a plain UNIQUE(name) constraint would still
// allow "Masjid A" and "masjid a" as distinct rows.
const addUniqueNames = `CREATE UNIQUE INDEX IF NOT EXISTS h_name_lower_key ON h (lower(name));
CREATE UNIQUE INDEX IF NOT EXISTS m_name_lower_key ON m (lower(name));`

func addUniqueNameConstraints() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(addUniqueNames)

			return err
		},
	}
}
