package m

import (
	"fmt"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"
)

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

func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var v M

	err := c.SQL.QueryRowContext(c, `SELECT id, name, created_at, updated_at, status FROM m WHERE id = $1`, id).
		Scan(&v.Id, &v.Name, &v.CreatedAt, &v.UpdatedAt, &v.Status)
	if err != nil {
		return nil, err
	}

	return v, nil
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

	_, err := c.SQL.ExecContext(c, `UPDATE m SET name = $1, created_at = $2, updated_at = $3, status = $4 WHERE id = $5`,
		v.Name, v.CreatedAt, v.UpdatedAt, v.Status, id)
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
