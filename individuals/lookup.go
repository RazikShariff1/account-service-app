package individuals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"gofr.dev/pkg/gofr"
	gofrSQL "gofr.dev/pkg/gofr/datasource/sql"

	"main/address"
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

func fetchAddress(ctx context.Context, tx *gofrSQL.Tx, id int) (*Address, error) {
	v, err := address.GetByID(ctx, tx, strconv.Itoa(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errReferenceNotFound
		}

		return nil, err
	}

	r, err := fetchRoad(ctx, tx, v.RoadId)
	if err != nil {
		return nil, err
	}

	return &Address{
		Id:        v.Id,
		Road:      *r,
		DoorNo:    v.DoorNo,
		Landmark:  v.Landmark,
		City:      v.City,
		State:     v.State,
		Pincode:   v.Pincode,
		Country:   v.Country,
		Latitude:  v.Latitude,
		Longitude: v.Longitude,
	}, nil
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

// enrichIndividual resolves the h_id/m_id/r_id/address_id already scanned onto
// resp.{Halqa,Masjid,Road,Address}.Id against the primary account-service
// tables, all within one transaction (see withPrimaryTx).
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

		addr, err := fetchAddress(c, tx, resp.Address.Id)
		if err != nil {
			return err
		}

		resp.Address = *addr

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
// multiplies latency to the primary db by the row count.
func enrichIndividuals(c *gofr.Context, list []*IndividualResponse) error {
	if len(list) == 0 {
		return nil
	}

	return withPrimaryTx(c.SQL, func(tx *gofrSQL.Tx) error {
		hIDs := make([]int, len(list))
		mIDs := make([]int, len(list))
		rIDs := make([]int, len(list))
		aIDs := make([]int, len(list))

		for i, ind := range list {
			hIDs[i] = ind.Halqa.Id
			mIDs[i] = ind.Masjid.Id
			rIDs[i] = ind.Road.Id
			aIDs[i] = ind.Address.Id
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

		masjidByID := make(map[int]Masjid, len(ms))
		for _, v := range ms {
			masjidByID[v.Id] = Masjid{Id: v.Id, Name: v.Name}
		}

		rs, err := road.GetByIDs(tx, uniqueInts(rIDs))
		if err != nil {
			return err
		}

		roadByID := make(map[int]Road, len(rs))
		for _, v := range rs {
			roadByID[v.Id] = Road{Id: v.Id, Name: v.Name}
		}

		addrs, err := address.GetByIDs(tx, uniqueInts(aIDs))
		if err != nil {
			return err
		}

		// Addresses reference their own road_id, which may not overlap with
		// rIDs above — fetch whatever's missing.
		var missingRoadIDs []int

		for _, a := range addrs {
			if _, ok := roadByID[a.RoadId]; !ok {
				missingRoadIDs = append(missingRoadIDs, a.RoadId)
			}
		}

		if len(missingRoadIDs) > 0 {
			more, err := road.GetByIDs(tx, uniqueInts(missingRoadIDs))
			if err != nil {
				return err
			}

			for _, v := range more {
				roadByID[v.Id] = Road{Id: v.Id, Name: v.Name}
			}
		}

		addressByID := make(map[int]Address, len(addrs))
		for _, a := range addrs {
			addressByID[a.Id] = Address{
				Id:        a.Id,
				Road:      roadByID[a.RoadId],
				DoorNo:    a.DoorNo,
				Landmark:  a.Landmark,
				City:      a.City,
				State:     a.State,
				Pincode:   a.Pincode,
				Country:   a.Country,
				Latitude:  a.Latitude,
				Longitude: a.Longitude,
			}
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

			addr, ok := addressByID[ind.Address.Id]
			if !ok {
				return fmt.Errorf("individuals: address_id %d not found", ind.Address.Id)
			}

			ind.Address = addr
		}

		return nil
	})
}
