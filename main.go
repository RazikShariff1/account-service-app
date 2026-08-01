package main

import (
	"main/accounts"
	"main/address"
	"main/h"
	"main/individuals"
	"main/m"
	"main/middleware"
	"main/migrations"
	"main/pgs"
	"main/professions"
	"main/professiontype"
	"main/road"
	"main/schools"
	"main/secondarydb"

	"gofr.dev/pkg/gofr"
)

func main() {
	app := gofr.New()

	if err := secondarydb.Init(); err != nil {
		app.Logger().Fatalf("failed to connect to secondary db: %v", err)
	}

	if err := secondarydb.Migrate(); err != nil {
		app.Logger().Fatalf("failed to migrate secondary db: %v", err)
	}

	app.Migrate(migrations.All())

	app.UseMiddleware(middleware.Auth())

	m.RegisterRoutes(app)
	h.RegisterRoutes(app)
	accounts.RegisterRoutes(app)
	road.RegisterRoutes(app)
	schools.RegisterRoutes(app)
	pgs.RegisterRoutes(app)
	address.RegisterRoutes(app)
	professions.RegisterRoutes(app)
	professiontype.RegisterRoutes(app)
	individuals.RegisterRoutes(app)

	app.Run()
}
