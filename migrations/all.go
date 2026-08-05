package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

func All() map[int64]migration.Migrate {
	return map[int64]migration.Migrate{
		1722268801: createTableM(),
		1722268802: createTableH(),
		1722268803: createTableAccounts(),
		1722268804: createTableRoad(),
		1722268805: createTableSchools(),
		1722268806: createTablePgs(),
		1722268807: createTableAddresses(),
		1722268808: addAccountsHMColumns(),
		1722268809: addRoadMColumn(),
		1722268810: addMHColumn(),
		1722268811: addUniqueNameConstraints(),
		1722268812: addAccountsVerificationStatus(),
		1722268814: addAccountsLastLoggedInAt(),
	}
}
