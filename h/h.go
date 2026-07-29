package h

import (
	"fmt"
	"time"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"
)

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

func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var v H

	err := c.SQL.QueryRowContext(c, `SELECT id, name, created_at, updated_at, status FROM h WHERE id = $1`, id).
		Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var v H
	if err := c.Bind(&v); err != nil {
		return nil, err
	}

	if err := validate(&v); err != nil {
		return nil, err
	}

	v.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err := c.SQL.ExecContext(c, `UPDATE h SET name = $1, updated_at = $2, status = $3 WHERE id = $4`,
		v.Name, v.UpdatedAt, v.Status, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("h successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	_, err := c.SQL.ExecContext(c, `DELETE FROM h WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("h successfully deleted with id: %s", id), nil
}
