package migrations

import (
	"gofr.dev/pkg/gofr/migration"

	"main/secondarydb"
)

// dropAddresses removes the legacy address_id column from individuals and
// drops the addresses table itself, now that the only data worth keeping
// (latitude/longitude, plus door_no as address_detail) has already been
// migrated directly onto individuals by backfillIndividualsLocation. Nothing
// else references addresses (its own road_id FK points outward, not back),
// so the drop is self-contained. This runs only after
// backfillIndividualsLocation's data has been verified.
func dropAddresses() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			if _, err := secondarydb.DB.Exec(`ALTER TABLE individuals DROP COLUMN IF EXISTS address_id`); err != nil {
				return err
			}

			_, err := d.SQL.Exec(`DROP TABLE IF EXISTS addresses`)

			return err
		},
	}
}
