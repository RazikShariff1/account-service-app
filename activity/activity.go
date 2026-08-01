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
	Notes        string          `json:"notes"`
}

// Log is a single activity log entry. OldValue/NewValue are free-form JSON —
// callers decide what shape fits the event being recorded. AccountDetails is
// resolved fresh at read time (see GetAll) from the account_id stored on the
// row, not persisted, so it always reflects the account's current name/etc.
type Log struct {
	Id             int               `json:"id"`
	IndividualId   int               `json:"individual_id"`
	AccountId      *int              `json:"account_id"`
	AccountDetails *accounts.Account `json:"account_details,omitempty"`
	LogName        string            `json:"log_name"`
	OldValue       json.RawMessage   `json:"old_value,omitempty"`
	NewValue       json.RawMessage   `json:"new_value,omitempty"`
	Notes          string            `json:"notes,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

// Create records one activity log entry against an individual. The acting
// account is always taken from the caller's token, not the request body, so
// a log entry can't be forged as having been made by someone else. Only the
// account id is stored — full details are resolved on read (see GetAll).
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

	return Record(c, req.IndividualId, &claims.AId, req.LogName, req.OldValue, req.NewValue, req.Notes)
}

// Record inserts a single activity log entry. It is shared by the HTTP Create
// handler above and by other packages that need to log an event as a side
// effect of their own operation (e.g. individuals.Create logging the
// individual's creation) without going through the HTTP layer.
func Record(
	ctx *gofr.Context, individualID int, accountID *int, logName string, oldValue, newValue json.RawMessage, notes string,
) (*Log, error) {
	log := Log{
		IndividualId: individualID,
		AccountId:    accountID,
		LogName:      logName,
		OldValue:     oldValue,
		NewValue:     newValue,
		Notes:        notes,
	}

	err := secondarydb.DB.QueryRowContext(ctx,
		`INSERT INTO activity_logs (individual_id, account_id, log_name, old_value, new_value, notes)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		log.IndividualId, accountID, log.LogName, rawOrNil(log.OldValue), rawOrNil(log.NewValue), stringOrNil(log.Notes)).
		Scan(&log.Id, &log.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &log, nil
}

func rawOrNil(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}

	return []byte(raw)
}

func stringOrNil(s string) any {
	if s == "" {
		return nil
	}

	return s
}

// GetAll returns every activity log entry for the individual given by the
// mandatory ?id= query param, newest first, with each entry's acting account
// resolved to its full details from the primary database (activity_logs has
// no FK there — accounts lives in a separate physical database).
func GetAll(c *gofr.Context) (any, error) {
	individualID := c.Param("id")
	if individualID == "" {
		return nil, gofrHTTP.ErrorMissingParam{Params: []string{"id"}}
	}

	if _, err := strconv.Atoi(individualID); err != nil {
		return nil, gofrHTTP.ErrorInvalidParam{Params: []string{"id"}}
	}

	rows, err := secondarydb.DB.QueryContext(c,
		`SELECT id, individual_id, account_id, log_name, old_value, new_value, notes, created_at
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
			accountID          *int
			oldValue, newValue []byte
			notes              *string
		)

		if err := rows.Scan(&l.Id, &l.IndividualId, &accountID, &l.LogName, &oldValue, &newValue, &notes, &l.CreatedAt); err != nil {
			return nil, err
		}

		if notes != nil {
			l.Notes = *notes
		}

		l.AccountId = accountID

		if accountID != nil {
			accountIDs = append(accountIDs, *accountID)
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

	if len(result) == 0 {
		return []*Log{}, nil
	}

	if len(accountIDs) > 0 {
		accs, err := accounts.GetByIDs(c.SQL, uniqueInts(accountIDs))
		if err != nil {
			return nil, err
		}

		accByID := make(map[int]*accounts.Account, len(accs))
		for i := range accs {
			accByID[accs[i].Id] = &accs[i]
		}

		for _, l := range result {
			if l.AccountId != nil {
				l.AccountDetails = accByID[*l.AccountId]
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
