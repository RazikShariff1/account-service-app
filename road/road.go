package road

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

	"main/m"
	"main/middleware"
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

// GetByIDs looks up several road rows by id in a single round trip, for
// callers (e.g. individuals' list enrichment) that would otherwise issue
// one GetByID call per row.
func GetByIDs(db multiQuerier, ids []int) ([]Road, error) {
	rows, err := db.Query(`SELECT id, name, created_at, updated_at, status, m_id FROM road WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Road

	for rows.Next() {
		var v Road

		if err := rows.Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status, &v.MId); err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, rows.Err()
}

type Road struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Status    string `json:"status"`
	MId       int    `json:"m_id"`
}

func RegisterRoutes(a *gofr.App) {
	a.POST("/road", Create)
	a.GET("/road", GetAll)
	a.GET("/road/{id}", Get)
	a.PUT("/road/{id}", Update)
	a.DELETE("/road/{id}", Delete)
}

func validate(v *Road) error {
	var missing []string

	if v.Name == "" {
		missing = append(missing, "name")
	}

	if v.Status == "" {
		missing = append(missing, "status")
	}

	if v.MId == 0 {
		missing = append(missing, "m_id")
	}

	if len(missing) > 0 {
		return gofrHTTP.ErrorMissingParam{Params: missing}
	}

	return nil
}

// validateMId checks m_id against the m table.
func validateMId(c *gofr.Context, v *Road) error {
	if _, err := m.GetByID(c, c.SQL, strconv.Itoa(v.MId)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gofrHTTP.ErrorInvalidParam{Params: []string{"m_id"}}
		}

		return err
	}

	return nil
}

func Create(c *gofr.Context) (any, error) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	var v Road
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	v.MId = claims.MId

	if err := validate(&v); err != nil {
		return nil, err
	}

	if err := validateMId(c, &v); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	v.CreatedAt = now
	v.UpdatedAt = now

	err := c.SQL.QueryRowContext(c,
		`INSERT INTO road (name, created_at, updated_at, status, m_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		v.Name, v.CreatedAt, v.UpdatedAt, v.Status, v.MId).Scan(&v.Id)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func GetAll(c *gofr.Context) (any, error) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	rows, err := c.SQL.QueryContext(c,
		`SELECT id, name, created_at, updated_at, status, m_id FROM road WHERE m_id = $1`, claims.MId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Road

	for rows.Next() {
		var v Road

		if err := rows.Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status, &v.MId); err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, nil
}

// GetByID looks up a single road by id, unscoped by m_id. Used internally
// by individuals' enrichment, which scopes at the individuals-query level
// instead. db is typically c.SQL, or a caller-managed *sql.Tx.
func GetByID(ctx context.Context, db querier, id string) (*Road, error) {
	var v Road

	err := db.QueryRowContext(ctx, `SELECT id, name, created_at, updated_at, status, m_id FROM road WHERE id = $1`, id).
		Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status, &v.MId)
	if err != nil {
		return nil, err
	}

	return &v, nil
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

	if v.MId != claims.MId {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	return v, nil
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	var v Road
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	v.MId = claims.MId

	if err := validate(&v); err != nil {
		return nil, err
	}

	if err := validateMId(c, &v); err != nil {
		return nil, err
	}

	v.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	result, err := c.SQL.ExecContext(c,
		`UPDATE road SET name = $1, updated_at = $2, status = $3, m_id = $4 WHERE id = $5 AND m_id = $6`,
		v.Name, v.UpdatedAt, v.Status, v.MId, id, claims.MId)
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

	return fmt.Sprintf("road successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	result, err := c.SQL.ExecContext(c, `DELETE FROM road WHERE id = $1 AND m_id = $2`, id, claims.MId)
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

	return fmt.Sprintf("road successfully deleted with id: %s", id), nil
}
