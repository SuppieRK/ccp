package workspaces

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const writeTimeout = 100 * time.Millisecond

var (
	workspacesBucket = []byte("workspaces")
	userHomeDir      = os.UserHomeDir
	nowUTC           = func() time.Time { return time.Now().UTC() }
)

type Workspace struct {
	CWD         string    `json:"cwd"`
	MetricsPath string    `json:"metrics_path"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

func DefaultPath() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return PathForHome(home), nil
}

func PathForHome(home string) string {
	return filepath.Join(home, ".config", "ccp", "workspaces.db")
}

func UpsertPath(path, cwd, metricsPath string) (err error) {
	normalized, ok, err := normalizeUpsertPathInput(path, cwd, metricsPath)
	if err != nil || !ok {
		return err
	}
	if err := ensureSchema(path); err != nil {
		return err
	}
	db, err := openDB(path, false)
	if err != nil {
		return err
	}
	defer closeBoltDBWithErr(db, &err)

	timestamp := nowUTC()
	return db.Update(func(tx *bolt.Tx) error {
		return upsertWorkspaceRecord(tx, normalized, timestamp)
	})
}

type normalizedWorkspaceInput struct {
	CWD         string
	MetricsPath string
}

func normalizeUpsertPathInput(path, cwd, metricsPath string) (normalizedWorkspaceInput, bool, error) {
	if strings.TrimSpace(path) == "" {
		return normalizedWorkspaceInput{}, false, nil
	}
	normalizedCWD, err := normalizePath(cwd)
	if err != nil {
		return normalizedWorkspaceInput{}, false, err
	}
	normalizedMetricsPath, err := normalizeOptionalPath(metricsPath)
	if err != nil {
		return normalizedWorkspaceInput{}, false, err
	}
	return normalizedWorkspaceInput{
		CWD:         normalizedCWD,
		MetricsPath: normalizedMetricsPath,
	}, true, nil
}

func upsertWorkspaceRecord(tx *bolt.Tx, normalized normalizedWorkspaceInput, timestamp time.Time) error {
	b := tx.Bucket(workspacesBucket)
	if b == nil {
		return errors.New("workspaces bucket missing")
	}
	entry, err := mergedWorkspaceEntry(b.Get([]byte(normalized.CWD)), normalized, timestamp)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return b.Put([]byte(normalized.CWD), payload)
}

func mergedWorkspaceEntry(raw []byte, normalized normalizedWorkspaceInput, timestamp time.Time) (Workspace, error) {
	entry := Workspace{
		CWD:         normalized.CWD,
		MetricsPath: normalized.MetricsPath,
		FirstSeenAt: timestamp,
		LastSeenAt:  timestamp,
	}
	if len(raw) == 0 {
		return entry, nil
	}
	var existing Workspace
	if err := json.Unmarshal(raw, &existing); err != nil {
		return Workspace{}, err
	}
	entry.FirstSeenAt = existing.FirstSeenAt
	if entry.MetricsPath == "" {
		entry.MetricsPath = existing.MetricsPath
	}
	return entry, nil
}

func ListPath(path string) (entries []Workspace, err error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	defer closeBoltDBWithErr(db, &err)

	out := make([]Workspace, 0, 16)
	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(workspacesBucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, raw []byte) error {
			var entry Workspace
			if err := json.Unmarshal(raw, &entry); err != nil {
				return err
			}
			out = append(out, entry)
			return nil
		})
	}); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CWD < out[j].CWD
	})
	return out, nil
}

func DeletePath(path, cwd string) (err error) {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	normalizedCWD, err := normalizePath(cwd)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	db, err := openDB(path, false)
	if err != nil {
		return err
	}
	defer closeBoltDBWithErr(db, &err)

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(workspacesBucket)
		if b == nil {
			return nil
		}
		return b.Delete([]byte(normalizedCWD))
	})
}

func normalizePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path must not be empty")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func normalizeOptionalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	return normalizePath(path)
}

func ensureSchema(path string) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := openDB(path, false)
	if err != nil {
		return err
	}
	defer closeBoltDBWithErr(db, &err)
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(workspacesBucket)
		return err
	})
}

func openDB(path string, readOnly bool) (*bolt.DB, error) {
	opts := &bolt.Options{
		ReadOnly:       readOnly,
		Timeout:        writeTimeout,
		NoFreelistSync: true,
	}
	db, err := bolt.Open(path, 0o600, opts)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		db.NoSync = true
	}
	return db, nil
}

func closeBoltDBWithErr(db *bolt.DB, retErr *error) {
	if closeErr := db.Close(); *retErr == nil && closeErr != nil {
		*retErr = closeErr
	}
}

func WithTestConfig(home string, now func() time.Time) func() {
	prevHome := userHomeDir
	prevNow := nowUTC
	userHomeDir = func() (string, error) { return home, nil }
	if now != nil {
		nowUTC = now
	}
	return func() {
		userHomeDir = prevHome
		nowUTC = prevNow
	}
}
