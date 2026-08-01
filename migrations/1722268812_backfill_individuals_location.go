package migrations

import (
	"database/sql"

	"gofr.dev/pkg/gofr/migration"

	"main/secondarydb"
)

type addressLocation struct {
	latitude, longitude sql.NullFloat64
	doorNo              sql.NullString
}

// backfillIndividualsLocation adds latitude/longitude/address_detail directly
// onto individuals and backfills them from each individual's linked addresses
// row (via the legacy address_id) before that indirection is removed.
// addresses (primary db) and individuals (secondary db) are physically
// separate databases with no FK between them, so the join has to happen here
// in Go rather than in a single SQL statement. Guarded on address_id's
// existence so re-running this migration, or running it on an install that
// already dropped address_id, is a no-op.
func backfillIndividualsLocation() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			if _, err := secondarydb.DB.Exec(`
				ALTER TABLE individuals
					ADD COLUMN IF NOT EXISTS latitude double precision,
					ADD COLUMN IF NOT EXISTS longitude double precision,
					ADD COLUMN IF NOT EXISTS address_detail varchar(255)`); err != nil {
				return err
			}

			var hasAddressID bool

			err := secondarydb.DB.QueryRow(`
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'individuals' AND column_name = 'address_id'
				)`).Scan(&hasAddressID)
			if err != nil {
				return err
			}

			if !hasAddressID {
				return nil
			}

			return backfillFromAddresses(d)
		},
	}
}

func backfillFromAddresses(d migration.Datasource) error {
	addrRows, err := d.SQL.Query(`SELECT id, latitude, longitude, door_no FROM addresses`)
	if err != nil {
		return err
	}

	addrByID := make(map[int]addressLocation)

	for addrRows.Next() {
		var id int

		var loc addressLocation

		if err := addrRows.Scan(&id, &loc.latitude, &loc.longitude, &loc.doorNo); err != nil {
			addrRows.Close()

			return err
		}

		addrByID[id] = loc
	}

	if err := addrRows.Err(); err != nil {
		addrRows.Close()

		return err
	}

	addrRows.Close()

	tx, err := secondarydb.DB.Begin()
	if err != nil {
		return err
	}

	if err := backfillIndividualRows(tx, addrByID); err != nil {
		tx.Rollback()

		return err
	}

	return tx.Commit()
}

func backfillIndividualRows(tx *sql.Tx, addrByID map[int]addressLocation) error {
	type link struct {
		individualID, addressID int
	}

	rows, err := tx.Query(`SELECT id, address_id FROM individuals`)
	if err != nil {
		return err
	}

	var links []link

	for rows.Next() {
		var l link

		if err := rows.Scan(&l.individualID, &l.addressID); err != nil {
			rows.Close()

			return err
		}

		links = append(links, l)
	}

	if err := rows.Err(); err != nil {
		rows.Close()

		return err
	}

	rows.Close()

	for _, l := range links {
		loc := addrByID[l.addressID]

		if _, err := tx.Exec(
			`UPDATE individuals SET latitude = $1, longitude = $2, address_detail = $3 WHERE id = $4`,
			loc.latitude, loc.longitude, loc.doorNo, l.individualID,
		); err != nil {
			return err
		}
	}

	return nil
}
