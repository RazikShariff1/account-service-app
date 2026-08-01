package address

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lib/pq"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/middleware"
	"main/road"
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

// validateRoadId confirms roadID exists and belongs to the caller's own masjid,
// so a caller can't attach an address to a road outside their claimed scope.
func validateRoadId(c *gofr.Context, mId, roadId int) error {
	r, err := road.GetByID(c, c.SQL, strconv.Itoa(roadId))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gofrHTTP.ErrorInvalidParam{Params: []string{"road_id"}}
		}

		return err
	}

	if r.MId != mId {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"road_id"}}
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

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	if err := validateRoadId(c, claims.MId, v.RoadId); err != nil {
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
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	rows, err := c.SQL.QueryContext(c,
		`SELECT a.id, a.road_id, a.door_no, a.landmark, a.city, a.state, a.pincode, a.country, a.latitude, a.longitude, a.img, a.created_at, a.updated_at
		FROM addresses a
		JOIN road r ON r.id = a.road_id
		WHERE r.m_id = $1`, claims.MId)
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
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	v, err := GetByID(c, c.SQL, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	r, err := road.GetByID(c, c.SQL, strconv.Itoa(v.RoadId))
	if err != nil {
		return nil, err
	}

	if r.MId != claims.MId {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	return v, nil
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

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	if err := validateRoadId(c, claims.MId, v.RoadId); err != nil {
		return nil, err
	}

	result, err := c.SQL.ExecContext(c,
		`UPDATE addresses SET road_id = $1, door_no = $2, landmark = $3, city = $4, state = $5, pincode = $6,
		country = $7, latitude = $8, longitude = $9, img = $10, updated_at = now()
		WHERE id = $11 AND road_id IN (SELECT id FROM road WHERE m_id = $12)`,
		v.RoadId, v.DoorNo, v.Landmark, v.City, v.State, v.Pincode, v.Country, v.Latitude, v.Longitude, v.Img, id, claims.MId)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	return fmt.Sprintf("address successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	result, err := c.SQL.ExecContext(c,
		`DELETE FROM addresses WHERE id = $1 AND road_id IN (SELECT id FROM road WHERE m_id = $2)`, id, claims.MId)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	return fmt.Sprintf("address successfully deleted with id: %s", id), nil
}
