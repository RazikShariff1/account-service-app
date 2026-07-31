package individuals

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/secondarydb"
)

// IndividualRequest is the payload accepted by the create and update endpoints.
type IndividualRequest struct {
	Name             string          `json:"name"`
	Phone            *string         `json:"phone"`
	HId              int             `json:"h_id"`
	MId              int             `json:"m_id"`
	RId              int             `json:"r_id"`
	AddressId        int             `json:"address_id"`
	Email            *string         `json:"email"`
	ProfessionTypeId int             `json:"profession_type_id"`
	ProfessionStatus int             `json:"profession_status"`
	MetaData         json.RawMessage `json:"meta_data"`
	Img              *string         `json:"img"`
}

// IndividualResponse is the enriched view returned by the get/list endpoints,
// with foreign keys resolved to their referenced records.
type IndividualResponse struct {
	Id               int             `json:"id"`
	Name             string          `json:"name"`
	Phone            *string         `json:"phone"`
	Email            *string         `json:"email"`
	ProfessionStatus int             `json:"profession_status"`
	Img              *string         `json:"img"`
	MetaData         json.RawMessage `json:"meta_data"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	LastMetAt        *time.Time      `json:"last_met_at"`
	Halqa            Halqa           `json:"halqa"`
	Masjid           Masjid          `json:"masjid"`
	Road             Road            `json:"road"`
	Profession       Profession      `json:"profession"`
	Address          Address         `json:"address"`
}

type Halqa struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type Masjid struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type Road struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type ProfessionType struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type Profession struct {
	Id             int            `json:"id"`
	Name           string         `json:"name"`
	ProfessionType ProfessionType `json:"profession_type"`
}

type Address struct {
	Id        int      `json:"id"`
	Road      Road     `json:"road"`
	DoorNo    *string  `json:"door_no"`
	Landmark  *string  `json:"landmark"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	Pincode   *string  `json:"pincode"`
	Country   string   `json:"country"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

const selectIndividualQuery = `
SELECT
    i.id, i.name, i.phone, i.email, i.profession_status, i.img, i.meta_data,
    i.created_at, i.updated_at, i.last_met_at,
    i.h_id, i.m_id, i.r_id, i.address_id,
    pt.id, pt.name, p.id, p.name
FROM individuals i
JOIN profession_types pt ON pt.id = i.profession_type_id
JOIN professions p ON p.id = pt.profession_id
WHERE i.deleted_at IS NULL`

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanIndividual(row rowScanner) (*IndividualResponse, error) {
	var (
		resp              IndividualResponse
		phone, email, img sql.NullString
		lastMetAt         sql.NullTime
		metaData          []byte
	)

	err := row.Scan(
		&resp.Id, &resp.Name, &phone, &email, &resp.ProfessionStatus, &img, &metaData,
		&resp.CreatedAt, &resp.UpdatedAt, &lastMetAt,
		&resp.Halqa.Id, &resp.Masjid.Id, &resp.Road.Id, &resp.Address.Id,
		&resp.Profession.ProfessionType.Id, &resp.Profession.ProfessionType.Name, &resp.Profession.Id, &resp.Profession.Name,
	)
	if err != nil {
		return nil, err
	}

	if metaData != nil {
		resp.MetaData = metaData
	}

	if phone.Valid {
		resp.Phone = &phone.String
	}

	if email.Valid {
		resp.Email = &email.String
	}

	if img.Valid {
		resp.Img = &img.String
	}

	if lastMetAt.Valid {
		resp.LastMetAt = &lastMetAt.Time
	}

	return &resp, nil
}

func getIndividualByID(c *gofr.Context, id string) (*IndividualResponse, error) {
	row := secondarydb.DB.QueryRowContext(c, selectIndividualQuery+" AND i.id = $1", id)

	resp, err := scanIndividual(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	if err := enrichIndividual(c, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

var fkColumns = []string{"profession_type_id"}

func mapSQLError(err error) error {
	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate"):
		return gofrHTTP.ErrorEntityAlreadyExist{}
	case strings.Contains(msg, "foreign key constraint"):
		for _, col := range fkColumns {
			if strings.Contains(msg, col) {
				return gofrHTTP.ErrorInvalidParam{Params: []string{col}}
			}
		}

		return gofrHTTP.ErrorInvalidParam{Params: []string{"profession_type_id"}}
	default:
		return err
	}
}

// validateReferenceIDs checks h_id, m_id, r_id and address_id against the
// primary account-service database, since individuals only stores the ids.
//
// Checked within a single transaction (see withPrimaryTx in lookup.go),
// since Neon's PgBouncer pooler mishandles multiple sequential
// extended-protocol queries issued outside a transaction.
func validateReferenceIDs(c *gofr.Context, req *IndividualRequest) error {
	var invalid []string

	err := withPrimaryTx(c.SQL, func(tx *gofrSQL.Tx) error {
		checks := []struct {
			field string
			fetch func() error
		}{
			{"h_id", func() error { _, err := fetchHalqa(c, tx, req.HId); return err }},
			{"m_id", func() error { _, err := fetchMasjid(c, tx, req.MId); return err }},
			{"r_id", func() error { _, err := fetchRoad(c, tx, req.RId); return err }},
			{"address_id", func() error { _, err := fetchAddress(c, tx, req.AddressId); return err }},
		}

		for _, chk := range checks {
			if err := chk.fetch(); err != nil {
				if errors.Is(err, errReferenceNotFound) {
					invalid = append(invalid, chk.field)
					continue
				}

				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if len(invalid) > 0 {
		return gofrHTTP.ErrorInvalidParam{Params: invalid}
	}

	return nil
}

func Create(c *gofr.Context) (any, error) {
	var req IndividualRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if err := validate(c, &req); err != nil {
		return nil, err
	}

	const insertQuery = `
INSERT INTO individuals (name, phone, h_id, m_id, r_id, address_id, email, profession_type_id, profession_status, meta_data, img)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id`

	var id int

	err := secondarydb.DB.QueryRowContext(c, insertQuery,
		req.Name, req.Phone, req.HId, req.MId, req.RId, req.AddressId, req.Email,
		req.ProfessionTypeId, req.ProfessionStatus, req.MetaData, req.Img,
	).Scan(&id)
	if err != nil {
		return nil, mapSQLError(err)
	}

	return getIndividualByID(c, strconv.Itoa(id))
}

func validate(c *gofr.Context, req *IndividualRequest) error {
	if req.Name == "" {
		return gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	if len(req.Name) < 5 {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"name"}}
	}

	if req.Phone != nil && (len(*req.Phone) < 1 || len(*req.Phone) > 10) {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"phone"}}
	}

	if req.Email != nil && *req.Email == "" {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"email"}}
	}

	return validateReferenceIDs(c, req)
}

// GetAll returns every individual that has not been soft-deleted.
func GetAll(c *gofr.Context) (any, error) {
	rows, err := secondarydb.DB.QueryContext(c, selectIndividualQuery+" ORDER BY i.id")
	if err != nil {
		return nil, err
	}

	individuals := make([]*IndividualResponse, 0)

	for rows.Next() {
		individual, err := scanIndividual(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}

		individuals = append(individuals, individual)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	// Close the secondarydb cursor before querying the primary db for
	// enrichment below — keeping it open while issuing unrelated queries
	// trips a "prepared statement does not exist" error against Neon's
	// PgBouncer pooler.
	rows.Close()

	if err := enrichIndividuals(c, individuals); err != nil {
		return nil, err
	}

	return individuals, nil
}

// Get returns a single individual by id.
func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	if id == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"id"}}
	}

	if idInt, err := strconv.Atoi(id); err != nil || idInt < 0 {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"id"}}
	}

	return getIndividualByID(c, id)
}

// Update replaces an existing individual's details.
func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var req IndividualRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if err := validate(c, &req); err != nil {
		return nil, err
	}

	const updateQuery = `
UPDATE individuals
SET name = $1, phone = $2, h_id = $3, m_id = $4, r_id = $5, address_id = $6, email = $7,
    profession_type_id = $8, profession_status = $9, meta_data = $10, img = $11, updated_at = now()
WHERE id = $12 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, updateQuery,
		req.Name, req.Phone, req.HId, req.MId, req.RId, req.AddressId, req.Email,
		req.ProfessionTypeId, req.ProfessionStatus, req.MetaData, req.Img, id,
	)
	if err != nil {
		return nil, mapSQLError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	return getIndividualByID(c, id)
}

// Delete soft-deletes an individual by stamping deleted_at.
func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	const deleteQuery = `UPDATE individuals SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, deleteQuery, id)
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

	return "individual successfully deleted with id: " + id, nil
}

// RegisterRoutes wires up the individuals CRUD endpoints on the given app.
func RegisterRoutes(a *gofr.App) {
	a.POST("/individual", Create)
	a.GET("/individual", GetAll)
	a.GET("/individual/{id}", Get)
	a.PUT("/individual/{id}", Update)
	a.DELETE("/individual/{id}", Delete)
}
