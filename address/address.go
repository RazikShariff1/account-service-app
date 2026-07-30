package address

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"
)

// querier is satisfied by both c.SQL and a *sql.Tx, so GetByID can run
// either directly or as part of a caller-managed transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// multiQuerier is satisfied by both c.SQL and a *sql.Tx (gofr's Tx only
// exposes a context-less Query, unlike its QueryRowContext).
type multiQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// GetByIDs looks up several addresses by id in a single round trip, for
// callers (e.g. individuals' list enrichment) that would otherwise issue
// one GetByID call per row.
func GetByIDs(db multiQuerier, ids []int) ([]AddressResponse, error) {
	rows, err := db.Query(
		`SELECT id, road_id, door_no, landmark, city, state, pincode, country, latitude, longitude, img, created_at, updated_at
		FROM addresses WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AddressResponse

	for rows.Next() {
		v, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, rows.Err()
}

// AddressRequest is the payload accepted by the create and update endpoints.
type AddressRequest struct {
	RoadId    int      `json:"road_id"`
	DoorNo    *string  `json:"door_no"`
	Landmark  *string  `json:"landmark"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	Pincode   *string  `json:"pincode"`
	Country   string   `json:"country"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Img       *string  `json:"img"`
}

// AddressResponse is the view returned by the get/list endpoints.
type AddressResponse struct {
	Id        int       `json:"id"`
	RoadId    int       `json:"road_id"`
	DoorNo    *string   `json:"door_no"`
	Landmark  *string   `json:"landmark"`
	City      string    `json:"city"`
	State     string    `json:"state"`
	Pincode   *string   `json:"pincode"`
	Country   string    `json:"country"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	Img       *string   `json:"img"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func RegisterRoutes(a *gofr.App) {
	a.POST("/address", Create)
	a.GET("/address", GetAll)
	a.GET("/address/{id}", Get)
	a.PUT("/address/{id}", Update)
	a.DELETE("/address/{id}", Delete)
}

func validate(v *AddressRequest) error {
	var missing []string

	if v.RoadId == 0 {
		missing = append(missing, "road_id")
	}

	if v.City == "" {
		missing = append(missing, "city")
	}

	if v.State == "" {
		missing = append(missing, "state")
	}

	if v.Country == "" {
		missing = append(missing, "country")
	}

	if len(missing) > 0 {
		return gofrHTTP.ErrorMissingParam{Params: missing}
	}

	return nil
}

func scanAddress(row interface{ Scan(...any) error }) (AddressResponse, error) {
	var v AddressResponse

	err := row.Scan(&v.Id, &v.RoadId, &v.DoorNo, &v.Landmark, &v.City, &v.State, &v.Pincode,
		&v.Country, &v.Latitude, &v.Longitude, &v.Img, &v.CreatedAt, &v.UpdatedAt)

	return v, err
}

func Create(c *gofr.Context) (any, error) {
	var v AddressRequest
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	row := c.SQL.QueryRowContext(c,
		`INSERT INTO addresses (road_id, door_no, landmark, city, state, pincode, country, latitude, longitude, img)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, road_id, door_no, landmark, city, state, pincode, country, latitude, longitude, img, created_at, updated_at`,
		v.RoadId, v.DoorNo, v.Landmark, v.City, v.State, v.Pincode, v.Country, v.Latitude, v.Longitude, v.Img)

	result, err := scanAddress(row)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func GetAll(c *gofr.Context) (any, error) {
	rows, err := c.SQL.QueryContext(c,
		`SELECT id, road_id, door_no, landmark, city, state, pincode, country, latitude, longitude, img, created_at, updated_at
		FROM addresses`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AddressResponse

	for rows.Next() {
		v, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, nil
}

// GetByID looks up a single address by id. Returns sql.ErrNoRows, unwrapped,
// when no row matches, so callers outside this package (e.g. individuals)
// can distinguish "not found" from other errors. db is typically c.SQL, or
// a caller-managed *sql.Tx when looking up several rows across packages in
// one request (Neon's PgBouncer pooler mishandles multiple sequential
// extended-protocol queries issued outside a transaction).
func GetByID(ctx context.Context, db querier, id string) (AddressResponse, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, road_id, door_no, landmark, city, state, pincode, country, latitude, longitude, img, created_at, updated_at
		FROM addresses WHERE id = $1`, id)

	return scanAddress(row)
}

func Get(c *gofr.Context) (any, error) {
	return GetByID(c, c.SQL, c.PathParam("id"))
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var v AddressRequest
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	_, err := c.SQL.ExecContext(c,
		`UPDATE addresses SET road_id = $1, door_no = $2, landmark = $3, city = $4, state = $5, pincode = $6,
		country = $7, latitude = $8, longitude = $9, img = $10, updated_at = now() WHERE id = $11`,
		v.RoadId, v.DoorNo, v.Landmark, v.City, v.State, v.Pincode, v.Country, v.Latitude, v.Longitude, v.Img, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("address successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	_, err := c.SQL.ExecContext(c, `DELETE FROM addresses WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("address successfully deleted with id: %s", id), nil
}
