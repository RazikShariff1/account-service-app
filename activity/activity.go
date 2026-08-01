package activity

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/accounts"
	"main/middleware"
	"main/secondarydb"
)

type ErrUnauthorized struct{}

func (ErrUnauthorized) Error() string { return "missing request claims" }

func (ErrUnauthorized) StatusCode() int { return http.StatusUnauthorized }

func RegisterRoutes(a *gofr.App) {
	a.POST("/activity", Create)
	a.GET("/activity/{id}", GetAll)
}

type createRequest struct {
	IndividualId int             `json:"individual_id"`
	LogName      string          `json:"log_name"`
	OldValue     json.RawMessage `json:"old_value"`
	NewValue     json.RawMessage `json:"new_value"`
}

// Log is a single activity log entry. OldValue/NewValue are free-form JSON —
// callers decide what shape fits the event being recorded.
type Log struct {
	Id           int             `json:"id"`
	IndividualId int             `json:"individual_id"`
	AccountId    *int            `json:"account_id"`
	AccountName  string          `json:"account_name,omitempty"`
	LogName      string          `json:"log_name"`
	OldValue     json.RawMessage `json:"old_value,omitempty"`
	NewValue     json.RawMessage `json:"new_value,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Create records one activity log entry against an individual. The acting
// account is always taken from the caller's token, not the request body, so
// a log entry can't be forged as having been made by someone else.
func Create(c *gofr.Context) (any, error) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		return nil, ErrUnauthorized{}
	}

	var req createRequest
	if err := c.Bind(&req); err != nil {
		return nil, err
	}

	if req.IndividualId == 0 {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"individual_id"}}
	}

	if req.LogName == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"log_name"}}
	}

	log := Log{
		IndividualId: req.IndividualId,
		AccountId:    &claims.AId,
		LogName:      req.LogName,
		OldValue:     req.OldValue,
		NewValue:     req.NewValue,
	}

	err := secondarydb.DB.QueryRowContext(c,
		`INSERT INTO activity_logs (individual_id, account_id, log_name, old_value, new_value)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		log.IndividualId, claims.AId, log.LogName, rawOrNil(log.OldValue), rawOrNil(log.NewValue)).
		Scan(&log.Id, &log.CreatedAt)
	if err != nil {
		return nil, err
	}

	return log, nil
}

func rawOrNil(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}

	return []byte(raw)
}

// GetAll returns every activity log entry for the given individual, newest
// first, with each entry's acting account name resolved from the primary
// database (activity_logs.account_id has no FK there — accounts lives in a
// separate physical database).
func GetAll(c *gofr.Context) (any, error) {
	individualID := c.PathParam("id")

	if _, err := strconv.Atoi(individualID); err != nil {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"id"}}
	}

	rows, err := secondarydb.DB.QueryContext(c,
		`SELECT id, individual_id, account_id, log_name, old_value, new_value, created_at
		FROM activity_logs WHERE individual_id = $1 ORDER BY created_at DESC`, individualID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		result     []*Log
		accountIDs []int
	)

	for rows.Next() {
		var (
			l                  Log
			accountID          sql.NullInt32
			oldValue, newValue []byte
		)

		if err := rows.Scan(&l.Id, &l.IndividualId, &accountID, &l.LogName, &oldValue, &newValue, &l.CreatedAt); err != nil {
			return nil, err
		}

		if oldValue != nil {
			l.OldValue = oldValue
		}

		if newValue != nil {
			l.NewValue = newValue
		}

		if accountID.Valid {
			id := int(accountID.Int32)
			l.AccountId = &id
			accountIDs = append(accountIDs, id)
		}

		result = append(result, &l)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(accountIDs) > 0 {
		accs, err := accounts.GetByIDs(c.SQL, uniqueInts(accountIDs))
		if err != nil {
			return nil, err
		}

		nameByID := make(map[int]string, len(accs))
		for _, a := range accs {
			nameByID[a.Id] = a.Name
		}

		for _, l := range result {
			if l.AccountId != nil {
				l.AccountName = nameByID[*l.AccountId]
			}
		}
	}

	return result, nil
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
