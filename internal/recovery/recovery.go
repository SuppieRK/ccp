package recovery

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/projectfiles"
	"go-command-compression-proxy/internal/replay"
)

const (
	maxArtifacts    = 20
	maxArtifactSize = 1 << 20
	maxArtifactAge  = 7 * 24 * time.Hour
)

type Event struct {
	Sequence int
	Stream   contracts.Stream
	Data     []byte
}

type Artifact struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Created  time.Time `json:"created"`
	ExitCode int       `json:"exit_code"`
	Bytes    int64     `json:"bytes"`
}

type config struct {
	Enabled bool `json:"enabled"`
}

var (
	userConfigDir = os.UserConfigDir
	nowUTC        = func() time.Time { return time.Now().UTC() }
	recoveryMu    sync.Mutex
)

func ConfigPath() (string, error) {
	root, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ccp", "recovery.json"), nil
}

func RootPath() (string, error) {
	root, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ccp", "recovery"), nil
}

func Enabled() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var cfg config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return false, err
	}
	return cfg.Enabled, nil
}

func SetEnabled(enabled bool) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	body, err := json.MarshalIndent(config{Enabled: enabled}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return projectfiles.AtomicWriteFile(path, body, 0o600)
}

func Store(args []string, events []Event, exitCode int) (*Artifact, error) {
	recoveryMu.Lock()
	defer recoveryMu.Unlock()
	if exitCode == 0 || len(events) == 0 {
		return nil, nil
	}
	replayEvents, total, oversized, err := prepareRecoveryEvents(events)
	if err != nil {
		return nil, err
	}
	if oversized {
		return nil, nil
	}
	root, err := RootPath()
	if err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, err
	}
	id, err := artifactID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, id)
	if err := writeRecoveryArtifact(dir, args, replayEvents, exitCode); err != nil {
		return nil, err
	}
	if err := rotate(root); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	return &Artifact{ID: id, Path: dir, Created: info.ModTime().UTC(), ExitCode: exitCode, Bytes: int64(total)}, nil
}

func prepareRecoveryEvents(events []Event) ([]replay.Event, int, bool, error) {
	total := 0
	replayEvents := make([]replay.Event, 0, len(events))
	for _, event := range events {
		total += len(event.Data)
		if total > maxArtifactSize {
			return nil, total, true, nil
		}
		replayEvents = append(replayEvents, replay.Event{
			Sequence: event.Sequence,
			Stream:   event.Stream,
			Line:     string(event.Data),
		})
	}
	if err := replay.ValidateSequence(replayEvents); err != nil {
		return nil, total, false, err
	}
	return replayEvents, total, false, nil
}

func writeRecoveryArtifact(dir string, args []string, replayEvents []replay.Event, exitCode int) (retErr error) {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := replay.WriteCommandWithExitCodeMode(filepath.Join(dir, replay.CommandFileName), args, exitCode, false, 0o600); err != nil {
		return err
	}
	if err := replay.WriteSequencedEventsMode(filepath.Join(dir, replay.StdoutFileName), replayEvents, contracts.StreamStdout, 0o600); err != nil {
		return err
	}
	if err := replay.WriteSequencedEventsMode(filepath.Join(dir, replay.StderrFileName), replayEvents, contracts.StreamStderr, 0o600); err != nil {
		return err
	}
	return nil
}

func List() ([]Artifact, error) {
	recoveryMu.Lock()
	defer recoveryMu.Unlock()
	root, err := RootPath()
	if err != nil {
		return nil, err
	}
	if err := rotate(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		spec, err := replay.ReadCommand(filepath.Join(dir, replay.CommandFileName))
		if err != nil {
			continue
		}
		size, err := directorySize(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, Artifact{
			ID:       entry.Name(),
			Path:     dir,
			Created:  info.ModTime().UTC(),
			ExitCode: spec.ExitCode,
			Bytes:    size,
		})
	}
	slices.SortFunc(out, func(left, right Artifact) int {
		return right.Created.Compare(left.Created)
	})
	return out, nil
}

func Purge() (int, error) {
	recoveryMu.Lock()
	defer recoveryMu.Unlock()
	root, err := RootPath()
	if err != nil {
		return 0, err
	}
	if err := projectfiles.RejectSymlinkPath(root); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.Contains(entry.Name(), string(os.PathSeparator)) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func rotate(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type candidate struct {
		name    string
		modTime time.Time
	}
	items := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(nowUTC().Add(-maxArtifactAge)) {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
			continue
		}
		items = append(items, candidate{name: entry.Name(), modTime: info.ModTime()})
	}
	slices.SortFunc(items, func(left, right candidate) int {
		if order := right.modTime.Compare(left.modTime); order != 0 {
			return order
		}
		return strings.Compare(right.name, left.name)
	})
	if len(items) <= maxArtifacts {
		return nil
	}
	for _, item := range items[maxArtifacts:] {
		if err := os.RemoveAll(filepath.Join(root, item.name)); err != nil {
			return err
		}
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	if err := projectfiles.RejectSymlinkPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func artifactID() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%s", nowUTC().UnixNano(), hex.EncodeToString(random)), nil
}

func directorySize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}
