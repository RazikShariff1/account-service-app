package individuals

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/accounts"
	"main/activity"
	"main/middleware"
	"main/notify"
	"main/secondarydb"
)

type ErrUnauthorized struct{}

func (ErrUnauthorized) Error() string {
	return "missing request claims"
}

func (ErrUnauthorized) StatusCode() int {
	return http.StatusUnauthorized
}

// IndividualRequest is the payload accepted by the create and update endpoints.
type IndividualRequest struct {
	Name             string          `json:"name"`
	Phone            *string         `json:"phone"`
	HId              int             `json:"h_id"`
	MId              int             `json:"m_id"`
	RId              int             `json:"r_id"`
	Latitude         *float64        `json:"latitude"`
	Longitude        *float64        `json:"longitude"`
	AddressDetail    *string         `json:"address_detail"`
	Email            *string         `json:"email"`
	EmailStatus      bool            `json:"email_status"`
	PhoneStatus      bool            `json:"phone_status"`
	ProfessionTypeId int             `json:"profession_type_id"`
	ProfessionStatus int             `json:"profession_status"`
	MetaData         json.RawMessage `json:"meta_data"`
	Img              *string         `json:"img"`
}

type Pagination struct {
	Page   int `json:"page"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// IndividualListResponse is the payload returned by GetAll.
type IndividualListResponse struct {
	Individuals []*IndividualResponse `json:"individuals"`
	Pagination  Pagination            `json:"pagination"`
}

type IndividualResponse struct {
	Id                  int             `json:"id"`
	Name                string          `json:"name"`
	Phone               *string         `json:"phone"`
	Email               *string         `json:"email"`
	EmailStatus         bool            `json:"email_status"`
	PhoneStatus         bool            `json:"phone_status"`
	ProfessionStatus    int             `json:"profession_status"`
	Img                 *string         `json:"img"`
	MetaData            json.RawMessage `json:"meta_data"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	LastMetAt           *time.Time      `json:"last_met_at"`
	ProfessionUpdatedAt *time.Time      `json:"profession_updated_at"`
	Halqa               Halqa           `json:"halqa"`
	Masjid              Masjid          `json:"masjid"`
	Road                Road            `json:"road"`
	Profession          Profession      `json:"profession"`
	Latitude            *float64        `json:"latitude"`
	Longitude           *float64        `json:"longitude"`
	AddressDetail       *string         `json:"address_detail"`
}

type Halqa struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type Masjid struct {
	Id    int     `json:"id"`
	Name  string  `json:"name"`
	HId   *int    `json:"h_id,omitempty"`
	HName *string `json:"h_name,omitempty"`
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

type Filter struct {
	H_id             int
	R_id             int
	Name             string
	Profession       string
	ProfessionType   string
	ProfessionId     int
	ProfessionTypeId int
}

const selectIndividualQuery = `
SELECT
    i.id, i.name, i.phone, i.email, i.email_status, i.phone_status, i.profession_status, i.img, i.meta_data,
    i.created_at, i.updated_at, i.last_met_at, i.profession_updated_at,
    i.h_id, i.m_id, i.r_id, i.latitude, i.longitude, i.address_detail,
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
		resp                IndividualResponse
		phone, email, img   sql.NullString
		lastMetAt           sql.NullTime
		professionUpdatedAt sql.NullTime
		metaData            []byte
		latitude, longitude sql.NullFloat64
		addressDetail       sql.NullString
	)

	err := row.Scan(
		&resp.Id, &resp.Name, &phone, &email, &resp.EmailStatus, &resp.PhoneStatus, &resp.ProfessionStatus, &img, &metaData,
		&resp.CreatedAt, &resp.UpdatedAt, &lastMetAt, &professionUpdatedAt,
		&resp.Halqa.Id, &resp.Masjid.Id, &resp.Road.Id, &latitude, &longitude, &addressDetail,
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

	if professionUpdatedAt.Valid {
		resp.ProfessionUpdatedAt = &professionUpdatedAt.Time
	}

	if latitude.Valid {
		resp.Latitude = &latitude.Float64
	}

	if longitude.Valid {
		resp.Longitude = &longitude.Float64
	}

	if addressDetail.Valid {
		resp.AddressDetail = &addressDetail.String
	}

	return &resp, nil
}

func getIndividualByID(c *gofr.Context, id string, mID int) (*IndividualResponse, error) {
	row := secondarydb.DB.QueryRowContext(c, selectIndividualQuery+" AND i.m_id = $1 AND i.id = $2", mID, id)

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
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var req IndividualRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	req.HId = claims.HId
	req.MId = claims.MId

	if err := validate(c, &req); err != nil {
		return nil, err
	}

	const insertQuery = `
INSERT INTO individuals (name, phone, h_id, m_id, r_id, latitude, longitude, address_detail, email, email_status, phone_status, profession_type_id, profession_status, meta_data, img)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id`

	var id int

	err := secondarydb.DB.QueryRowContext(c, insertQuery,
		req.Name, req.Phone, req.HId, req.MId, req.RId, req.Latitude, req.Longitude, req.AddressDetail, req.Email,
		req.EmailStatus, req.PhoneStatus, req.ProfessionTypeId, req.ProfessionStatus, req.MetaData, req.Img,
	).Scan(&id)
	if err != nil {
		return nil, mapSQLError(err)
	}

	resp, err := getIndividualByID(c, strconv.Itoa(id), req.MId)
	if err != nil {
		return nil, err
	}

	if _, err := activity.Record(c, id, &claims.AId, "individual_created", nil, nil, "Individual created"); err != nil {
		return nil, err
	}

	notify.ToMasjid(c, req.MId, "new_individual_added", map[string]any{
		"Type":       resp.Profession.Name,
		"Profession": resp.Profession.ProfessionType.Name,
		"Author":     authorName(c, claims.AId),
	})

	return resp, nil
}

// authorName resolves accountID to its account name, for use as a push
// template's {{.Author}} placeholder. Returns "" if the account can't be
// found, so a lookup failure never blocks the push it's feeding into.
func authorName(c *gofr.Context, accountID int) string {
	accs, err := accounts.GetByIDs(c.SQL, []int{accountID})
	if err != nil || len(accs) == 0 {
		return ""
	}

	return accs[0].Name
}

func validate(c *gofr.Context, req *IndividualRequest) error {
	if req.Name == "" {
		return gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	if len(req.Name) < 3 {
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

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

const countIndividualQuery = `
SELECT COUNT(*)
FROM individuals i
JOIN profession_types pt ON pt.id = i.profession_type_id
JOIN professions p ON p.id = pt.profession_id
WHERE i.deleted_at IS NULL`

// buildIndividualFilterClause returns the "AND ..." clause matching filter,
// scoped to mID, along with its positional args (starting at $1).
func buildIndividualFilterClause(filter Filter, mID int) (string, []any) {
	clause := " AND i.m_id = $1"
	args := []any{mID}

	if filter.H_id != 0 {
		args = append(args, filter.H_id)
		clause += fmt.Sprintf(" AND i.h_id = $%d", len(args))
	}

	if filter.R_id != 0 {
		args = append(args, filter.R_id)
		clause += fmt.Sprintf(" AND i.r_id = $%d", len(args))
	}

	if filter.Name != "" {
		args = append(args, "%"+filter.Name+"%")
		clause += fmt.Sprintf(" AND i.name ILIKE $%d", len(args))
	}

	if filter.Profession != "" {
		args = append(args, "%"+filter.Profession+"%")
		clause += fmt.Sprintf(" AND p.name ILIKE $%d", len(args))
	}

	if filter.ProfessionType != "" {
		args = append(args, "%"+filter.ProfessionType+"%")
		clause += fmt.Sprintf(" AND pt.name ILIKE $%d", len(args))
	}

	if filter.ProfessionId != 0 {
		args = append(args, filter.ProfessionId)
		clause += fmt.Sprintf(" AND p.id = $%d", len(args))
	}

	if filter.ProfessionTypeId != 0 {
		args = append(args, filter.ProfessionTypeId)
		clause += fmt.Sprintf(" AND pt.id = $%d", len(args))
	}

	return clause, args
}

func countIndividuals(c *gofr.Context, clause string, args []any) (int, error) {
	var total int

	err := secondarydb.DB.QueryRowContext(c, countIndividualQuery+clause, args...).Scan(&total)

	return total, err
}

// GetAll returns every individual that has not been soft-deleted, scoped to
// the caller's masjid.
func GetAll(c *gofr.Context) (any, error) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	filter, err := getFilters(c)
	if err != nil {
		return nil, err
	}

	pagination, err := getPagination(c)
	if err != nil {
		return nil, err
	}

	clause, args := buildIndividualFilterClause(filter, claims.MId)

	total, err := countIndividuals(c, clause, args)
	if err != nil {
		return nil, err
	}

	pagination.Total = total

	query := selectIndividualQuery + clause +
		fmt.Sprintf(" ORDER BY i.id LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

	rows, err := secondarydb.DB.QueryContext(c, query, append(args, pagination.Limit, pagination.Offset)...)
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

	includeHData := c.Param("hdata") == "true"

	if err := enrichIndividuals(c, individuals, includeHData); err != nil {
		return nil, err
	}

	return IndividualListResponse{Individuals: individuals, Pagination: pagination}, nil
}

func getPagination(c *gofr.Context) (Pagination, error) {
	page := 1

	if p := c.Param("page"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil || v < 1 {
			return Pagination{}, gofrHTTP.ErrorInvalidParam{Params: []string{"page"}}
		}

		page = v
	}

	limit := defaultPageLimit

	if l := c.Param("limit"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil || v < 1 || v > maxPageLimit {
			return Pagination{}, gofrHTTP.ErrorInvalidParam{Params: []string{"limit"}}
		}

		limit = v
	}

	return Pagination{Page: page, Limit: limit, Offset: (page - 1) * limit}, nil
}

func getFilters(c *gofr.Context) (Filter, error) {
	var filter Filter

	if hID := c.Param("h_id"); hID != "" {
		v, err := strconv.Atoi(hID)
		if err != nil {
			return Filter{}, gofrHTTP.ErrorInvalidParam{Params: []string{"h_id"}}
		}

		filter.H_id = v
	}

	if rID := c.Param("r_id"); rID != "" {
		v, err := strconv.Atoi(rID)
		if err != nil {
			return Filter{}, gofrHTTP.ErrorInvalidParam{Params: []string{"r_id"}}
		}

		filter.R_id = v
	}

	if ptID := c.Param("professiontypeid"); ptID != "" {
		v, err := strconv.Atoi(ptID)
		if err != nil {
			return Filter{}, gofrHTTP.ErrorInvalidParam{Params: []string{"professiontypeid"}}
		}

		filter.ProfessionTypeId = v
	}

	if pID := c.Param("professionid"); pID != "" {
		v, err := strconv.Atoi(pID)
		if err != nil {
			return Filter{}, gofrHTTP.ErrorInvalidParam{Params: []string{"professionid"}}
		}

		filter.ProfessionId = v
	}

	filter.Name = c.Param("name")
	filter.Profession = c.Param("profession")
	filter.ProfessionType = c.Param("profession_type")

	return filter, nil
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

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	return getIndividualByID(c, id, claims.MId)
}

// Update replaces an existing individual's details.
func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var req IndividualRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	req.HId = claims.HId
	req.MId = claims.MId

	if err := validate(c, &req); err != nil {
		return nil, err
	}

	const updateQuery = `
UPDATE individuals
SET name = $1, phone = $2, h_id = $3, m_id = $4, r_id = $5, latitude = $6, longitude = $7, address_detail = $8, email = $9,
    email_status = $10, phone_status = $11, profession_type_id = $12, profession_status = $13, meta_data = $14, img = $15,
    profession_updated_at = CASE WHEN profession_type_id IS DISTINCT FROM $12 THEN now() ELSE profession_updated_at END,
    updated_at = now()
WHERE id = $16 AND m_id = $17 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, updateQuery,
		req.Name, req.Phone, req.HId, req.MId, req.RId, req.Latitude, req.Longitude, req.AddressDetail, req.Email,
		req.EmailStatus, req.PhoneStatus, req.ProfessionTypeId, req.ProfessionStatus, req.MetaData, req.Img, id, claims.MId,
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

	return getIndividualByID(c, id, claims.MId)
}

// Delete soft-deletes an individual by stamping deleted_at.
func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	const deleteQuery = `UPDATE individuals SET deleted_at = now() WHERE id = $1 AND m_id = $2 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, deleteQuery, id, claims.MId)
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
	a.PUT("/individual/{id}/met", Met)
	a.PUT("/individual/{id}/email-status", EmailVerified)
	a.DELETE("/individual/{id}", Delete)
}

// Met stamps last_met_at on an individual to record a home visit.
func Met(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	const metQuery = `UPDATE individuals SET last_met_at = now() WHERE id = $1 AND m_id = $2 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, metQuery, id, claims.MId)
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

	return getIndividualByID(c, id, claims.MId)
}

// EmailVerified stamps email_status=true on an individual once they've confirmed
// their email address via the link sent by communication-svc.
func EmailVerified(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	const emailVerifiedQuery = `UPDATE individuals SET email_status = true, updated_at = now() WHERE id = $1 AND m_id = $2 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, emailVerifiedQuery, id, claims.MId)
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

	return getIndividualByID(c, id, claims.MId)
}
