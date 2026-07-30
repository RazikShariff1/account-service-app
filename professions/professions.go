package professions

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/secondarydb"
)

type ProfessionRequest struct {
	Name string `json:"name"`
}

type ProfessionResponse struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const selectProfessionQuery = `SELECT id, name, created_at, updated_at FROM professions WHERE deleted_at IS NULL`

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProfession(row rowScanner) (*ProfessionResponse, error) {
	var resp ProfessionResponse

	err := row.Scan(&resp.Id, &resp.Name, &resp.CreatedAt, &resp.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func getProfessionByID(c *gofr.Context, id string) (*ProfessionResponse, error) {
	row := secondarydb.DB.QueryRowContext(c, selectProfessionQuery+" AND id = $1", id)

	resp, err := scanProfession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	return resp, nil
}

// Create adds a new profession.
func Create(c *gofr.Context) (any, error) {
	var req ProfessionRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	const insertQuery = `INSERT INTO professions (name) VALUES ($1) RETURNING id`

	var id int

	err := secondarydb.DB.QueryRowContext(c, insertQuery, req.Name).Scan(&id)
	if err != nil {
		return nil, err
	}

	return getProfessionByID(c, strconv.Itoa(id))
}

// GetAll returns every profession that has not been soft-deleted.
func GetAll(c *gofr.Context) (any, error) {
	rows, err := secondarydb.DB.QueryContext(c, selectProfessionQuery+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	professions := make([]*ProfessionResponse, 0)

	for rows.Next() {
		profession, err := scanProfession(rows)
		if err != nil {
			return nil, err
		}

		professions = append(professions, profession)
	}

	return professions, rows.Err()
}

// Get returns a single profession by id.
func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	return getProfessionByID(c, id)
}

// Update replaces an existing profession's details.
func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var req ProfessionRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	const updateQuery = `UPDATE professions SET name = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, updateQuery, req.Name, id)
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

	return getProfessionByID(c, id)
}

// Delete soft-deletes a profession by stamping deleted_at.
func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	const deleteQuery = `UPDATE professions SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`

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

	return "profession successfully deleted with id: " + id, nil
}

// RegisterRoutes wires up the professions CRUD endpoints on the given app.
func RegisterRoutes(a *gofr.App) {
	a.POST("/profession", Create)
	a.GET("/profession", GetAll)
	a.GET("/profession/{id}", Get)
	a.PUT("/profession/{id}", Update)
	a.DELETE("/profession/{id}", Delete)
}
