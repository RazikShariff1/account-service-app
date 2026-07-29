package accounts

import (
	"fmt"
	"net/mail"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"
)

const minPasswordLength = 8

type Account struct {
	Id           int    `json:"id" sql:"auto_increment"`
	Email        string `json:"email" sql:"not_null"`
	Password     string `json:"password,omitempty"`
	PasswordHash string `json:"-" sql:"not_null"`
	Name         string `json:"name"`
	PhoneNumber  string `json:"phone_number"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func RegisterRoutes(a *gofr.App) {
	a.POST("/accounts", Create)
	a.GET("/accounts", GetAll)
	a.GET("/accounts/{id}", Get)
	a.PUT("/accounts/{id}", Update)
	a.DELETE("/accounts/{id}", Delete)
}

func validateAccount(acc *Account, requirePassword bool) error {
	var missing []string

	if acc.Email == "" {
		missing = append(missing, "email")
	}

	if acc.Name == "" {
		missing = append(missing, "name")
	}

	if requirePassword && acc.Password == "" {
		missing = append(missing, "password")
	}

	if len(missing) > 0 {
		return gofrHTTP.ErrorMissingParam{Params: missing}
	}

	if _, err := mail.ParseAddress(acc.Email); err != nil {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"email"}}
	}

	if acc.Password != "" && len(acc.Password) < minPasswordLength {
		return gofrHTTP.ErrorInvalidParam{Params: []string{"password"}}
	}

	return nil
}

func Create(c *gofr.Context) (any, error) {
	var acc Account
	if err := c.Bind(&acc); err != nil {
		return nil, err
	}

	if err := validateAccount(&acc, true); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(acc.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	acc.PasswordHash = string(hash)
	acc.Password = ""

	now := time.Now().UTC().Format(time.RFC3339)
	acc.CreatedAt = now
	acc.UpdatedAt = now

	err = c.SQL.QueryRowContext(c,
		`INSERT INTO accounts (email, password_hash, name, phone_number, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		acc.Email, acc.PasswordHash, acc.Name, acc.PhoneNumber, acc.Status, acc.CreatedAt, acc.UpdatedAt).
		Scan(&acc.Id)
	if err != nil {
		return nil, err
	}

	acc.PasswordHash = ""

	return acc, nil
}

func GetAll(c *gofr.Context) (any, error) {
	rows, err := c.SQL.QueryContext(c,
		`SELECT id, email, name, phone_number, status, created_at, updated_at FROM accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Account

	for rows.Next() {
		var acc Account

		if err := rows.Scan(&acc.Id, &acc.Email, &acc.Name, &acc.PhoneNumber,
			&acc.Status, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
			return nil, err
		}

		result = append(result, acc)
	}

	return result, nil
}

func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var acc Account

	err := c.SQL.QueryRowContext(c,
		`SELECT id, email, name, phone_number, status, created_at, updated_at FROM accounts WHERE id = $1`, id).
		Scan(&acc.Id, &acc.Email, &acc.Name, &acc.PhoneNumber, &acc.Status, &acc.CreatedAt, &acc.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return acc, nil
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	var acc Account
	if err := c.Bind(&acc); err != nil {
		return nil, err
	}

	if err := validateAccount(&acc, false); err != nil {
		return nil, err
	}

	acc.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if acc.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(acc.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}

		_, err = c.SQL.ExecContext(c,
			`UPDATE accounts SET email = $1, password_hash = $2, name = $3, phone_number = $4, status = $5,
			updated_at = $6 WHERE id = $7`,
			acc.Email, string(hash), acc.Name, acc.PhoneNumber, acc.Status, acc.UpdatedAt, id)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := c.SQL.ExecContext(c,
			`UPDATE accounts SET email = $1, name = $2, phone_number = $3, status = $4,
			updated_at = $5 WHERE id = $6`,
			acc.Email, acc.Name, acc.PhoneNumber, acc.Status, acc.UpdatedAt, id)
		if err != nil {
			return nil, err
		}
	}

	return fmt.Sprintf("account successfully updated with id: %s", id), nil
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	_, err := c.SQL.ExecContext(c, `DELETE FROM accounts WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("account successfully deleted with id: %s", id), nil
}
