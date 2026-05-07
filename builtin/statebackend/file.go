package statebackend

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alex-poliushkin/theater"
	statemodel "github.com/alex-poliushkin/theater/state"
)

const (
	fileRecordsDir = "records"
	filePoolsDir   = "pools"

	filePoolStateAvailable = "available"
	filePoolStateReserved  = "reserved"
	filePoolStateUsed      = "used"
	filePoolStateStale     = "stale"
)

type fileBackend struct {
	root string
}

type fileRecordDocument struct {
	Key     string         `json:"key"`
	Value   map[string]any `json:"value"`
	Version int64          `json:"version"`
}

type filePoolDocument struct {
	Pool  string         `json:"pool"`
	Items []filePoolItem `json:"items"`
}

type filePoolItem struct {
	ID           string         `json:"id"`
	Fields       map[string]any `json:"fields"`
	State        string         `json:"state"`
	ClaimID      string         `json:"claim_id,omitempty"`
	ExpiresAt    string         `json:"expires_at,omitempty"`
	ExpiryPolicy string         `json:"expiry_policy,omitempty"`
	Tombstone    map[string]any `json:"tombstone,omitempty"`
	Version      int64          `json:"version"`
}

func fileBackendDefinition() theater.StateBackendDef {
	return theater.StateBackendDef{
		Params: []theater.ParamSpec{
			{
				Name:     "root",
				Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
				Required: true,
			},
		},
		Describe: func(config theater.Values) (theater.StateDescriptor, error) {
			if _, ok := config["root"].(string); !ok {
				return theater.StateDescriptor{}, errors.New("root must be string")
			}

			return theater.StateDescriptor{
				Guarantee:       theater.StateGuaranteeLocalAtomic,
				SupportsCAS:     true,
				SupportsClaim:   true,
				SupportsRenew:   true,
				SupportsRelease: true,
				SupportsConsume: true,
			}, nil
		},
		Open: func(config theater.Values) (theater.StateBackend, error) {
			root, _ := config["root"].(string)
			if root == "" {
				return nil, errors.New("root is required")
			}

			backend := &fileBackend{root: root}
			if err := backend.ensureLayout(); err != nil {
				return nil, err
			}

			return backend, nil
		},
	}
}

func (b *fileBackend) Describe(context.Context) (statemodel.Descriptor, error) {
	return statemodel.Descriptor{
		Guarantee:       statemodel.GuaranteeLocalAtomic,
		SupportsCAS:     true,
		SupportsClaim:   true,
		SupportsRenew:   true,
		SupportsRelease: true,
		SupportsConsume: true,
	}, nil
}

func (b *fileBackend) ReadRecord(_ context.Context, key string) (statemodel.RecordSnapshot, error) {
	doc, err := b.withRecordLock(key, func() (fileRecordDocument, error) {
		return b.readRecordDocument(key)
	})
	if err != nil {
		return statemodel.RecordSnapshot{}, err
	}

	return statemodel.RecordSnapshot{
		Key:       key,
		Value:     cloneObject(doc.Value),
		Version:   strconv.FormatInt(doc.Version, 10),
		Backend:   "",
		Guarantee: statemodel.GuaranteeLocalAtomic,
	}, nil
}

func (b *fileBackend) CompareAndSetRecord(
	_ context.Context,
	key string,
	expectedVersion string,
	value map[string]any,
) (statemodel.RecordSnapshot, error) {
	doc, err := b.withRecordLock(key, func() (fileRecordDocument, error) {
		doc, err := b.readRecordDocument(key)
		if err != nil {
			return fileRecordDocument{}, err
		}

		if got := strconv.FormatInt(doc.Version, 10); got != expectedVersion {
			return fileRecordDocument{}, fmt.Errorf("record %q version mismatch: got %s want %s", key, got, expectedVersion)
		}

		doc.Value = cloneObject(value)
		doc.Version++
		if err := b.writeRecordDocument(doc); err != nil {
			return fileRecordDocument{}, err
		}

		return doc, nil
	})
	if err != nil {
		return statemodel.RecordSnapshot{}, err
	}

	return statemodel.RecordSnapshot{
		Key:       key,
		Value:     cloneObject(doc.Value),
		Version:   strconv.FormatInt(doc.Version, 10),
		Backend:   "",
		Guarantee: statemodel.GuaranteeLocalAtomic,
	}, nil
}

func (b *fileBackend) Claim(
	_ context.Context,
	pool string,
	selector statemodel.Selector,
	lease statemodel.LeaseSpec,
) (statemodel.ClaimResult, error) {
	doc, claim, item, err := b.withPoolLock(pool, func() (filePoolDocument, statemodel.ClaimHandle, map[string]any, error) {
		doc, err := b.readPoolDocument(pool)
		if err != nil {
			return filePoolDocument{}, statemodel.ClaimHandle{}, nil, err
		}

		now := time.Now().UTC()
		changed := false
		for i := range doc.Items {
			normalized, itemChanged := normalizeExpiredPoolItem(doc.Items[i], now)
			doc.Items[i] = normalized
			changed = changed || itemChanged
		}

		index := -1
		for i := range doc.Items {
			if doc.Items[i].State != filePoolStateAvailable {
				continue
			}
			if !selectorMatches(doc.Items[i], selector) {
				continue
			}

			index = i
			break
		}
		if index == -1 {
			if changed {
				if err := b.writePoolDocument(doc); err != nil {
					return filePoolDocument{}, statemodel.ClaimHandle{}, nil, err
				}
			}

			return filePoolDocument{}, statemodel.ClaimHandle{}, nil, fmt.Errorf("pool %q has no available fixture", pool)
		}

		item := &doc.Items[index]
		item.State = filePoolStateReserved
		item.ClaimID = randomClaimID()
		item.ExpiresAt = now.Add(lease.TTL).Format(time.RFC3339Nano)
		item.ExpiryPolicy = string(lease.ExpiryPolicy)
		item.Version++
		if err := b.writePoolDocument(doc); err != nil {
			return filePoolDocument{}, statemodel.ClaimHandle{}, nil, err
		}

		return doc, statemodel.ClaimHandle{
			Pool:         pool,
			ItemID:       item.ID,
			ClaimID:      item.ClaimID,
			ExpiresAt:    now.Add(lease.TTL),
			Version:      strconv.FormatInt(item.Version, 10),
			ExpiryPolicy: lease.ExpiryPolicy,
			Guarantee:    statemodel.GuaranteeLocalAtomic,
		}, itemValue(*item), nil
	})
	if err != nil {
		return statemodel.ClaimResult{}, err
	}

	claim.Backend = ""
	_ = doc
	return statemodel.ClaimResult{
		Item:  item,
		Claim: claim,
	}, nil
}

func (b *fileBackend) Renew(_ context.Context, claim statemodel.ClaimHandle, ttl time.Duration) (statemodel.ClaimHandle, error) {
	renewed, err := b.withPoolClaimLock(claim.Pool, func() (statemodel.ClaimHandle, error) {
		doc, err := b.readPoolDocument(claim.Pool)
		if err != nil {
			return statemodel.ClaimHandle{}, err
		}

		item, err := claimedPoolItem(&doc, claim, true)
		if err != nil {
			return statemodel.ClaimHandle{}, err
		}

		expiresAt := time.Now().UTC().Add(ttl)
		item.ExpiresAt = expiresAt.Format(time.RFC3339Nano)
		item.Version++
		if err := b.writePoolDocument(doc); err != nil {
			return statemodel.ClaimHandle{}, err
		}

		return statemodel.ClaimHandle{
			Pool:         claim.Pool,
			ItemID:       item.ID,
			ClaimID:      item.ClaimID,
			ExpiresAt:    expiresAt,
			Version:      strconv.FormatInt(item.Version, 10),
			ExpiryPolicy: statemodel.ExpiryPolicy(item.ExpiryPolicy),
			Guarantee:    statemodel.GuaranteeLocalAtomic,
		}, nil
	})
	return renewed, err
}

func (b *fileBackend) Release(_ context.Context, claim statemodel.ClaimHandle, _ string) error {
	err := b.withPoolDocumentLock(claim.Pool, func() error {
		doc, err := b.readPoolDocument(claim.Pool)
		if err != nil {
			return err
		}

		item, err := claimedPoolItem(&doc, claim, true)
		if err != nil {
			return err
		}

		item.State = filePoolStateAvailable
		item.ClaimID = ""
		item.ExpiresAt = ""
		item.ExpiryPolicy = ""
		item.Version++
		if err := b.writePoolDocument(doc); err != nil {
			return err
		}

		return nil
	})
	return err
}

func (b *fileBackend) Consume(_ context.Context, claim statemodel.ClaimHandle, _ string, tombstone map[string]any) error {
	err := b.withPoolDocumentLock(claim.Pool, func() error {
		doc, err := b.readPoolDocument(claim.Pool)
		if err != nil {
			return err
		}

		item, err := claimedPoolItem(&doc, claim, true)
		if err != nil {
			return err
		}

		item.State = filePoolStateUsed
		item.ClaimID = ""
		item.ExpiresAt = ""
		item.ExpiryPolicy = ""
		item.Tombstone = cloneObject(tombstone)
		item.Version++
		if err := b.writePoolDocument(doc); err != nil {
			return err
		}

		return nil
	})
	return err
}

func (b *fileBackend) ensureLayout() error {
	for _, dir := range []string{b.recordsDir(), b.poolsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func (b *fileBackend) recordsDir() string { return filepath.Join(b.root, fileRecordsDir) }
func (b *fileBackend) poolsDir() string   { return filepath.Join(b.root, filePoolsDir) }

func (b *fileBackend) recordPath(key string) (string, error) {
	if err := validateStateResourceName(key); err != nil {
		return "", fmt.Errorf("record %q uses unsupported path syntax: %w", key, err)
	}

	return stateResourcePath(b.recordsDir(), key), nil
}

func (b *fileBackend) poolPath(name string) (string, error) {
	if err := validateStateResourceName(name); err != nil {
		return "", fmt.Errorf("pool %q uses unsupported path syntax: %w", name, err)
	}

	return stateResourcePath(b.poolsDir(), name), nil
}

func (b *fileBackend) flatEncodedRecordPath(key string) string {
	return filepath.Join(b.recordsDir(), flatEncodedStateResourceName(key)+".json")
}

func (b *fileBackend) flatEncodedPoolPath(name string) string {
	return filepath.Join(b.poolsDir(), flatEncodedStateResourceName(name)+".json")
}

func (b *fileBackend) withRecordLock(key string, fn func() (fileRecordDocument, error)) (fileRecordDocument, error) {
	path, err := b.recordPath(key)
	if err != nil {
		return fileRecordDocument{}, err
	}

	lock, err := lockFile(path + ".lock")
	if err != nil {
		return fileRecordDocument{}, err
	}
	defer lock.close()

	return fn()
}

func (b *fileBackend) withPoolLock(
	name string,
	fn func() (filePoolDocument, statemodel.ClaimHandle, map[string]any, error),
) (filePoolDocument, statemodel.ClaimHandle, map[string]any, error) {
	path, err := b.poolPath(name)
	if err != nil {
		return filePoolDocument{}, statemodel.ClaimHandle{}, nil, err
	}

	lock, err := lockFile(path + ".lock")
	if err != nil {
		return filePoolDocument{}, statemodel.ClaimHandle{}, nil, err
	}
	defer lock.close()

	return fn()
}

func (b *fileBackend) withPoolClaimLock(
	name string,
	fn func() (statemodel.ClaimHandle, error),
) (statemodel.ClaimHandle, error) {
	path, err := b.poolPath(name)
	if err != nil {
		return statemodel.ClaimHandle{}, err
	}

	lock, err := lockFile(path + ".lock")
	if err != nil {
		return statemodel.ClaimHandle{}, err
	}
	defer lock.close()

	return fn()
}

func (b *fileBackend) withPoolDocumentLock(name string, fn func() error) error {
	path, err := b.poolPath(name)
	if err != nil {
		return err
	}

	lock, err := lockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer lock.close()

	return fn()
}

func (b *fileBackend) readRecordDocument(key string) (fileRecordDocument, error) {
	path, err := b.recordPath(key)
	if err != nil {
		return fileRecordDocument{}, err
	}
	if err := rejectFlatEncodedFile(path, b.flatEncodedRecordPath(key), "record", key); err != nil {
		return fileRecordDocument{}, err
	}

	var doc fileRecordDocument
	if err := ensureJSONFile(path, defaultRecordDocument(key)); err != nil {
		return fileRecordDocument{}, err
	}
	if err := readJSONFile(path, &doc); err != nil {
		return fileRecordDocument{}, err
	}
	if doc.Key != key {
		return fileRecordDocument{}, fmt.Errorf("record %q collides with existing document for %q", key, doc.Key)
	}

	return doc, nil
}

func (b *fileBackend) writeRecordDocument(doc fileRecordDocument) error {
	path, err := b.recordPath(doc.Key)
	if err != nil {
		return err
	}

	return writeJSONFileAtomic(path, doc)
}

func (b *fileBackend) readPoolDocument(name string) (filePoolDocument, error) {
	path, err := b.poolPath(name)
	if err != nil {
		return filePoolDocument{}, err
	}
	if err := rejectFlatEncodedFile(path, b.flatEncodedPoolPath(name), "pool", name); err != nil {
		return filePoolDocument{}, err
	}

	var doc filePoolDocument
	if err := ensureJSONFile(path, defaultPoolDocument(name)); err != nil {
		return filePoolDocument{}, err
	}
	if err := readJSONFile(path, &doc); err != nil {
		return filePoolDocument{}, err
	}
	if doc.Pool != name {
		return filePoolDocument{}, fmt.Errorf("pool %q collides with existing document for %q", name, doc.Pool)
	}

	return doc, nil
}

func (b *fileBackend) writePoolDocument(doc filePoolDocument) error {
	path, err := b.poolPath(doc.Pool)
	if err != nil {
		return err
	}

	return writeJSONFileAtomic(path, doc)
}

type lockedFile struct {
	file *os.File
}

func lockFile(path string) (*lockedFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}

	return &lockedFile{file: file}, nil
}

func (l *lockedFile) close() {
	if l == nil || l.file == nil {
		return
	}

	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("state resource %q is missing", path)
		}

		return err
	}

	if err := json.Unmarshal(data, target); err != nil {
		return err
	}

	return nil
}

func ensureJSONFile(path string, value any) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return writeJSONFileAtomic(path, value)
}

func rejectFlatEncodedFile(path, flatPath, kind, name string) error {
	if flatPath == "" || flatPath == path {
		return nil
	}
	if _, err := os.Stat(flatPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	return fmt.Errorf(
		"%s %q uses unsupported flat encoded state file %q; remove or reseed the backend root with nested paths",
		kind,
		name,
		flatPath,
	)
}

func writeJSONFileAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	return os.Rename(tempName, path)
}

func claimedPoolItem(doc *filePoolDocument, claim statemodel.ClaimHandle, failOnExpired bool) (*filePoolItem, error) {
	now := time.Now().UTC()
	for i := range doc.Items {
		normalized, _ := normalizeExpiredPoolItem(doc.Items[i], now)
		doc.Items[i] = normalized

		if doc.Items[i].ID != claim.ItemID {
			continue
		}

		item := &doc.Items[i]
		if item.State != filePoolStateReserved {
			return nil, fmt.Errorf("claim for item %q is no longer active", claim.ItemID)
		}
		if item.ClaimID != claim.ClaimID {
			return nil, fmt.Errorf("claim for item %q is no longer owned by this run", claim.ItemID)
		}
		if strconv.FormatInt(item.Version, 10) != claim.Version {
			return nil, fmt.Errorf("claim for item %q is stale", claim.ItemID)
		}

		expiresAt, err := parseExpiry(item.ExpiresAt)
		if err != nil {
			return nil, err
		}
		if failOnExpired && !expiresAt.After(now) {
			return nil, fmt.Errorf("claim for item %q is expired", claim.ItemID)
		}

		return item, nil
	}

	return nil, fmt.Errorf("claimed item %q is missing", claim.ItemID)
}

func normalizeExpiredPoolItem(item filePoolItem, now time.Time) (filePoolItem, bool) {
	if item.State != filePoolStateReserved || item.ExpiresAt == "" {
		return item, false
	}

	expiresAt, err := parseExpiry(item.ExpiresAt)
	if err != nil || expiresAt.After(now) {
		return item, false
	}

	switch statemodel.ExpiryPolicy(item.ExpiryPolicy) {
	case statemodel.ExpiryReclaim:
		item.State = filePoolStateAvailable
		item.ClaimID = ""
		item.ExpiresAt = ""
		item.ExpiryPolicy = ""
	default:
		item.State = filePoolStateStale
		item.ClaimID = ""
		item.ExpiresAt = ""
	}
	item.Version++
	return item, true
}

func selectorMatches(item filePoolItem, selector statemodel.Selector) bool {
	if selector.ID != "" && item.ID != selector.ID {
		return false
	}

	for key, want := range selector.Fields {
		got, ok := item.Fields[key]
		if !ok {
			return false
		}
		if fmt.Sprint(got) != want {
			return false
		}
	}

	return true
}

func itemValue(item filePoolItem) map[string]any {
	value := cloneObject(item.Fields)
	if value == nil {
		value = map[string]any{}
	}
	if _, ok := value["id"]; !ok {
		value["id"] = item.ID
	}

	return value
}

func parseExpiry(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("claim expiry is missing")
	}

	return time.Parse(time.RFC3339Nano, value)
}

func cloneObject(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}

	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func defaultRecordDocument(key string) fileRecordDocument {
	return fileRecordDocument{
		Key:     key,
		Value:   map[string]any{},
		Version: 0,
	}
}

func defaultPoolDocument(name string) filePoolDocument {
	return filePoolDocument{
		Pool:  name,
		Items: []filePoolItem{},
	}
}

func stateResourcePath(root, name string) string {
	segments := safePathSegments(name)
	if len(segments) == 1 {
		return filepath.Join(root, segments[0]+".json")
	}

	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, root)
	parts = append(parts, segments[:len(segments)-1]...)
	parts = append(parts, segments[len(segments)-1]+".json")
	return filepath.Join(parts...)
}

func safePathSegments(name string) []string {
	rawSegments := strings.Split(name, "/")
	segments := make([]string, 0, len(rawSegments))
	for i := range rawSegments {
		segments = append(segments, safePathSegment(rawSegments[i]))
	}
	return segments
}

func safePathSegment(segment string) string {
	return strings.NewReplacer("\\", "%5C", " ", "_").Replace(segment)
}

func flatEncodedStateResourceName(name string) string {
	return strings.NewReplacer("/", "%2F", "\\", "%5C", " ", "_").Replace(name)
}

func validateStateResourceName(name string) error {
	for _, segment := range strings.Split(name, "/") {
		switch segment {
		case "", ".", "..":
			return fmt.Errorf("segment %q is reserved, use non-empty segments and avoid dot segments", segment)
		}
	}

	return nil
}

func randomClaimID() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}

	return hex.EncodeToString(bytes[:])
}
