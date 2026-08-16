package filtertrust

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/SuppieRK/cmdshape/internal/projectfiles"
)

const (
	storeVersion = 1
	digestDomain = "cmdshape-project-filter-trust-v1"
)

type State string

const (
	StateAbsent    State = "absent"
	StateUntrusted State = "untrusted"
	StateTrusted   State = "trusted"
	StateChanged   State = "changed"
	StateUnsafe    State = "unsafe"
)

type Decision struct {
	Root   string
	Digest string
	State  State
	Reason string
}

type SourceFile struct {
	Name string
	Raw  []byte
}

type approval struct {
	Root      string    `json:"root"`
	Digest    string    `json:"digest"`
	TrustedAt time.Time `json:"trusted_at"`
}

type trustStore struct {
	Version  int        `json:"version"`
	Projects []approval `json:"projects"`
}

var (
	userHomeDir = os.UserHomeDir
	nowUTC      = func() time.Time { return time.Now().UTC() }
)

func DefaultPath() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "cmdshape", "filter-trust.json"), nil
}

func CanonicalRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		var err error
		root, err = projectfiles.ResolveProjectRoot("")
		if err != nil {
			return "", err
		}
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project root links: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect project root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root is not a directory: %s", canonical)
	}
	return filepath.Clean(canonical), nil
}

func Evaluate(root string) (Decision, error) {
	decision, _, err := EvaluateSource(root)
	return decision, err
}

// EvaluateSource returns the exact safely opened bytes whose digest was
// evaluated. Runtime loaders must compile these bytes rather than reopening
// the source after trust has been decided.
func EvaluateSource(root string) (Decision, []SourceFile, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Decision{State: StateUnsafe, Reason: err.Error()}, nil, err
	}
	files, present, err := readSourceFiles(canonical)
	if err != nil {
		return Decision{Root: canonical, State: StateUnsafe, Reason: err.Error()}, nil, err
	}
	if !present {
		return Decision{Root: canonical, State: StateAbsent}, nil, nil
	}
	digest := sourceFilesDigest(canonical, files)
	store, err := loadDefaultStore()
	if err != nil {
		return Decision{Root: canonical, Digest: digest, State: StateUnsafe, Reason: err.Error()}, nil, err
	}
	for _, item := range store.Projects {
		if filepath.Clean(item.Root) != canonical {
			continue
		}
		if item.Digest == digest {
			return Decision{Root: canonical, Digest: digest, State: StateTrusted}, files, nil
		}
		return Decision{Root: canonical, Digest: digest, State: StateChanged}, files, nil
	}
	return Decision{Root: canonical, Digest: digest, State: StateUntrusted}, files, nil
}

func Trust(root string) (Decision, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Decision{}, err
	}
	digest, present, err := sourceDigest(canonical)
	if err != nil {
		return Decision{Root: canonical, State: StateUnsafe, Reason: err.Error()}, err
	}
	if !present {
		return Decision{Root: canonical, State: StateAbsent}, fmt.Errorf("no project filters found at %s", filepath.Join(canonical, ".cmdshape", "filters"))
	}
	store, err := loadDefaultStore()
	if err != nil {
		return Decision{}, err
	}
	next := approval{Root: canonical, Digest: digest, TrustedAt: nowUTC()}
	replaced := false
	for i := range store.Projects {
		if filepath.Clean(store.Projects[i].Root) == canonical {
			store.Projects[i] = next
			replaced = true
			break
		}
	}
	if !replaced {
		store.Projects = append(store.Projects, next)
	}
	slices.SortFunc(store.Projects, func(a, b approval) int {
		return strings.Compare(a.Root, b.Root)
	})
	if err := writeDefaultStore(store); err != nil {
		return Decision{}, err
	}
	return Decision{Root: canonical, Digest: digest, State: StateTrusted}, nil
}

func Untrust(root string) (Decision, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Decision{}, err
	}
	store, err := loadDefaultStore()
	if err != nil {
		return Decision{}, err
	}
	store.Projects = slices.DeleteFunc(store.Projects, func(item approval) bool {
		return filepath.Clean(item.Root) == canonical
	})
	if err := writeDefaultStore(store); err != nil {
		return Decision{}, err
	}
	decision, evalErr := Evaluate(canonical)
	if evalErr != nil {
		return decision, evalErr
	}
	return decision, nil
}

func sourceDigest(root string) (string, bool, error) {
	files, present, err := readSourceFiles(root)
	if err != nil || !present {
		return "", present, err
	}
	return sourceFilesDigest(root, files), true, nil
}

func readSourceFiles(root string) ([]SourceFile, bool, error) {
	filtersDir := filepath.Join(root, ".cmdshape", "filters")
	if err := projectfiles.RejectSymlinkPath(filtersDir); err != nil {
		return nil, false, fmt.Errorf("project filter source is unsafe: %w", err)
	}
	entries, err := os.ReadDir(filtersDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read project filters: %w", err)
	}

	names := projectFilterNames(entries)
	if len(names) == 0 {
		return nil, false, nil
	}
	slices.Sort(names)

	files := make([]SourceFile, 0, len(names))
	for _, name := range names {
		raw, err := readProjectFilter(root, filepath.Join(filtersDir, name), name)
		if err != nil {
			return nil, false, err
		}
		files = append(files, SourceFile{Name: name, Raw: raw})
	}
	return files, true, nil
}

func sourceFilesDigest(root string, files []SourceFile) string {
	hash := sha256.New()
	writeDigestPart(hash, digestDomain)
	writeDigestPart(hash, filepath.ToSlash(root))
	for _, file := range files {
		writeDigestPart(hash, filepath.ToSlash(file.Name))
		writeDigestBytes(hash, file.Raw)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func projectFilterNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if name == ".mappings.yaml" || name == ".mappings.yml" ||
			(!strings.HasPrefix(name, ".") && (filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml")) {
			names = append(names, name)
		}
	}
	return names
}

func readProjectFilter(root, path, name string) ([]byte, error) {
	file, err := projectfiles.OpenFileBeneath(root, path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("read project filter %s: %w", name, err)
	}
	raw, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read project filter %s: %w", name, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close project filter %s: %w", name, closeErr)
	}
	return raw, nil
}

func writeDigestPart(dst io.Writer, value string) {
	writeDigestBytes(dst, []byte(value))
}

func writeDigestBytes(dst io.Writer, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = dst.Write(size[:])
	_, _ = dst.Write(value)
}

func loadDefaultStore() (trustStore, error) {
	path, err := DefaultPath()
	if err != nil {
		return trustStore{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return trustStore{Version: storeVersion}, nil
	}
	if err != nil {
		return trustStore{}, fmt.Errorf("read filter trust store: %w", err)
	}
	var store trustStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return trustStore{}, fmt.Errorf("decode filter trust store: %w", err)
	}
	if store.Version != storeVersion {
		return trustStore{}, fmt.Errorf("filter trust store version must be exactly %d", storeVersion)
	}
	return store, nil
}

func writeDefaultStore(store trustStore) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	store.Version = storeVersion
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(path)
	if err := projectfiles.RejectSymlinkPath(path); err != nil {
		return fmt.Errorf("refuse unsafe filter trust path: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create filter trust directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure filter trust directory: %w", err)
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse non-regular filter trust store %q", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secure filter trust store: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := projectfiles.AtomicWriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write filter trust store: %w", err)
	}
	return nil
}

func WithTestHome(home string) func() {
	previousHome := userHomeDir
	previousNow := nowUTC
	userHomeDir = func() (string, error) { return home, nil }
	nowUTC = func() time.Time { return time.Unix(1, 0).UTC() }
	return func() {
		userHomeDir = previousHome
		nowUTC = previousNow
	}
}
