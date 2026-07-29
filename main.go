package main

import (
	"main/accounts"
	"main/h"
	"main/m"
	"main/migrations"

	"gofr.dev/pkg/gofr"
)

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())

	m.RegisterRoutes(app)
	h.RegisterRoutes(app)
	accounts.RegisterRoutes(app)

	app.Run()
}
