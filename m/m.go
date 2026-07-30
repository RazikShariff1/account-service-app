package m

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

// GetByIDs looks up several m rows by id in a single round trip, for callers
// (e.g. individuals' list enrichment) that would otherwise issue one
// GetByID call per row.
func GetByIDs(db multiQuerier, ids []int) ([]M, error) {
	rows, err := db.Query(`SELECT id, name, created_at, updated_at, status FROM m WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []M

	for rows.Next() {
		var v M

		if err := rows.Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status); err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, rows.Err()
}

type M struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Status    string `json:"status"`
}

func RegisterRoutes(a *gofr.App) {
	a.POST("/m", Create)
	a.GET("/m", GetAll)
	a.GET("/m/{id}", Get)
	a.PUT("/m/{id}", Update)
	a.DELETE("/m/{id}", Delete)
}

func validate(v *M) error {
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
	var v M
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	v.CreatedAt = now
	v.UpdatedAt = now

	err := c.SQL.QueryRowContext(c, `INSERT INTO m (name, created_at, updated_at, status) VALUES ($1, $2, $3, $4) RETURNING id`,
		v.Name, v.CreatedAt, v.UpdatedAt, v.Status).Scan(&v.Id)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func GetAll(c *gofr.Context) (any, error) {
	rows, err := c.SQL.QueryContext(c, `SELECT id, name, created_at, updated_at, status FROM m`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []M

	for rows.Next() {
		var v M

		if err := rows.Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status); err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return result, nil
}

// GetByID looks up a single m by id. Returns sql.ErrNoRows, unwrapped, when
// no row matches, so callers outside this package (e.g. individuals) can
// distinguish "not found" from other errors. db is typically c.SQL, or a
// caller-managed *sql.Tx when looking up several rows across packages in
// one request (Neon's PgBouncer pooler mishandles multiple sequential
// extended-protocol queries issued outside a transaction).
func GetByID(ctx context.Context, db querier, id string) (*M, error) {
	var v M

	err := db.QueryRowContext(ctx, `SELECT id, name, created_at, updated_at, status FROM m WHERE id = $1`, id).
		Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status)
	if err != nil {
		return nil, err
	}

	return &v, nil
}

func Get(c *gofr.Context) (any, error) {
	return GetByID(c, c.SQL, c.PathParam("id"))
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var v M
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	v.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err := c.SQL.ExecContext(c, `UPDATE m SET name = $1, updated_at = $2, status = $3 WHERE id = $4`,
		v.Name, v.UpdatedAt, v.Status, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("m successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	_, err := c.SQL.ExecContext(c, `DELETE FROM m WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("m successfully deleted with id: %s", id), nil
}
