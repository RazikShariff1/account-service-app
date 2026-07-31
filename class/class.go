package class

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/h"
	"main/secondarydb"
)

// ClassRequest is the payload accepted by the create and update endpoints.
type ClassRequest struct {
	Name string `json:"name"`
	HId  int    `json:"h_id"`
}

// ClassResponse is the enriched view returned by the get/list endpoints,
// with h_id resolved to its referenced halqa record.
type ClassResponse struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	Halqa     Halqa     `json:"halqa"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Halqa struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

const selectClassQuery = `SELECT id, name, h_id, created_at, updated_at FROM classes WHERE deleted_at IS NULL`

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanClass(row rowScanner) (*ClassResponse, error) {
	var resp ClassResponse

	err := row.Scan(&resp.Id, &resp.Name, &resp.Halqa.Id, &resp.CreatedAt, &resp.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// fetchHalqa resolves an h_id against the primary account-service database,
// since class only stores the id.
func fetchHalqa(c *gofr.Context, id int) (*Halqa, error) {
	v, err := h.GetByID(c, c.SQL, strconv.Itoa(id))
	if err != nil {
		return nil, err
	}

	return &Halqa{Id: v.Id, Name: v.Name}, nil
}

func enrichClass(c *gofr.Context, resp *ClassResponse) error {
	halqa, err := fetchHalqa(c, resp.Halqa.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gofrHTTP.ErrorInvalidParam{Params: []string{"h_id"}}
		}

		return err
	}

	resp.Halqa = *halqa

	return nil
}

func uniqueInts(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	result := make([]int, 0, len(ids))

	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}

		result = append(result, id)
	}

	return result
}

// enrichClasses resolves h_id for a whole list in a single batched query
// instead of one lookup per row.
func enrichClasses(c *gofr.Context, list []*ClassResponse) error {
	if len(list) == 0 {
		return nil
	}

	hIDs := make([]int, len(list))
	for i, cls := range list {
		hIDs[i] = cls.Halqa.Id
	}

	hs, err := h.GetByIDs(c.SQL, uniqueInts(hIDs))
	if err != nil {
		return err
	}

	halqaByID := make(map[int]Halqa, len(hs))
	for _, v := range hs {
		halqaByID[v.Id] = Halqa{Id: v.Id, Name: v.Name}
	}

	for _, cls := range list {
		halqa, ok := halqaByID[cls.Halqa.Id]
		if !ok {
			return fmt.Errorf("class: h_id %d not found", cls.Halqa.Id)
		}

		cls.Halqa = halqa
	}

	return nil
}

func getClassByID(c *gofr.Context, id string) (*ClassResponse, error) {
	row := secondarydb.DB.QueryRowContext(c, selectClassQuery+" AND id = $1", id)

	resp, err := scanClass(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	if err := enrichClass(c, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func validate(c *gofr.Context, req *ClassRequest) error {
	if req.Name == "" {
		return gofrHTTP.ErrorMissingParam{Params: []string{"name"}}
	}

	if _, err := fetchHalqa(c, req.HId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gofrHTTP.ErrorInvalidParam{Params: []string{"h_id"}}
		}

		return err
	}

	return nil
}

// Create adds a new class.
func Create(c *gofr.Context) (any, error) {
	var req ClassRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if err := validate(c, &req); err != nil {
		return nil, err
	}

	const insertQuery = `INSERT INTO classes (name, h_id) VALUES ($1, $2) RETURNING id`

	var id int

	err := secondarydb.DB.QueryRowContext(c, insertQuery, req.Name, req.HId).Scan(&id)
	if err != nil {
		return nil, err
	}

	return getClassByID(c, strconv.Itoa(id))
}

// GetAll returns every class that has not been soft-deleted.
func GetAll(c *gofr.Context) (any, error) {
	rows, err := secondarydb.DB.QueryContext(c, selectClassQuery+" ORDER BY id")
	if err != nil {
		return nil, err
	}

	classes := make([]*ClassResponse, 0)

	for rows.Next() {
		cls, err := scanClass(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}

		classes = append(classes, cls)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	if err := enrichClasses(c, classes); err != nil {
		return nil, err
	}

	return classes, nil
}

// Get returns a single class by id.
func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	return getClassByID(c, id)
}

// Update replaces an existing class's details.
func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var req ClassRequest

	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if err := validate(c, &req); err != nil {
		return nil, err
	}

	const updateQuery = `UPDATE classes SET name = $1, h_id = $2, updated_at = now() WHERE id = $3 AND deleted_at IS NULL`

	result, err := secondarydb.DB.ExecContext(c, updateQuery, req.Name, req.HId, id)
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

	return getClassByID(c, id)
}

// Delete soft-deletes a class by stamping deleted_at.
func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	const deleteQuery = `UPDATE classes SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`

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

	return "class successfully deleted with id: " + id, nil
}

// RegisterRoutes wires up the class CRUD endpoints on the given app.
func RegisterRoutes(a *gofr.App) {
	a.POST("/class", Create)
	a.GET("/class", GetAll)
	a.GET("/class/{id}", Get)
	a.PUT("/class/{id}", Update)
	a.DELETE("/class/{id}", Delete)
}
