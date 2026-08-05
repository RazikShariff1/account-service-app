package individuals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"

	"main/h"
	"main/m"
	"main/road"
)

var errReferenceNotFound = errors.New("individuals: reference not found")

func fetchHalqa(ctx context.Context, tx *gofrSQL.Tx, id int) (*Halqa, error) {
	v, err := h.GetByID(ctx, tx, strconv.Itoa(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errReferenceNotFound
		}

		return nil, err
	}

	return &Halqa{Id: v.Id, Name: v.Name}, nil
}

func fetchMasjid(ctx context.Context, tx *gofrSQL.Tx, id int) (*Masjid, error) {
	v, err := m.GetByID(ctx, tx, strconv.Itoa(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errReferenceNotFound
		}

		return nil, err
	}

	return &Masjid{Id: v.Id, Name: v.Name}, nil
}

func fetchRoad(ctx context.Context, tx *gofrSQL.Tx, id int) (*Road, error) {
	v, err := road.GetByID(ctx, tx, strconv.Itoa(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errReferenceNotFound
		}

		return nil, err
	}

	return &Road{Id: v.Id, Name: v.Name}, nil
}

// withPrimaryTx runs fn against a fresh transaction on the primary db,
// committing on success and rolling back on error.
//
// A transaction is required here, not just nice-to-have: Neon's PgBouncer
// pooler runs in transaction-pooling mode, which can silently reassign the
// backend server between separate autocommitted statements on the same
// pooled connection. Several distinct parameterized queries issued back to
// back outside a transaction intermittently fail with "pq: unnamed prepared
// statement does not exist" once the backend swaps mid-sequence. Wrapping
// the whole sequence in one transaction pins a single backend for its
// duration and avoids this.
func withPrimaryTx(db querier, fn func(tx *gofrSQL.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

type querier interface {
	Begin() (*gofrSQL.Tx, error)
}

// enrichIndividual resolves the h_id/m_id/r_id already scanned onto
// resp.{Halqa,Masjid,Road}.Id against the primary account-service tables,
// all within one transaction (see withPrimaryTx).
func enrichIndividual(c *gofr.Context, resp *IndividualResponse) error {
	return withPrimaryTx(c.SQL, func(tx *gofrSQL.Tx) error {
		halqa, err := fetchHalqa(c, tx, resp.Halqa.Id)
		if err != nil {
			return err
		}

		resp.Halqa = *halqa

		masjid, err := fetchMasjid(c, tx, resp.Masjid.Id)
		if err != nil {
			return err
		}

		resp.Masjid = *masjid

		rd, err := fetchRoad(c, tx, resp.Road.Id)
		if err != nil {
			return err
		}

		resp.Road = *rd

		return nil
	})
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

// enrichIndividuals resolves references for a whole list in a handful of
// batched queries instead of one enrichIndividual call per row: GetAll can
// return dozens of rows, and per-row enrichment (4+ round trips each)
// multiplies latency to the primary db by the row count. When includeHData
// is set, each Masjid is additionally populated with the halqa it belongs
// to (masjid.h_id/h_name), for callers that need that context without a
// second request.
func enrichIndividuals(c *gofr.Context, list []*IndividualResponse, includeHData bool) error {
	if len(list) == 0 {
		return nil
	}

	return withPrimaryTx(c.SQL, func(tx *gofrSQL.Tx) error {
		hIDs := make([]int, len(list))
		mIDs := make([]int, len(list))
		rIDs := make([]int, len(list))

		for i, ind := range list {
			hIDs[i] = ind.Halqa.Id
			mIDs[i] = ind.Masjid.Id
			rIDs[i] = ind.Road.Id
		}

		hs, err := h.GetByIDs(tx, uniqueInts(hIDs))
		if err != nil {
			return err
		}

		halqaByID := make(map[int]Halqa, len(hs))
		for _, v := range hs {
			halqaByID[v.Id] = Halqa{Id: v.Id, Name: v.Name}
		}

		ms, err := m.GetByIDs(tx, uniqueInts(mIDs))
		if err != nil {
			return err
		}

		var masjidHalqaNameByID map[int]string

		if includeHData {
			var masjidHIDs []int

			for _, v := range ms {
				if v.HId != 0 {
					masjidHIDs = append(masjidHIDs, v.HId)
				}
			}

			masjidHs, err := h.GetByIDs(tx, uniqueInts(masjidHIDs))
			if err != nil {
				return err
			}

			masjidHalqaNameByID = make(map[int]string, len(masjidHs))
			for _, v := range masjidHs {
				masjidHalqaNameByID[v.Id] = v.Name
			}
		}

		masjidByID := make(map[int]Masjid, len(ms))
		for _, v := range ms {
			masjid := Masjid{Id: v.Id, Name: v.Name}

			if includeHData && v.HId != 0 {
				hID := v.HId
				masjid.HId = &hID

				if name, ok := masjidHalqaNameByID[v.HId]; ok {
					masjid.HName = &name
				}
			}

			masjidByID[v.Id] = masjid
		}

		rs, err := road.GetByIDs(tx, uniqueInts(rIDs))
		if err != nil {
			return err
		}

		roadByID := make(map[int]Road, len(rs))
		for _, v := range rs {
			roadByID[v.Id] = Road{Id: v.Id, Name: v.Name}
		}

		for _, ind := range list {
			halqa, ok := halqaByID[ind.Halqa.Id]
			if !ok {
				return fmt.Errorf("individuals: h_id %d not found", ind.Halqa.Id)
			}

			ind.Halqa = halqa

			masjid, ok := masjidByID[ind.Masjid.Id]
			if !ok {
				return fmt.Errorf("individuals: m_id %d not found", ind.Masjid.Id)
			}

			ind.Masjid = masjid

			rd, ok := roadByID[ind.Road.Id]
			if !ok {
				return fmt.Errorf("individuals: r_id %d not found", ind.Road.Id)
			}

			ind.Road = rd
		}

		return nil
	})
}
