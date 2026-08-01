package migrations

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"

	"main/secondarydb"
)

type addressLocation struct {
	latitude, longitude sql.NullFloat64
	doorNo              sql.NullString
}

// MigrateIndividualsAddressData moves latitude/longitude/address_detail off
// the primary database's addresses table onto individuals directly, then
// drops the legacy address_id column and the addresses table itself.
//
// This deliberately does NOT run through gofr's own versioned migration
// system (app.Migrate/migrations.All): that tracker's "last migration"
// bookkeeping lives entirely in the primary database, so a migration
// recorded as done there says nothing about whether THIS secondary database
// (individuals lives in secondarydb.DB, a separate physical database) was
// ever actually touched — which is exactly what caused this to silently
// no-op against a secondary database that never got migrated. Checking real
// schema state via information_schema every boot, instead of trusting a
// recorded version number, makes this self-healing regardless of which
// secondary database is connected or how many times this partially ran
// before.
func MigrateIndividualsAddressData() error {
	hasAddressID, err := columnExists(secondarydb.DB, "individuals", "address_id")
	if err != nil {
		return fmt.Errorf("address migration: checking address_id: %w", err)
	}

	if !hasAddressID {
		return nil
	}

	if _, err := secondarydb.DB.Exec(`
		ALTER TABLE individuals
			ADD COLUMN IF NOT EXISTS latitude double precision,
			ADD COLUMN IF NOT EXISTS longitude double precision,
			ADD COLUMN IF NOT EXISTS address_detail varchar(255)`); err != nil {
		return fmt.Errorf("address migration: adding columns: %w", err)
	}

	primaryDB, err := openPrimaryDB()
	if err != nil {
		return fmt.Errorf("address migration: connecting to primary db: %w", err)
	}
	defer primaryDB.Close()

	hasAddresses, err := tableExists(primaryDB, "addresses")
	if err != nil {
		return fmt.Errorf("address migration: checking addresses table: %w", err)
	}

	if hasAddresses {
		if err := backfillFromAddresses(primaryDB); err != nil {
			return fmt.Errorf("address migration: backfilling: %w", err)
		}
	} else {
		fmt.Fprintln(os.Stderr,
			"address migration: addresses table already gone on the primary db; "+
				"individuals rows on this secondary db will keep NULL latitude/longitude/address_detail "+
				"since there's no source data left to backfill from")
	}

	if _, err := secondarydb.DB.Exec(`ALTER TABLE individuals DROP COLUMN IF EXISTS address_id`); err != nil {
		return fmt.Errorf("address migration: dropping address_id: %w", err)
	}

	if hasAddresses {
		if _, err := primaryDB.Exec(`DROP TABLE IF EXISTS addresses`); err != nil {
			return fmt.Errorf("address migration: dropping addresses table: %w", err)
		}
	}

	return nil
}

func openPrimaryDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), os.Getenv("DB_SSL_MODE"))

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()

		return nil, err
	}

	return db, nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	var exists bool

	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists)

	return exists, err
}

func tableExists(db *sql.DB, table string) (bool, error) {
	var exists bool

	err := db.QueryRow(`SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists)

	return exists, err
}

func backfillFromAddresses(primaryDB *sql.DB) error {
	addrRows, err := primaryDB.Query(`SELECT id, latitude, longitude, door_no FROM addresses`)
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
