package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

const createAddressesTable = `CREATE TABLE IF NOT EXISTS addresses (
	id 		SERIAL 		PRIMARY KEY,
	road_id 	INTEGER 	NOT NULL REFERENCES road(id),
	door_no 	VARCHAR(50),
	landmark 	VARCHAR(255),
	city 		VARCHAR(255) 	NOT NULL,
	state 		VARCHAR(255) 	NOT NULL,
	pincode 	VARCHAR(20),
	country 	VARCHAR(255) 	NOT NULL,
	latitude 	DOUBLE PRECISION,
	longitude 	DOUBLE PRECISION,
	img 		VARCHAR(255),
	created_at 	TIMESTAMP 	NOT NULL DEFAULT now(),
	updated_at 	TIMESTAMP 	NOT NULL DEFAULT now()
);`

func createTableAddresses() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createAddressesTable)

			return err
		},
	}
}
