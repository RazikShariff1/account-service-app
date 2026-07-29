package migrations

import (
	"gofr.dev/pkg/gofr/migration"
)

func All() map[int64]migration.Migrate {
	return map[int64]migration.Migrate{
		1722268801: createTableM(),
		1722268802: createTableH(),
		1722268803: createTableAccounts(),
	}
}
