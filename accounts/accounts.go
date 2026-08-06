package accounts

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/h"
	"main/jwt"
	"main/m"
	"main/middleware"
)

const minPasswordLength = 8

// multiQuerier is satisfied by both c.SQL and a *sql.Tx (gofr's Tx only
// exposes a context-less Query, unlike its QueryRowContext).
type multiQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// GetByIDs looks up several accounts by id in a single round trip, for
// callers (e.g. individuals' activity log) that need the acting account's
// full details, not just its id. Password/PasswordHash are never populated.
func GetByIDs(db multiQuerier, ids []int) ([]Account, error) {
	rows, err := db.Query(
		`SELECT id, email, name, phone_number, status, h_id, m_id, h_admin, m_admin, created_at, updated_at
		FROM accounts WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Account

	for rows.Next() {
		var acc Account

		if err := rows.Scan(&acc.Id, &acc.Email, &acc.Name, &acc.PhoneNumber,
			&acc.Status, &acc.HId, &acc.MId, &acc.HAdmin, &acc.MAdmin, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
			return nil, err
		}

		result = append(result, acc)
	}

	return result, rows.Err()
}

type Account struct {
	Id             int     `json:"id" sql:"auto_increment"`
	Email          string  `json:"email" sql:"not_null"`
	Password       string  `json:"password,omitempty"`
	PasswordHash   string  `json:"-" sql:"not_null"`
	Name           string  `json:"name"`
	PhoneNumber    string  `json:"phone_number"`
	Status         string  `json:"status"`
	HId            int     `json:"h_id"`
	MId            int     `json:"m_id"`
	HAdmin         bool    `json:"h_admin"`
	MAdmin         bool    `json:"m_admin"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	LastLoggedInAt *string `json:"last_logged_in_at,omitempty"`
}

func RegisterRoutes(a *gofr.App) {
	a.POST("/login", Login)
	a.POST("/signup", Signup)
	a.POST("/accounts", Create)
	a.GET("/accounts", GetAll)
	a.GET("/accounts/{id}", Get)
	a.PUT("/accounts/{id}", Update)
	a.PUT("/accounts/{id}/email-status", EmailVerified)
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

	if requirePassword && acc.HId == 0 {
		missing = append(missing, "h_id")
	}

	if requirePassword && acc.MId == 0 {
		missing = append(missing, "m_id")
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

// validateReferenceIDs checks h_id/m_id against the h/m tables, when set.
func validateReferenceIDs(c *gofr.Context, acc *Account) error {
	var invalid []string

	if acc.HId != 0 {
		if _, err := h.GetByID(c, c.SQL, strconv.Itoa(acc.HId)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				invalid = append(invalid, "h_id")
			} else {
				return err
			}
		}
	}

	if acc.MId != 0 {
		if _, err := m.GetByID(c, c.SQL, strconv.Itoa(acc.MId)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				invalid = append(invalid, "m_id")
			} else {
				return err
			}
		}
	}

	if len(invalid) > 0 {
		return gofrHTTP.ErrorInvalidParam{Params: invalid}
	}

	return nil
}

// validateAdminUniqueness rejects h_admin/m_admin being set on a new account
// when the target h/m already has an admin, since each h and m may have at
// most one admin account.
func validateAdminUniqueness(c *gofr.Context, acc *Account) error {
	var invalid []string

	if acc.HAdmin {
		var exists bool
		if err := c.SQL.QueryRowContext(c,
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE h_id = $1 AND h_admin = true)`, acc.HId).Scan(&exists); err != nil {
			return err
		}

		if exists {
			invalid = append(invalid, "h_admin")
		}
	}

	if acc.MAdmin {
		var exists bool
		if err := c.SQL.QueryRowContext(c,
			`SELECT EXISTS(SELECT 1 FROM accounts WHERE m_id = $1 AND m_admin = true)`, acc.MId).Scan(&exists); err != nil {
			return err
		}

		if exists {
			invalid = append(invalid, "m_admin")
		}
	}

	if len(invalid) > 0 {
		return gofrHTTP.ErrorInvalidParam{Params: invalid}
	}

	return nil
}

type ErrInvalidCredentials struct{}

func (ErrInvalidCredentials) Error() string {
	return "invalid email or password"
}

func (ErrInvalidCredentials) StatusCode() int {
	return http.StatusUnauthorized
}

type ErrUnauthorized struct{}

func (ErrUnauthorized) Error() string {
	return "missing request claims"
}

func (ErrUnauthorized) StatusCode() int {
	return http.StatusUnauthorized
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token          string  `json:"token"`
	AccountDetails Account `json:"account_details"`
}

func Login(c *gofr.Context) (any, error) {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	var (
		acc          Account
		passwordHash string
		hID, mID     sql.NullInt32
	)

	err := c.SQL.QueryRowContext(c,
		`SELECT id, email, name, phone_number, status, password_hash, h_id, m_id, h_admin, m_admin, created_at, updated_at
		FROM accounts WHERE email = $1`, req.Email).
		Scan(&acc.Id, &acc.Email, &acc.Name, &acc.PhoneNumber, &acc.Status, &passwordHash,
			&hID, &mID, &acc.HAdmin, &acc.MAdmin, &acc.CreatedAt, &acc.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredentials{}
		}

		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials{}
	}

	acc.HId, acc.MId = int(hID.Int32), int(mID.Int32)

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := c.SQL.ExecContext(c,
		`UPDATE accounts SET last_logged_in_at = $1 WHERE id = $2`, now, acc.Id); err != nil {
		return nil, err
	}

	acc.LastLoggedInAt = &now

	token, err := jwt.Generate(acc.Id, acc.HId, acc.MId)
	if err != nil {
		return nil, err
	}

	return loginResponse{Token: token, AccountDetails: acc}, nil
}

// hashAndInsertAccount hashes acc.Password and inserts the account row,
// populating acc.Id and clearing acc.Password/PasswordHash afterward.
func hashAndInsertAccount(c *gofr.Context, acc *Account) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(acc.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	acc.PasswordHash = string(hash)
	acc.Password = ""

	now := time.Now().UTC().Format(time.RFC3339)
	acc.CreatedAt = now
	acc.UpdatedAt = now

	err = c.SQL.QueryRowContext(c,
		`INSERT INTO accounts (email, password_hash, name, phone_number, status, h_id, m_id, h_admin, m_admin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		acc.Email, acc.PasswordHash, acc.Name, acc.PhoneNumber, acc.Status, acc.HId, acc.MId, acc.HAdmin, acc.MAdmin,
		acc.CreatedAt, acc.UpdatedAt).
		Scan(&acc.Id)
	if err != nil {
		return err
	}

	acc.PasswordHash = ""

	return nil
}

func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate")
}

type signupResponse struct {
	Account Account `json:"account"`
	Token   string  `json:"token"`
}

// Signup is the public, unauthenticated counterpart to Create: it lets a new
// user register directly, taking h_id/m_id from the request body since there
// are no claims yet to derive them from.
func Signup(c *gofr.Context) (any, error) {
	var acc Account
	if err := c.Bind(&acc); err != nil {
		return nil, err
	}

	if err := validateAccount(&acc, true); err != nil {
		return nil, err
	}

	if err := validateReferenceIDs(c, &acc); err != nil {
		return nil, err
	}

	if err := validateAdminUniqueness(c, &acc); err != nil {
		return nil, err
	}

	if err := hashAndInsertAccount(c, &acc); err != nil {
		if isUniqueViolation(err) {
			return nil, gofrHTTP.ErrorEntityAlreadyExist{}
		}

		return nil, err
	}

	token, err := jwt.Generate(acc.Id, acc.HId, acc.MId)
	if err != nil {
		return nil, err
	}

	return signupResponse{Account: acc, Token: token}, nil
}

func Create(c *gofr.Context) (any, error) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var acc Account
	if err := c.Bind(&acc); err != nil {
		return nil, err
	}

	acc.HId = claims.HId
	acc.MId = claims.MId

	if err := validateAccount(&acc, true); err != nil {
		return nil, err
	}

	if err := validateReferenceIDs(c, &acc); err != nil {
		return nil, err
	}

	if err := validateAdminUniqueness(c, &acc); err != nil {
		return nil, err
	}

	if err := hashAndInsertAccount(c, &acc); err != nil {
		if isUniqueViolation(err) {
			return nil, gofrHTTP.ErrorEntityAlreadyExist{}
		}

		return nil, err
	}

	return acc, nil
}

// GetAll lists accounts under the caller's own m_id, or, when hId/mId is
// given via the h_id/m_id query param, every account under that hierarchy
// node instead — used by other services (e.g. communication-svc) to resolve
// a hierarchy/merchant broadcast target down to the account ids it covers.
func GetAll(c *gofr.Context) (any, error) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var (
		rows *sql.Rows
		err  error
	)

	switch {
	case c.Param("h_id") != "":
		hID, convErr := strconv.Atoi(c.Param("h_id"))
		if convErr != nil {
			return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"h_id"}}
		}

		rows, err = c.SQL.QueryContext(c,
			`SELECT id, email, name, phone_number, status, h_id, m_id, h_admin, m_admin, created_at, updated_at
			FROM accounts WHERE h_id = $1`, hID)
	case c.Param("m_id") != "":
		mID, convErr := strconv.Atoi(c.Param("m_id"))
		if convErr != nil {
			return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"m_id"}}
		}

		rows, err = c.SQL.QueryContext(c,
			`SELECT id, email, name, phone_number, status, h_id, m_id, h_admin, m_admin, created_at, updated_at
			FROM accounts WHERE m_id = $1`, mID)
	default:
		rows, err = c.SQL.QueryContext(c,
			`SELECT id, email, name, phone_number, status, h_id, m_id, h_admin, m_admin, created_at, updated_at
			FROM accounts WHERE m_id = $1`, claims.MId)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Account

	for rows.Next() {
		var acc Account

		if err := rows.Scan(&acc.Id, &acc.Email, &acc.Name, &acc.PhoneNumber,
			&acc.Status, &acc.HId, &acc.MId, &acc.HAdmin, &acc.MAdmin, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
			return nil, err
		}

		result = append(result, acc)
	}

	return result, nil
}

func Get(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var acc Account

	err := c.SQL.QueryRowContext(c,
		`SELECT id, email, name, phone_number, status, h_id, m_id, h_admin, m_admin, created_at, updated_at
		FROM accounts WHERE id = $1 AND m_id = $2`, id, claims.MId).
		Scan(&acc.Id, &acc.Email, &acc.Name, &acc.PhoneNumber, &acc.Status, &acc.HId, &acc.MId, &acc.HAdmin, &acc.MAdmin,
			&acc.CreatedAt, &acc.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gofrHTTP.ErrorEntityNotFound{Name: "id", Value: id}
		}

		return nil, err
	}

	return acc, nil
}

func Update(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var acc Account
	if err := c.Bind(&acc); err != nil {
		return nil, err
	}

	acc.HId = claims.HId
	acc.MId = claims.MId

	if err := validateAccount(&acc, false); err != nil {
		return nil, err
	}

	if err := validateReferenceIDs(c, &acc); err != nil {
		return nil, err
	}

	acc.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	var result sql.Result

	var err error

	if acc.Password != "" {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(acc.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, hashErr
		}

		result, err = c.SQL.ExecContext(c,
			`UPDATE accounts SET email = $1, password_hash = $2, name = $3, phone_number = $4, status = $5,
			h_id = $6, m_id = $7, h_admin = $8, m_admin = $9, updated_at = $10 WHERE id = $11 AND m_id = $12`,
			acc.Email, string(hash), acc.Name, acc.PhoneNumber, acc.Status, acc.HId, acc.MId, acc.HAdmin, acc.MAdmin,
			acc.UpdatedAt, id, claims.MId)
	} else {
		result, err = c.SQL.ExecContext(c,
			`UPDATE accounts SET email = $1, name = $2, phone_number = $3, status = $4,
			h_id = $5, m_id = $6, h_admin = $7, m_admin = $8, updated_at = $9 WHERE id = $10 AND m_id = $11`,
			acc.Email, acc.Name, acc.PhoneNumber, acc.Status, acc.HId, acc.MId, acc.HAdmin, acc.MAdmin, acc.UpdatedAt,
			id, claims.MId)
	}

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

	return fmt.Sprintf("account successfully updated with id: %s", id), nil
}

// EmailVerified stamps email_status=true on an account once its owner has
// confirmed their email address via the link sent by communication-svc.
func EmailVerified(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	result, err := c.SQL.ExecContext(c,
		`UPDATE accounts SET email_status = true, updated_at = $1 WHERE id = $2 AND m_id = $3`,
		time.Now().UTC().Format(time.RFC3339), id, claims.MId)
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

	return Get(c)
}

func Delete(c *gofr.Context) (any, error) {
	id := c.PathParam("id")

	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	result, err := c.SQL.ExecContext(c, `DELETE FROM accounts WHERE id = $1 AND m_id = $2`, id, claims.MId)
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

	return fmt.Sprintf("account successfully deleted with id: %s", id), nil
}
