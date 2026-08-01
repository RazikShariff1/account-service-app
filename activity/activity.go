package activity

import (
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
	a.GET("/activity", GetAll)
}

type createRequest struct {
	IndividualId int             `json:"individual_id"`
	LogName      string          `json:"log_name"`
	OldValue     json.RawMessage `json:"old_value"`
	NewValue     json.RawMessage `json:"new_value"`
}

// Log is a single activity log entry. OldValue/NewValue are free-form JSON —
// callers decide what shape fits the event being recorded. AccountDetails is
// a full snapshot of the acting account taken at write time (see Create),
// not a live lookup, so it still reflects the account as it was even if the
// account is later renamed, reassigned, or deleted.
type Log struct {
	Id             int             `json:"id"`
	IndividualId   int             `json:"individual_id"`
	AccountId      *int            `json:"account_id"`
	AccountDetails json.RawMessage `json:"account_details,omitempty"`
	LogName        string          `json:"log_name"`
	OldValue       json.RawMessage `json:"old_value,omitempty"`
	NewValue       json.RawMessage `json:"new_value,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Create records one activity log entry against an individual. The acting
// account is always taken from the caller's token, not the request body, so
// a log entry can't be forged as having been made by someone else. The
// account's full details are fetched once here and persisted alongside the
// log row, rather than resolved on every read.
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

	accs, err := accounts.GetByIDs(c.SQL, []int{claims.AId})
	if err != nil {
		return nil, err
	}

	if len(accs) == 0 {
		return nil, gofrHTTP.ErrorEntityNotFound{Name: "account_id", Value: strconv.Itoa(claims.AId)}
	}

	accountDetails, err := json.Marshal(accs[0])
	if err != nil {
		return nil, err
	}

	log := Log{
		IndividualId:   req.IndividualId,
		AccountId:      &claims.AId,
		AccountDetails: accountDetails,
		LogName:        req.LogName,
		OldValue:       req.OldValue,
		NewValue:       req.NewValue,
	}

	err = secondarydb.DB.QueryRowContext(c,
		`INSERT INTO activity_logs (individual_id, account_id, account_details, log_name, old_value, new_value)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		log.IndividualId, claims.AId, []byte(accountDetails), log.LogName, rawOrNil(log.OldValue), rawOrNil(log.NewValue)).
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

// GetAll returns every activity log entry for the individual given by the
// mandatory ?id= query param, newest first.
func GetAll(c *gofr.Context) (any, error) {
	individualID := c.Param("id")
	if individualID == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"id"}}
	}

	if _, err := strconv.Atoi(individualID); err != nil {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"id"}}
	}

	rows, err := secondarydb.DB.QueryContext(c,
		`SELECT id, individual_id, account_id, account_details, log_name, old_value, new_value, created_at
		FROM activity_logs WHERE individual_id = $1 ORDER BY created_at DESC`, individualID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*Log, 0)

	for rows.Next() {
		var (
			l                                  Log
			accountID                          *int
			accountDetails, oldValue, newValue []byte
		)

		if err := rows.Scan(&l.Id, &l.IndividualId, &accountID, &accountDetails, &l.LogName, &oldValue, &newValue, &l.CreatedAt); err != nil {
			return nil, err
		}

		l.AccountId = accountID

		if accountDetails != nil {
			l.AccountDetails = accountDetails
		}

		if oldValue != nil {
			l.OldValue = oldValue
		}

		if newValue != nil {
			l.NewValue = newValue
		}

		result = append(result, &l)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
