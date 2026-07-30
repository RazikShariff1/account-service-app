package main

import (
	"main/accounts"
	"main/address"
	"main/h"
	"main/m"
	"main/migrations"
	"main/pgs"
	"main/road"
	"main/schools"

	"gofr.dev/pkg/gofr"
)

func main() {
	app := gofr.New()

	app.Migrate(migrations.All())

	m.RegisterRoutes(app)
	h.RegisterRoutes(app)
	accounts.RegisterRoutes(app)
	road.RegisterRoutes(app)
	schools.RegisterRoutes(app)
	pgs.RegisterRoutes(app)
	address.RegisterRoutes(app)

	app.Run()
}
