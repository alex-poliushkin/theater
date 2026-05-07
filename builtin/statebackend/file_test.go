package statebackend_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-poliushkin/theater"
	builtinstatebackend "github.com/alex-poliushkin/theater/builtin/statebackend"
)

func TestFileBackendSupportsRecordCASUpdate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "records"), 0o755); err != nil {
		t.Fatalf("mkdir records failed: %v", err)
	}

	recordData, err := json.Marshal(map[string]any{
		"key":     "meta",
		"value":   map[string]any{"counter": 1},
		"version": 1,
	})
	if err != nil {
		t.Fatalf("marshal record failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "records", "meta.json"), recordData, 0o644); err != nil {
		t.Fatalf("write record failed: %v", err)
	}

	backend := openFileBackend(t, root)
	snapshot, err := backend.ReadRecord(context.Background(), "meta")
	if err != nil {
		t.Fatalf("read record failed: %v", err)
	}
	if got, want := snapshot.Version, "1"; got != want {
		t.Fatalf("version mismatch: got %q want %q", got, want)
	}

	updated, err := backend.CompareAndSetRecord(context.Background(), "meta", "1", map[string]any{"counter": 2})
	if err != nil {
		t.Fatalf("cas update failed: %v", err)
	}
	if got, want := updated.Version, "2"; got != want {
		t.Fatalf("updated version mismatch: got %q want %q", got, want)
	}
}

func TestFileBackendReclaimsExpiredFixtureWhenConfigured(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pools"), 0o755); err != nil {
		t.Fatalf("mkdir pools failed: %v", err)
	}

	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	poolData, err := json.Marshal(map[string]any{
		"pool": "otp",
		"items": []map[string]any{
			{
				"id":            "expired",
				"fields":        map[string]any{"email": "expired@example.test"},
				"state":         "reserved",
				"claim_id":      "old-claim",
				"expires_at":    expired,
				"expiry_policy": "reclaim",
				"version":       1,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal pool failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pools", "otp.json"), poolData, 0o644); err != nil {
		t.Fatalf("write pool failed: %v", err)
	}

	backend := openFileBackend(t, root)
	result, err := backend.Claim(context.Background(), "otp", theater.StateSelector{}, theater.StateLeaseSpec{
		TTL:          time.Minute,
		ExpiryPolicy: theater.StateExpiryReclaim,
	})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if got, want := result.Item["email"], "expired@example.test"; got != want {
		t.Fatalf("claimed item mismatch: got %#v want %#v", got, want)
	}
}

func TestFileBackendMaterializesMissingRecordAtNestedPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	backend := openFileBackend(t, root)

	snapshot, err := backend.ReadRecord(context.Background(), "env/shared-meta")
	if err != nil {
		t.Fatalf("read record failed: %v", err)
	}
	if got, want := snapshot.Version, "0"; got != want {
		t.Fatalf("version mismatch: got %q want %q", got, want)
	}

	recordPath := filepath.Join(root, "records", "env", "shared-meta.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read materialized record failed: %v", err)
	}
	if !strings.Contains(string(data), `"key": "env/shared-meta"`) {
		t.Fatalf("materialized record must preserve logical key, got %s", data)
	}
	if !strings.Contains(string(data), `"version": 0`) {
		t.Fatalf("materialized record must start at version 0, got %s", data)
	}
}

func TestFileBackendMaterializesMissingPoolAtNestedPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	backend := openFileBackend(t, root)

	_, err := backend.Claim(context.Background(), "fixtures/otp-identities", theater.StateSelector{}, theater.StateLeaseSpec{
		TTL:          time.Minute,
		ExpiryPolicy: theater.StateExpiryReclaim,
	})
	if err == nil {
		t.Fatal("claim missing pool = nil, want no-available-fixture error")
	}
	if !strings.Contains(err.Error(), `pool "fixtures/otp-identities" has no available fixture`) {
		t.Fatalf("claim error mismatch: %v", err)
	}

	poolPath := filepath.Join(root, "pools", "fixtures", "otp-identities.json")
	data, readErr := os.ReadFile(poolPath)
	if readErr != nil {
		t.Fatalf("read materialized pool failed: %v", readErr)
	}
	if !strings.Contains(string(data), `"pool": "fixtures/otp-identities"`) {
		t.Fatalf("materialized pool must preserve logical name, got %s", data)
	}
	if !strings.Contains(string(data), `"items": []`) {
		t.Fatalf("materialized pool must start empty, got %s", data)
	}
}

func TestFileBackendRejectsFlatEncodedRecordPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "records"), 0o755); err != nil {
		t.Fatalf("mkdir records failed: %v", err)
	}

	recordData, err := json.Marshal(map[string]any{
		"key":     "env/shared-meta",
		"value":   map[string]any{"counter": 1},
		"version": 1,
	})
	if err != nil {
		t.Fatalf("marshal record failed: %v", err)
	}

	flatPath := filepath.Join(root, "records", "env%2Fshared-meta.json")
	if err := os.WriteFile(flatPath, recordData, 0o644); err != nil {
		t.Fatalf("write flat encoded record failed: %v", err)
	}

	backend := openFileBackend(t, root)
	_, err = backend.ReadRecord(context.Background(), "env/shared-meta")
	if err == nil {
		t.Fatal("read record with flat encoded path = nil, want rejection error")
	}
	if !strings.Contains(err.Error(), `record "env/shared-meta" uses unsupported flat encoded state file`) {
		t.Fatalf("flat encoded rejection mismatch: %v", err)
	}

	nestedPath := filepath.Join(root, "records", "env", "shared-meta.json")
	if _, err := os.Stat(nestedPath); !os.IsNotExist(err) {
		t.Fatalf("nested record path must not be materialized when flat encoded file exists, got err=%v", err)
	}
}

func TestFileBackendRejectsFlatEncodedRecordPathWhenNestedFileAlsoExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "records", "env"), 0o755); err != nil {
		t.Fatalf("mkdir nested records failed: %v", err)
	}

	nestedData, err := json.Marshal(map[string]any{
		"key":     "env/shared-meta",
		"value":   map[string]any{"counter": 2},
		"version": 2,
	})
	if err != nil {
		t.Fatalf("marshal nested record failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "records", "env", "shared-meta.json"), nestedData, 0o644); err != nil {
		t.Fatalf("write nested record failed: %v", err)
	}

	flatData, err := json.Marshal(map[string]any{
		"key":     "env/shared-meta",
		"value":   map[string]any{"counter": 1},
		"version": 1,
	})
	if err != nil {
		t.Fatalf("marshal flat record failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "records", "env%2Fshared-meta.json"), flatData, 0o644); err != nil {
		t.Fatalf("write flat record failed: %v", err)
	}

	backend := openFileBackend(t, root)
	_, err = backend.ReadRecord(context.Background(), "env/shared-meta")
	if err == nil {
		t.Fatal("read record with mixed nested and flat paths = nil, want rejection error")
	}
	if !strings.Contains(err.Error(), `record "env/shared-meta" uses unsupported flat encoded state file`) {
		t.Fatalf("mixed-tree rejection mismatch: %v", err)
	}
}

func TestFileBackendRejectsFlatEncodedPoolPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pools"), 0o755); err != nil {
		t.Fatalf("mkdir pools failed: %v", err)
	}

	poolData, err := json.Marshal(map[string]any{
		"pool": "fixtures/otp-identities",
		"items": []map[string]any{
			{
				"id":      "mailbox-release",
				"fields":  map[string]any{"email": "pool@example.test"},
				"state":   "available",
				"version": 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal pool failed: %v", err)
	}

	flatPath := filepath.Join(root, "pools", "fixtures%2Fotp-identities.json")
	if err := os.WriteFile(flatPath, poolData, 0o644); err != nil {
		t.Fatalf("write flat encoded pool failed: %v", err)
	}

	backend := openFileBackend(t, root)
	_, err = backend.Claim(context.Background(), "fixtures/otp-identities", theater.StateSelector{
		ID: "mailbox-release",
	}, theater.StateLeaseSpec{
		TTL:          time.Minute,
		ExpiryPolicy: theater.StateExpiryReclaim,
	})
	if err == nil {
		t.Fatal("claim pool with flat encoded path = nil, want rejection error")
	}
	if !strings.Contains(err.Error(), `pool "fixtures/otp-identities" uses unsupported flat encoded state file`) {
		t.Fatalf("flat encoded rejection mismatch: %v", err)
	}

	nestedPath := filepath.Join(root, "pools", "fixtures", "otp-identities.json")
	if _, err := os.Stat(nestedPath); !os.IsNotExist(err) {
		t.Fatalf("nested pool path must not be materialized when flat encoded file exists, got err=%v", err)
	}
}

func TestFileBackendRejectsFlatEncodedPoolPathWhenNestedFileAlsoExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pools", "fixtures"), 0o755); err != nil {
		t.Fatalf("mkdir nested pools failed: %v", err)
	}

	nestedData, err := json.Marshal(map[string]any{
		"pool": "fixtures/otp-identities",
		"items": []map[string]any{
			{
				"id":      "mailbox-release",
				"fields":  map[string]any{"email": "pool@example.test"},
				"state":   "available",
				"version": 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal nested pool failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pools", "fixtures", "otp-identities.json"), nestedData, 0o644); err != nil {
		t.Fatalf("write nested pool failed: %v", err)
	}

	flatData, err := json.Marshal(map[string]any{
		"pool": "fixtures/otp-identities",
		"items": []map[string]any{
			{
				"id":      "mailbox-release",
				"fields":  map[string]any{"email": "pool@example.test"},
				"state":   "available",
				"version": 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal flat pool failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pools", "fixtures%2Fotp-identities.json"), flatData, 0o644); err != nil {
		t.Fatalf("write flat pool failed: %v", err)
	}

	backend := openFileBackend(t, root)
	_, err = backend.Claim(context.Background(), "fixtures/otp-identities", theater.StateSelector{
		ID: "mailbox-release",
	}, theater.StateLeaseSpec{
		TTL:          time.Minute,
		ExpiryPolicy: theater.StateExpiryReclaim,
	})
	if err == nil {
		t.Fatal("claim pool with mixed nested and flat paths = nil, want rejection error")
	}
	if !strings.Contains(err.Error(), `pool "fixtures/otp-identities" uses unsupported flat encoded state file`) {
		t.Fatalf("mixed-tree rejection mismatch: %v", err)
	}
}

func TestFileBackendRejectsRecordPathCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "records"), 0o755); err != nil {
		t.Fatalf("mkdir records failed: %v", err)
	}

	recordData, err := json.Marshal(map[string]any{
		"key":     "shared_meta",
		"value":   map[string]any{"counter": 1},
		"version": 1,
	})
	if err != nil {
		t.Fatalf("marshal record failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "records", "shared_meta.json"), recordData, 0o644); err != nil {
		t.Fatalf("write colliding record failed: %v", err)
	}

	backend := openFileBackend(t, root)
	_, err = backend.ReadRecord(context.Background(), "shared meta")
	if err == nil {
		t.Fatal("read colliding record = nil, want collision error")
	}
	if !strings.Contains(err.Error(), `record "shared meta" collides with existing document for "shared_meta"`) {
		t.Fatalf("collision error mismatch: %v", err)
	}
}

func TestFileBackendRejectsPoolPathCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pools", "fixtures"), 0o755); err != nil {
		t.Fatalf("mkdir pools failed: %v", err)
	}

	poolData, err := json.Marshal(map[string]any{
		"pool": "fixtures/shared_pool",
		"items": []map[string]any{
			{
				"id":      "mailbox-release",
				"fields":  map[string]any{"email": "pool@example.test"},
				"state":   "available",
				"version": 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal pool failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pools", "fixtures", "shared_pool.json"), poolData, 0o644); err != nil {
		t.Fatalf("write colliding pool failed: %v", err)
	}

	backend := openFileBackend(t, root)
	_, err = backend.Claim(context.Background(), "fixtures/shared pool", theater.StateSelector{
		ID: "mailbox-release",
	}, theater.StateLeaseSpec{
		TTL:          time.Minute,
		ExpiryPolicy: theater.StateExpiryReclaim,
	})
	if err == nil {
		t.Fatal("claim colliding pool = nil, want collision error")
	}
	if !strings.Contains(err.Error(), `pool "fixtures/shared pool" collides with existing document for "fixtures/shared_pool"`) {
		t.Fatalf("collision error mismatch: %v", err)
	}
}

func TestFileBackendRejectsReservedRecordPathSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		record  string
		segment string
	}{
		{name: "empty", record: "env//shared-meta", segment: `""`},
		{name: "dot", record: "env/./shared-meta", segment: `"."`},
		{name: "dotdot", record: "env/../shared-meta", segment: `".."`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			backend := openFileBackend(t, root)

			_, err := backend.ReadRecord(context.Background(), tt.record)
			if err == nil {
				t.Fatal("read record with reserved segment = nil, want validation error")
			}
			if !strings.Contains(err.Error(), `record "`+tt.record+`" uses unsupported path syntax`) {
				t.Fatalf("validation error mismatch: %v", err)
			}
			if !strings.Contains(err.Error(), `segment `+tt.segment+` is reserved`) {
				t.Fatalf("validation error must identify reserved segment, got %v", err)
			}

			assertEmptyDir(t, filepath.Join(root, "records"))
			assertEmptyDir(t, filepath.Join(root, "pools"))
		})
	}
}

func TestFileBackendRejectsReservedPoolPathSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pool    string
		segment string
	}{
		{name: "empty", pool: "fixtures//otp-identities", segment: `""`},
		{name: "dot", pool: "fixtures/./otp-identities", segment: `"."`},
		{name: "dotdot", pool: "fixtures/../otp-identities", segment: `".."`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			backend := openFileBackend(t, root)

			_, err := backend.Claim(context.Background(), tt.pool, theater.StateSelector{}, theater.StateLeaseSpec{
				TTL:          time.Minute,
				ExpiryPolicy: theater.StateExpiryReclaim,
			})
			if err == nil {
				t.Fatal("claim pool with reserved segment = nil, want validation error")
			}
			if !strings.Contains(err.Error(), `pool "`+tt.pool+`" uses unsupported path syntax`) {
				t.Fatalf("validation error mismatch: %v", err)
			}
			if !strings.Contains(err.Error(), `segment `+tt.segment+` is reserved`) {
				t.Fatalf("validation error must identify reserved segment, got %v", err)
			}

			assertEmptyDir(t, filepath.Join(root, "records"))
			assertEmptyDir(t, filepath.Join(root, "pools"))
		})
	}
}

func assertEmptyDir(t *testing.T, path string) {
	t.Helper()

	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read dir %s failed: %v", path, err)
	}
	if len(entries) != 0 {
		t.Fatalf("dir %s must stay empty, got %d entries", path, len(entries))
	}
}

func openFileBackend(t *testing.T, root string) theater.StateBackend {
	t.Helper()

	catalog := theater.NewCatalog()
	if err := builtinstatebackend.Register(catalog); err != nil {
		t.Fatalf("register state backend failed: %v", err)
	}

	def, err := catalog.ResolveStateBackend(builtinstatebackend.FileBackendRef)
	if err != nil {
		t.Fatalf("resolve state backend failed: %v", err)
	}

	backend, err := def.Open(theater.Values{"root": root})
	if err != nil {
		t.Fatalf("open state backend failed: %v", err)
	}

	return backend
}
