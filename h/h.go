package h

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/middleware"
)

func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate")
}

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

// GetByIDs looks up several h rows by id in a single round trip, for callers
// (e.g. individuals' list enrichment) that would otherwise issue one
// GetByID call per row.
func GetByIDs(db multiQuerier, ids []int) ([]H, error) {
	rows, err := db.Query(`SELECT id, name, created_at, updated_at, status FROM h WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []H

	for rows.Next() {
		var v H

		if err := rows.Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status); err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, rows.Err()
}

type H struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Status    string `json:"status"`
}

func RegisterRoutes(a *gofr.App) {
	a.POST("/h", Create)
	a.GET("/h", GetAll)
	a.GET("/h/{id}", Get)
	a.PUT("/h/{id}", Update)
	a.DELETE("/h/{id}", Delete)
}

func validate(v *H) error {
	var missing []string

	if v.Name == "" {
		missing = append(missing, "name")
	}

	if v.Status == "" {
		missing = append(missing, "status")
	}

	if len(missing) > 0 {
		return gofrHTTP.ErrorMissingParam{Params: missing}
	}

	return nil
}

func Create(c *gofr.Context) (any, error) {
	var v H
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	v.CreatedAt = now
	v.UpdatedAt = now

	err := c.SQL.QueryRowContext(c, `INSERT INTO h (name, created_at, updated_at, status) VALUES ($1, $2, $3, $4) RETURNING id`,
		v.Name, v.CreatedAt, v.UpdatedAt, v.Status).Scan(&v.Id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, gofrHTTP.ErrorEntityAlreadyExist{}
		}

		return nil, err
	}

	return v, nil
}

func GetAll(c *gofr.Context) (any, error) {
	rows, err := c.SQL.QueryContext(c, `SELECT id, name, created_at, updated_at, status FROM h`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []H

	for rows.Next() {
		var v H

		if err := rows.Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status); err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, nil
}

// GetByID looks up a single h by id. Returns sql.ErrNoRows, unwrapped, when
// no row matches, so callers outside this package (e.g. individuals) can
// distinguish "not found" from other errors. db is typically c.SQL, or a
// caller-managed *sql.Tx when looking up several rows across packages in
// one request (Neon's PgBouncer pooler mishandles multiple sequential
// extended-protocol queries issued outside a transaction).
func GetByID(ctx context.Context, db querier, id string) (*H, error) {
	var v H

	err := db.QueryRowContext(ctx, `SELECT id, name, created_at, updated_at, status FROM h WHERE id = $1`, id).
		Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status)
	if err != nil {
		return nil, err
	}

	return &v, nil
}

func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	v, err := GetByID(c, c.SQL, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	return v, nil
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"id"}}
	}

	if idInt != claims.HId {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	var v H
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	v.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err = c.SQL.ExecContext(c, `UPDATE h SET name = $1, updated_at = $2, status = $3 WHERE id = $4`,
		v.Name, v.UpdatedAt, v.Status, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("h successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"token"}}
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"id"}}
	}

	if idInt != claims.HId {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
	}

	_, err = c.SQL.ExecContext(c, `DELETE FROM h WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("h successfully deleted with id: %s", id), nil
}
