package professiontype

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/secondarydb"
)

// ProfessionTypeRequest is the payload accepted by the create and update
// endpoints. It maps a profession to one of its sub-types, e.g. profession
// "professional" -> profession type "engineer", or profession "school" ->
// profession type "11th".
type ProfessionTypeRequest struct {
	Name         string `json:"name"`
	ProfessionId int    `json:"profession_id"`
}

// ProfessionTypeResponse is the enriched view returned by the get/list
// endpoints, with profession_id resolved to its referenced profession record.
type ProfessionTypeResponse struct {
	Id         int        `json:"id"`
	Name       string     `json:"name"`
	Profession Profession `json:"profession"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Profession struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

const selectProfessionTypeQuery = `
SELECT pt.id, pt.name, pt.profession_id, p.name, pt.created_at, pt.updated_at
FROM profession_types pt
JOIN professions p ON p.id = pt.profession_id
WHERE pt.deleted_at IS NULL`

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProfessionType(row rowScanner) (*ProfessionTypeResponse, error) {
	var resp ProfessionTypeResponse

	err := row.Scan(&resp.Id, &resp.Name, &resp.Profession.Id, &resp.Profession.Name, &resp.CreatedAt, &resp.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func mapSQLError(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "foreign key constraint") {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"profession_id"}}
	}

	return err
}

func getProfessionTypeByID(c *gofr.Context, id string) (*ProfessionTypeResponse, error) {
	row := secondarydb.DB.QueryRowContext(c, selectProfessionTypeQuery+" AND pt.id = $1", id)

	resp, err := scanProfessionType(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	return resp, nil
}

// Create adds a new profession type.
func Create(c *gofr.Context) (any, error) {
	var req ProfessionTypeRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	const insertQuery = `INSERT INTO profession_types (name, profession_id) VALUES ($1, $2) RETURNING id`

	var id int

	err := secondarydb.DB.QueryRowContext(c, insertQuery, req.Name, req.ProfessionId).Scan(&id)
	if err != nil {
		return nil, mapSQLError(err)
	}

	return getProfessionTypeByID(c, strconv.Itoa(id))
}

// GetAll returns every profession type that has not been soft-deleted.
func GetAll(c *gofr.Context) (any, error) {
	rows, err := secondarydb.DB.QueryContext(c, selectProfessionTypeQuery+" ORDER BY pt.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := make([]*ProfessionTypeResponse, 0)

	for rows.Next() {
		pt, err := scanProfessionType(rows)
		if err != nil {
			return nil, err
		}

		types = append(types, pt)
	}

	return types, rows.Err()
}

// Get returns a single profession type by id.
func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	return getProfessionTypeByID(c, id)
}

// Update replaces an existing profession type's details.
func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var req ProfessionTypeRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	const updateQuery = `UPDATE profession_types SET name = $1, profession_id = $2, updated_at = now()
		WHERE id = $3 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, updateQuery, req.Name, req.ProfessionId, id)
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

	return getProfessionTypeByID(c, id)
}

// Delete soft-deletes a profession type by stamping deleted_at.
func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	const deleteQuery = `UPDATE profession_types SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`

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

	return "profession type successfully deleted with id: " + id, nil
}

// RegisterRoutes wires up the profession-type CRUD endpoints on the given app.
func RegisterRoutes(a *gofr.App) {
	a.POST("/profession-type", Create)
	a.GET("/profession-type", GetAll)
	a.GET("/profession-type/{id}", Get)
	a.PUT("/profession-type/{id}", Update)
	a.DELETE("/profession-type/{id}", Delete)
}
