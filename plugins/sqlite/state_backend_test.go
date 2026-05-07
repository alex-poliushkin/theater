package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater/plugin/protocol"
	statemodel "github.com/alex-poliushkin/theater/state"

	_ "modernc.org/sqlite"
)

func TestStateBackendReadAndCAS(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newTestServerState(t)
	config := map[string]any{"profile": "app"}

	db, err := state.stateDB(ctx, config)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	defer db.Close()

	insertStateRecord(t, db, "settings", 1, map[string]any{"token": "initial"})

	readResult, err := state.readStateRecord(ctx, protocol.StateReadParams{
		Config: config,
		Key:    "settings",
	})
	if err != nil {
		t.Fatalf("read state record: %v", err)
	}
	if got, want := readResult.Snapshot.Version, "1"; got != want {
		t.Fatalf("record version mismatch: got %q want %q", got, want)
	}
	if got, want := readResult.Snapshot.Value["token"], "initial"; got != want {
		t.Fatalf("record value mismatch: got %v want %v", got, want)
	}

	casResult, err := state.compareAndSetStateRecord(ctx, protocol.StateCASParams{
		Config:          config,
		Key:             "settings",
		ExpectedVersion: "1",
		Value:           map[string]any{"token": "updated"},
	})
	if err != nil {
		t.Fatalf("cas state record: %v", err)
	}
	if got, want := casResult.Snapshot.Version, "2"; got != want {
		t.Fatalf("cas version mismatch: got %q want %q", got, want)
	}
	if got, want := casResult.Snapshot.Value["token"], "updated"; got != want {
		t.Fatalf("cas value mismatch: got %v want %v", got, want)
	}
}

func TestStateBackendClaimRenewReleaseAndConsume(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := newTestServerState(t)
	config := map[string]any{"profile": "app"}

	db, err := state.stateDB(ctx, config)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	defer db.Close()

	insertStatePoolItem(t, db, sqliteStatePoolItem{
		Pool:    "fixtures",
		ID:      "fixture-1",
		Fields:  map[string]any{"email": "fixture-1@example.test", "purpose": "registration"},
		State:   sqlitePoolStateAvailable,
		Version: 0,
	})

	claimResult, err := state.claimStatePoolItem(ctx, protocol.StateClaimParams{
		Config: config,
		Pool:   "fixtures",
		Selector: statemodel.Selector{
			Fields: map[string]string{"purpose": "registration"},
		},
		Lease: statemodel.LeaseSpec{
			TTL:          time.Minute,
			ExpiryPolicy: statemodel.ExpiryStale,
		},
	})
	if err != nil {
		t.Fatalf("claim pool item: %v", err)
	}
	if got, want := claimResult.Result.Item["email"], "fixture-1@example.test"; got != want {
		t.Fatalf("claimed item mismatch: got %v want %v", got, want)
	}

	renewResult, err := state.renewStateClaim(ctx, protocol.StateRenewParams{
		Config: config,
		Claim:  claimResult.Result.Claim,
		TTL:    2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("renew claim: %v", err)
	}
	if !renewResult.Claim.ExpiresAt.After(claimResult.Result.Claim.ExpiresAt) {
		t.Fatalf("renewed expiry was not extended: old=%s new=%s", claimResult.Result.Claim.ExpiresAt, renewResult.Claim.ExpiresAt)
	}

	if _, err := state.releaseStateClaim(ctx, protocol.StateReleaseParams{
		Config: config,
		Claim:  renewResult.Claim,
	}); err != nil {
		t.Fatalf("release claim: %v", err)
	}

	secondClaim, err := state.claimStatePoolItem(ctx, protocol.StateClaimParams{
		Config: config,
		Pool:   "fixtures",
		Selector: statemodel.Selector{
			Fields: map[string]string{"purpose": "registration"},
		},
		Lease: statemodel.LeaseSpec{
			TTL:          time.Minute,
			ExpiryPolicy: statemodel.ExpiryStale,
		},
	})
	if err != nil {
		t.Fatalf("claim after release: %v", err)
	}

	if _, err := state.consumeStateClaim(ctx, protocol.StateConsumeParams{
		Config: config,
		Claim:  secondClaim.Result.Claim,
		Tombstone: map[string]any{
			"status": "registered",
		},
	}); err != nil {
		t.Fatalf("consume claim: %v", err)
	}

	item, err := loadPoolItem(db, "fixtures", "fixture-1")
	if err != nil {
		t.Fatalf("load stored pool item: %v", err)
	}
	if got, want := item.State, sqlitePoolStateUsed; got != want {
		t.Fatalf("stored pool state mismatch: got %q want %q", got, want)
	}
	if got, want := item.Tombstone["status"], "registered"; got != want {
		t.Fatalf("stored tombstone mismatch: got %v want %v", got, want)
	}
}

func newTestServerState(t *testing.T) *serverState {
	t.Helper()

	return &serverState{
		config: sessionConfig{
			Profiles: map[string]profile{
				"app": {DSN: filepath.Join(t.TempDir(), "state.sqlite")},
			},
		},
	}
}

func insertStateRecord(t *testing.T, db *sql.DB, key string, version int64, value map[string]any) {
	t.Helper()

	rawValue, err := encodeObject(value)
	if err != nil {
		t.Fatalf("encode record value: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO "+sqliteStateRecordsTable+"(record_key, version, value_json) VALUES (?, ?, ?)",
		key,
		version,
		rawValue,
	); err != nil {
		t.Fatalf("insert record: %v", err)
	}
}

func insertStatePoolItem(t *testing.T, db *sql.DB, item sqliteStatePoolItem) {
	t.Helper()

	rawFields, err := encodeObject(item.Fields)
	if err != nil {
		t.Fatalf("encode pool fields: %v", err)
	}
	rawTombstone, err := encodeObject(item.Tombstone)
	if err != nil {
		t.Fatalf("encode pool tombstone: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO `+sqliteStatePoolItemsTable+`(
pool_name, item_id, fields_json, state, claim_id, expires_at, expiry_policy, tombstone_json, version
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Pool,
		item.ID,
		rawFields,
		item.State,
		item.ClaimID,
		item.ExpiresAt,
		item.ExpiryPolicy,
		rawTombstone,
		item.Version,
	); err != nil {
		t.Fatalf("insert pool item: %v", err)
	}
}

func loadPoolItem(db *sql.DB, pool string, id string) (sqliteStatePoolItem, error) {
	row := db.QueryRow(
		`SELECT fields_json, state, claim_id, expires_at, expiry_policy, tombstone_json, version
FROM `+sqliteStatePoolItemsTable+` WHERE pool_name = ? AND item_id = ?`,
		pool,
		id,
	)

	var (
		item         sqliteStatePoolItem
		rawFields    string
		rawTombstone string
	)
	item.Pool = pool
	item.ID = id
	if err := row.Scan(
		&rawFields,
		&item.State,
		&item.ClaimID,
		&item.ExpiresAt,
		&item.ExpiryPolicy,
		&rawTombstone,
		&item.Version,
	); err != nil {
		return sqliteStatePoolItem{}, err
	}

	fields, err := decodeObject(rawFields)
	if err != nil {
		return sqliteStatePoolItem{}, err
	}
	item.Fields = fields
	tombstone, err := decodeObject(rawTombstone)
	if err != nil {
		return sqliteStatePoolItem{}, err
	}
	item.Tombstone = tombstone
	return item, nil
}
