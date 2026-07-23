package metrics

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-command-compression-proxy/internal/projectfiles"

	bolt "go.etcd.io/bbolt"
)

const spoolDirectoryName = "metrics-spool"

type spoolEvent struct {
	ID     string    `json:"id"`
	Record runRecord `json:"record"`
}

type StorageStatus struct {
	Observed      int `json:"observed"`
	Pending       int `json:"pending"`
	Rejected      int `json:"rejected"`
	StorageErrors int `json:"storage_errors"`
}

func spoolProjectMetric(projectRoot, databasePath string, rec runRecord) error {
	if err := ensureLocalCCPGitignore(projectRoot, databasePath); err != nil {
		return err
	}
	id, err := newEventID()
	if err != nil {
		return fmt.Errorf("create metrics event id: %w", err)
	}
	event := spoolEvent{ID: id, Record: rec}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode metrics spool event: %w", err)
	}
	spoolDir := filepath.Join(filepath.Dir(databasePath), spoolDirectoryName)
	eventPath := filepath.Join(spoolDir, id+".json")
	if err := projectfiles.AtomicWriteFileBeneath(projectRoot, eventPath, body, 0o600); err != nil {
		return fmt.Errorf("write metrics spool event: %w", err)
	}
	// AtomicWriteFileBeneath creates missing parents without widening existing
	// modes. Tighten both automatic-state directories after the contained write.
	if err := os.Chmod(filepath.Dir(databasePath), 0o700); err != nil {
		return fmt.Errorf("tighten metrics directory: %w", err)
	}
	if err := os.Chmod(spoolDir, 0o700); err != nil {
		return fmt.Errorf("tighten metrics spool directory: %w", err)
	}
	return nil
}

func consolidateProjectSpool(projectRoot, databasePath string) (retErr error) {
	spoolDir := filepath.Join(filepath.Dir(databasePath), spoolDirectoryName)
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := ensureSchema(projectRoot, databasePath); err != nil {
		return err
	}
	db, err := openDBAt(projectRoot, databasePath, false)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()

	var joined error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(spoolDir, entry.Name())
		event, readErr := readSpoolEvent(projectRoot, path)
		if readErr != nil {
			joined = errors.Join(joined, readErr)
			continue
		}
		if commitErr := commitSpoolEvent(db, event); commitErr != nil {
			return errors.Join(joined, commitErr)
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			joined = errors.Join(joined, removeErr)
		}
	}
	return joined
}

func readSpoolEvent(projectRoot, path string) (_ spoolEvent, retErr error) {
	file, err := projectfiles.OpenFileBeneath(projectRoot, path, os.O_RDONLY, 0)
	if err != nil {
		return spoolEvent{}, err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	body, err := io.ReadAll(io.LimitReader(file, 2<<20))
	if err != nil {
		return spoolEvent{}, err
	}
	var event spoolEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return spoolEvent{}, fmt.Errorf("decode metrics spool event %q: %w", path, err)
	}
	if strings.TrimSpace(event.ID) == "" || filepath.Base(path) != event.ID+".json" {
		return spoolEvent{}, fmt.Errorf("reject malformed metrics spool event %q", path)
	}
	return event, nil
}

func commitSpoolEvent(db *bolt.DB, event spoolEvent) error {
	return db.Update(func(tx *bolt.Tx) error {
		events := tx.Bucket(eventsBucket)
		runs := tx.Bucket(runsBucket)
		if events == nil || runs == nil {
			return errors.New("metrics schema missing")
		}
		id := []byte(event.ID)
		if events.Get(id) != nil {
			return nil
		}
		seq, err := runs.NextSequence()
		if err != nil {
			return err
		}
		if err := runs.Put(encodeRunKey(event.Record.TimestampUnix, seq), encodeRunRecord(event.Record)); err != nil {
			return err
		}
		if err := events.Put(id, encodeRunKey(event.Record.TimestampUnix, 0)[:8]); err != nil {
			return err
		}
		cutoff := time.Now().UTC().Add(-defaultRetention)
		return errors.Join(
			pruneOldRuns(runs, cutoff, pruneBatchLimit),
			pruneOldEventIDs(events, cutoff, pruneBatchLimit),
		)
	})
}

func pruneOldRuns(bucket *bolt.Bucket, cutoff time.Time, limit int) error {
	cursor := bucket.Cursor()
	removed := 0
	for key, _ := cursor.First(); key != nil && removed < limit; key, _ = cursor.Next() {
		if len(key) < 8 || getBoundedInt64FromU64(key[:8]) >= cutoff.Unix() {
			break
		}
		if err := cursor.Delete(); err != nil {
			return err
		}
		removed++
	}
	return nil
}

func pruneOldEventIDs(bucket *bolt.Bucket, cutoff time.Time, limit int) error {
	cursor := bucket.Cursor()
	removed := 0
	for key, value := cursor.First(); key != nil && removed < limit; key, value = cursor.Next() {
		// Version-one markers stored a one-byte sentinel. Keep those entries:
		// they remain necessary for exactly-once replay and do not have enough
		// information for a safe retention decision.
		if len(value) < 8 || getBoundedInt64FromU64(value[:8]) >= cutoff.Unix() {
			continue
		}
		if err := cursor.Delete(); err != nil {
			return err
		}
		removed++
	}
	return nil
}

func ProjectStorageStatus(projectRoot, databasePath string) StorageStatus {
	status := StorageStatus{}
	spoolDir := filepath.Join(filepath.Dir(databasePath), spoolDirectoryName)
	if entries, err := os.ReadDir(spoolDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				status.Pending++
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		status.StorageErrors++
	}
	db, err := openDBAt(projectRoot, databasePath, true)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			status.StorageErrors++
		}
		return status
	}
	defer func() { _ = db.Close() }()
	_ = db.View(func(tx *bolt.Tx) error {
		if bucket := tx.Bucket(runsBucket); bucket != nil {
			status.Observed = bucket.Stats().KeyN
		}
		return nil
	})
	return status
}

// InspectStorage reports durable and pending metric records without mutating
// the store. It is intended for user-facing reports, including stores that
// are not the current project's contained automatic database.
func InspectStorage(databasePath string) StorageStatus {
	status := StorageStatus{}
	spoolDir := filepath.Join(filepath.Dir(databasePath), spoolDirectoryName)
	if entries, err := os.ReadDir(spoolDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				status.Pending++
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		status.StorageErrors++
	}
	if _, err := os.Stat(databasePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			status.StorageErrors++
		}
		return status
	}
	db, err := bolt.Open(databasePath, 0o600, &bolt.Options{
		ReadOnly: true,
		Timeout:  writeTimeout,
	})
	if err != nil {
		status.StorageErrors++
		return status
	}
	defer func() { _ = db.Close() }()
	if err := db.View(func(tx *bolt.Tx) error {
		if bucket := tx.Bucket(runsBucket); bucket != nil {
			status.Observed = bucket.Stats().KeyN
		}
		return nil
	}); err != nil {
		status.StorageErrors++
	}
	return status
}

func newEventID() (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return fmt.Sprintf("%020d-%d-%s", time.Now().UTC().UnixNano(), os.Getpid(), hex.EncodeToString(random)), nil
}
