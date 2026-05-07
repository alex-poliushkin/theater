package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/alex-poliushkin/theater/plugin/protocol"
	statemodel "github.com/alex-poliushkin/theater/state"
)

const (
	sqliteStateRecordsTable   = "theater_state_records"
	sqliteStatePoolItemsTable = "theater_state_pool_items"

	sqlitePoolStateAvailable = "available"
	sqlitePoolStateReserved  = "reserved"
	sqlitePoolStateUsed      = "used"
)

type sqliteStateRecord struct {
	Key     string
	Value   map[string]any
	Version int64
}

type sqliteStatePoolItem struct {
	Pool         string
	ID           string
	Fields       map[string]any
	State        string
	ClaimID      string
	ExpiresAt    string
	ExpiryPolicy string
	Tombstone    map[string]any
	Version      int64
}

func (s *serverState) prepareStateBackend(ctx context.Context, params protocol.PrepareParams) (protocol.PrepareResult, error) {
	db, err := s.stateDB(ctx, params.Properties)
	if err != nil {
		return protocol.PrepareResult{}, err
	}
	defer db.Close()

	return protocol.PrepareResult{}, nil
}

func (s *serverState) readStateRecord(
	ctx context.Context,
	params protocol.StateReadParams,
) (protocol.StateReadResult, error) {
	db, err := s.stateDB(ctx, params.Config)
	if err != nil {
		return protocol.StateReadResult{}, err
	}
	defer db.Close()

	record, err := loadStateRecord(ctx, db, params.Key)
	if err != nil {
		return protocol.StateReadResult{}, err
	}

	return protocol.StateReadResult{
		Snapshot: statemodel.RecordSnapshot{
			Key:       record.Key,
			Value:     record.Value,
			Version:   strconv.FormatInt(record.Version, 10),
			Guarantee: statemodel.GuaranteeLocalAtomic,
		},
	}, nil
}

func (s *serverState) compareAndSetStateRecord(
	ctx context.Context,
	params protocol.StateCASParams,
) (protocol.StateCASResult, error) {
	db, err := s.stateDB(ctx, params.Config)
	if err != nil {
		return protocol.StateCASResult{}, err
	}
	defer db.Close()

	record, err := withImmediateTx(ctx, db, func(conn *sql.Conn) (sqliteStateRecord, error) {
		record, err := loadStateRecordConn(ctx, conn, params.Key)
		if err != nil {
			return sqliteStateRecord{}, err
		}
		if got := strconv.FormatInt(record.Version, 10); got != params.ExpectedVersion {
			return sqliteStateRecord{}, fmt.Errorf("record %q version mismatch: got %s want %s", params.Key, got, params.ExpectedVersion)
		}

		payload := cloneObject(params.Value)
		if err := writeStateRecordConn(ctx, conn, sqliteStateRecord{
			Key:     params.Key,
			Value:   payload,
			Version: record.Version + 1,
		}); err != nil {
			return sqliteStateRecord{}, err
		}

		return sqliteStateRecord{
			Key:     params.Key,
			Value:   payload,
			Version: record.Version + 1,
		}, nil
	})
	if err != nil {
		return protocol.StateCASResult{}, err
	}

	return protocol.StateCASResult{
		Snapshot: statemodel.RecordSnapshot{
			Key:       record.Key,
			Value:     record.Value,
			Version:   strconv.FormatInt(record.Version, 10),
			Guarantee: statemodel.GuaranteeLocalAtomic,
		},
	}, nil
}

func (s *serverState) claimStatePoolItem(
	ctx context.Context,
	params protocol.StateClaimParams,
) (protocol.StateClaimResult, error) {
	db, err := s.stateDB(ctx, params.Config)
	if err != nil {
		return protocol.StateClaimResult{}, err
	}
	defer db.Close()

	return claimStatePoolItemTx(ctx, db, params)
}

func (s *serverState) renewStateClaim(
	ctx context.Context,
	params protocol.StateRenewParams,
) (protocol.StateRenewResult, error) {
	db, err := s.stateDB(ctx, params.Config)
	if err != nil {
		return protocol.StateRenewResult{}, err
	}
	defer db.Close()

	renewed, err := withImmediateTx(ctx, db, func(conn *sql.Conn) (statemodel.ClaimHandle, error) {
		item, err := claimedStatePoolItem(ctx, conn, params.Claim, true)
		if err != nil {
			return statemodel.ClaimHandle{}, err
		}
		if params.TTL <= 0 {
			return statemodel.ClaimHandle{}, errors.New("ttl must be positive")
		}

		expiresAt := time.Now().UTC().Add(params.TTL)
		item.ExpiresAt = expiresAt.Format(time.RFC3339Nano)
		item.Version++
		if err := writeStatePoolItemConn(ctx, conn, item); err != nil {
			return statemodel.ClaimHandle{}, err
		}

		return statemodel.ClaimHandle{
			Pool:         params.Claim.Pool,
			ItemID:       item.ID,
			ClaimID:      item.ClaimID,
			ExpiresAt:    expiresAt,
			Version:      strconv.FormatInt(item.Version, 10),
			ExpiryPolicy: statemodel.ExpiryPolicy(item.ExpiryPolicy),
			Guarantee:    statemodel.GuaranteeLocalAtomic,
		}, nil
	})
	if err != nil {
		return protocol.StateRenewResult{}, err
	}

	return protocol.StateRenewResult{Claim: renewed}, nil
}

func (s *serverState) releaseStateClaim(
	ctx context.Context,
	params protocol.StateReleaseParams,
) (protocol.StateReleaseResult, error) {
	db, err := s.stateDB(ctx, params.Config)
	if err != nil {
		return protocol.StateReleaseResult{}, err
	}
	defer db.Close()

	_, err = withImmediateTx(ctx, db, func(conn *sql.Conn) (struct{}, error) {
		item, err := claimedStatePoolItem(ctx, conn, params.Claim, true)
		if err != nil {
			return struct{}{}, err
		}

		item.State = sqlitePoolStateAvailable
		item.ClaimID = ""
		item.ExpiresAt = ""
		item.ExpiryPolicy = ""
		item.Version++
		item.Tombstone = nil
		if err := writeStatePoolItemConn(ctx, conn, item); err != nil {
			return struct{}{}, err
		}

		return struct{}{}, nil
	})
	if err != nil {
		return protocol.StateReleaseResult{}, err
	}

	return protocol.StateReleaseResult{}, nil
}

func (s *serverState) consumeStateClaim(
	ctx context.Context,
	params protocol.StateConsumeParams,
) (protocol.StateConsumeResult, error) {
	db, err := s.stateDB(ctx, params.Config)
	if err != nil {
		return protocol.StateConsumeResult{}, err
	}
	defer db.Close()

	_, err = withImmediateTx(ctx, db, func(conn *sql.Conn) (struct{}, error) {
		item, err := claimedStatePoolItem(ctx, conn, params.Claim, true)
		if err != nil {
			return struct{}{}, err
		}

		item.State = sqlitePoolStateUsed
		item.ClaimID = ""
		item.ExpiresAt = ""
		item.ExpiryPolicy = ""
		item.Tombstone = cloneObject(params.Tombstone)
		item.Version++
		if err := writeStatePoolItemConn(ctx, conn, item); err != nil {
			return struct{}{}, err
		}

		return struct{}{}, nil
	})
	if err != nil {
		return protocol.StateConsumeResult{}, err
	}

	return protocol.StateConsumeResult{}, nil
}

func (s *serverState) stateDB(ctx context.Context, config map[string]any) (*sql.DB, error) {
	profileName, err := requiredStringProperty(config, "profile")
	if err != nil {
		return nil, err
	}

	profileConfig, err := s.profile(profileName)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", profileConfig.DSN)
	if err != nil {
		return nil, err
	}
	if err := ensureStateSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func ensureStateSchema(ctx context.Context, db *sql.DB) error {
	for _, statement := range []string{
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  record_key TEXT PRIMARY KEY,
  version INTEGER NOT NULL,
  value_json TEXT NOT NULL
)`, sqliteStateRecordsTable),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  pool_name TEXT NOT NULL,
  item_id TEXT NOT NULL,
  fields_json TEXT NOT NULL,
  state TEXT NOT NULL,
  claim_id TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  expiry_policy TEXT NOT NULL DEFAULT '',
  tombstone_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL,
  PRIMARY KEY (pool_name, item_id)
)`, sqliteStatePoolItemsTable),
		fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s_pool_state_idx ON %s(pool_name, state, item_id)",
			sqliteStatePoolItemsTable,
			sqliteStatePoolItemsTable,
		),
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}

func claimStatePoolItemTx(
	ctx context.Context,
	db *sql.DB,
	params protocol.StateClaimParams,
) (protocol.StateClaimResult, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return protocol.StateClaimResult{}, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return protocol.StateClaimResult{}, err
	}

	committed := false
	defer rollbackTx(ctx, conn, &committed)

	result, err := claimStatePoolItemConn(ctx, conn, params)
	if err != nil {
		return protocol.StateClaimResult{}, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return protocol.StateClaimResult{}, err
	}
	committed = true
	return result, nil
}

func claimStatePoolItemConn(
	ctx context.Context,
	conn *sql.Conn,
	params protocol.StateClaimParams,
) (protocol.StateClaimResult, error) {
	items, err := loadStatePoolItemsConn(ctx, conn, params.Pool)
	if err != nil {
		return protocol.StateClaimResult{}, err
	}

	now := time.Now().UTC()
	updated := false
	chosenIndex := -1
	for i := range items {
		normalized, changed := normalizeExpiredStatePoolItem(items[i], now)
		if changed {
			if err := writeStatePoolItemConn(ctx, conn, normalized); err != nil {
				return protocol.StateClaimResult{}, err
			}
			updated = true
		}
		items[i] = normalized

		if chosenIndex != -1 {
			continue
		}
		if normalized.State != sqlitePoolStateAvailable {
			continue
		}
		if !selectorMatchesStatePoolItem(normalized, params.Selector) {
			continue
		}
		chosenIndex = i
	}

	if chosenIndex == -1 {
		if updated {
			if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
				return protocol.StateClaimResult{}, err
			}
		}
		return protocol.StateClaimResult{}, fmt.Errorf("pool %q has no available fixture", params.Pool)
	}

	if params.Lease.TTL <= 0 {
		return protocol.StateClaimResult{}, errors.New("lease.ttl must be positive")
	}

	claimID, err := randomClaimID()
	if err != nil {
		return protocol.StateClaimResult{}, err
	}

	expiryPolicy := params.Lease.ExpiryPolicy
	if expiryPolicy == "" {
		expiryPolicy = statemodel.ExpiryStale
	}

	item := items[chosenIndex]
	expiresAt := now.Add(params.Lease.TTL)
	item.State = sqlitePoolStateReserved
	item.ClaimID = claimID
	item.ExpiresAt = expiresAt.Format(time.RFC3339Nano)
	item.ExpiryPolicy = string(expiryPolicy)
	item.Version++
	if err := writeStatePoolItemConn(ctx, conn, item); err != nil {
		return protocol.StateClaimResult{}, err
	}

	return protocol.StateClaimResult{
		Result: statemodel.ClaimResult{
			Item: cloneObject(item.Fields),
			Claim: statemodel.ClaimHandle{
				Pool:         params.Pool,
				ItemID:       item.ID,
				ClaimID:      item.ClaimID,
				ExpiresAt:    expiresAt,
				Version:      strconv.FormatInt(item.Version, 10),
				ExpiryPolicy: expiryPolicy,
				Guarantee:    statemodel.GuaranteeLocalAtomic,
			},
		},
	}, nil
}

func loadStateRecord(ctx context.Context, db *sql.DB, key string) (sqliteStateRecord, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return sqliteStateRecord{}, err
	}
	defer conn.Close()

	return loadStateRecordConn(ctx, conn, key)
}

func loadStateRecordConn(ctx context.Context, conn *sql.Conn, key string) (sqliteStateRecord, error) {
	row := conn.QueryRowContext(
		ctx,
		fmt.Sprintf("SELECT version, value_json FROM %s WHERE record_key = ?", sqliteStateRecordsTable),
		key,
	)

	var version int64
	var rawValue string
	if err := row.Scan(&version, &rawValue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sqliteStateRecord{}, fmt.Errorf("record %q is not present", key)
		}
		return sqliteStateRecord{}, err
	}

	value, err := decodeObject(rawValue)
	if err != nil {
		return sqliteStateRecord{}, err
	}

	return sqliteStateRecord{Key: key, Value: value, Version: version}, nil
}

func writeStateRecordConn(ctx context.Context, conn *sql.Conn, record sqliteStateRecord) error {
	rawValue, err := encodeObject(record.Value)
	if err != nil {
		return err
	}

	_, err = conn.ExecContext(
		ctx,
		fmt.Sprintf("UPDATE %s SET version = ?, value_json = ? WHERE record_key = ?", sqliteStateRecordsTable),
		record.Version,
		rawValue,
		record.Key,
	)
	return err
}

func loadStatePoolItemsConn(ctx context.Context, conn *sql.Conn, pool string) ([]sqliteStatePoolItem, error) {
	rows, err := conn.QueryContext(
		ctx,
		fmt.Sprintf(`
SELECT item_id, fields_json, state, claim_id, expires_at, expiry_policy, tombstone_json, version
FROM %s
WHERE pool_name = ?
ORDER BY item_id
`, sqliteStatePoolItemsTable),
		pool,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]sqliteStatePoolItem, 0)
	for rows.Next() {
		var (
			item         sqliteStatePoolItem
			rawFields    string
			rawTombstone string
		)
		item.Pool = pool
		if err := rows.Scan(
			&item.ID,
			&rawFields,
			&item.State,
			&item.ClaimID,
			&item.ExpiresAt,
			&item.ExpiryPolicy,
			&rawTombstone,
			&item.Version,
		); err != nil {
			return nil, err
		}

		fields, err := decodeObject(rawFields)
		if err != nil {
			return nil, err
		}
		item.Fields = fields

		tombstone, err := decodeObject(rawTombstone)
		if err != nil {
			return nil, err
		}
		item.Tombstone = tombstone
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("pool %q is not present", pool)
	}

	return items, nil
}

func writeStatePoolItemConn(ctx context.Context, conn *sql.Conn, item sqliteStatePoolItem) error {
	rawFields, err := encodeObject(item.Fields)
	if err != nil {
		return err
	}
	rawTombstone, err := encodeObject(item.Tombstone)
	if err != nil {
		return err
	}

	_, err = conn.ExecContext(
		ctx,
		fmt.Sprintf(`
UPDATE %s
SET fields_json = ?, state = ?, claim_id = ?, expires_at = ?, expiry_policy = ?, tombstone_json = ?, version = ?
WHERE pool_name = ? AND item_id = ?
`, sqliteStatePoolItemsTable),
		rawFields,
		item.State,
		item.ClaimID,
		item.ExpiresAt,
		item.ExpiryPolicy,
		rawTombstone,
		item.Version,
		item.Pool,
		item.ID,
	)
	return err
}

func claimedStatePoolItem(
	ctx context.Context,
	conn *sql.Conn,
	claim statemodel.ClaimHandle,
	failOnExpired bool,
) (sqliteStatePoolItem, error) {
	items, err := loadStatePoolItemsConn(ctx, conn, claim.Pool)
	if err != nil {
		return sqliteStatePoolItem{}, err
	}

	now := time.Now().UTC()
	for i := range items {
		normalized, changed := normalizeExpiredStatePoolItem(items[i], now)
		if changed {
			if err := writeStatePoolItemConn(ctx, conn, normalized); err != nil {
				return sqliteStatePoolItem{}, err
			}
		}
		items[i] = normalized

		if items[i].ID != claim.ItemID {
			continue
		}

		item := items[i]
		if item.State != sqlitePoolStateReserved {
			return sqliteStatePoolItem{}, fmt.Errorf("claim for item %q is no longer active", claim.ItemID)
		}
		if item.ClaimID != claim.ClaimID {
			return sqliteStatePoolItem{}, fmt.Errorf("claim for item %q is no longer owned by this run", claim.ItemID)
		}
		if strconv.FormatInt(item.Version, 10) != claim.Version {
			return sqliteStatePoolItem{}, fmt.Errorf("claim for item %q is stale", claim.ItemID)
		}

		expiresAt, err := parseExpiry(item.ExpiresAt)
		if err != nil {
			return sqliteStatePoolItem{}, err
		}
		if failOnExpired && !expiresAt.After(now) {
			return sqliteStatePoolItem{}, fmt.Errorf("claim for item %q is expired", claim.ItemID)
		}

		return item, nil
	}

	return sqliteStatePoolItem{}, fmt.Errorf("claimed item %q is missing", claim.ItemID)
}

func normalizeExpiredStatePoolItem(item sqliteStatePoolItem, now time.Time) (sqliteStatePoolItem, bool) {
	if item.State != sqlitePoolStateReserved {
		return item, false
	}
	if item.ExpiryPolicy != string(statemodel.ExpiryReclaim) {
		return item, false
	}
	expiresAt, err := parseExpiry(item.ExpiresAt)
	if err != nil || expiresAt.After(now) {
		return item, false
	}

	item.State = sqlitePoolStateAvailable
	item.ClaimID = ""
	item.ExpiresAt = ""
	item.ExpiryPolicy = ""
	item.Version++
	return item, true
}

func selectorMatchesStatePoolItem(item sqliteStatePoolItem, selector statemodel.Selector) bool {
	if selector.ID != "" && item.ID != selector.ID {
		return false
	}
	for key, want := range selector.Fields {
		if fmt.Sprint(item.Fields[key]) != want {
			return false
		}
	}
	return true
}

func encodeObject(value map[string]any) (string, error) {
	if len(value) == 0 {
		return "{}", nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeObject(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, err
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}

func parseExpiry(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, errors.New("claim expiry is missing")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return expiresAt.UTC(), nil
}

func randomClaimID() (string, error) {
	value := time.Now().UTC().UnixNano()
	if value == 0 {
		return "", errors.New("claim id source is unavailable")
	}
	return fmt.Sprintf("claim-%d", value), nil
}

func withImmediateTx[T any](ctx context.Context, db *sql.DB, fn func(*sql.Conn) (T, error)) (T, error) {
	var zero T

	conn, err := db.Conn(ctx)
	if err != nil {
		return zero, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return zero, err
	}

	committed := false
	defer rollbackTx(ctx, conn, &committed)

	value, err := fn(conn)
	if err != nil {
		return zero, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return zero, err
	}
	committed = true
	return value, nil
}

func rollbackTx(ctx context.Context, conn *sql.Conn, committed *bool) {
	if committed != nil && *committed {
		return
	}

	rollbackCtx := context.WithoutCancel(ctx)
	_, _ = conn.ExecContext(rollbackCtx, "ROLLBACK")
}

func cloneObject(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}

	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	if cloned == nil {
		return map[string]any{}
	}
	return cloned
}
