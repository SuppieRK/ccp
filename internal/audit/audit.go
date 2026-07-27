package audit

import (
	"context"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultMaxSizeMB  = 8
	defaultMaxBackups = 7
	auditTimeFormat   = "2006-01-02T15:04:05.000Z"
)

var (
	userHomeDir = os.UserHomeDir
	nowUTC      = func() time.Time { return time.Now().UTC() }

	maxSizeMB  = defaultMaxSizeMB
	maxBackups = defaultMaxBackups

	mu             sync.Mutex
	currentLogger  *slog.Logger
	currentHandler slog.Handler
	currentWriter  *lumberjack.Logger
)

func DefaultPath() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "cmdshape", "audit", "audit.log"), nil
}

func ConfigureDefault() error {
	path, err := DefaultPath()
	if err != nil {
		// Audit logging is best-effort only. cmdshape must preserve command execution even when
		// the audit home cannot be resolved on a particular machine or runner.
		disableLockedState()
		return nil
	}
	return ConfigurePath(path, maxSizeMB, maxBackups)
}

func ConfigurePath(path string, sizeMB int, backups int) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		// Do not turn audit-path permission problems into runtime failures. The caller may
		// be executing a real command whose native semantics must remain unobstructed.
		disableLockedState()
		return nil
	}

	closeCurrentWriterLocked()

	currentWriter = newRollingWriter(path, sizeMB, backups)
	currentHandler = slog.NewJSONHandler(currentWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Value = slog.StringValue(nowUTC().Format(auditTimeFormat))
			}
			return attr
		},
	})
	currentLogger = slog.New(currentHandler)
	slog.SetDefault(currentLogger)
	return nil
}

func Append(event string, fields map[string]any) error {
	mu.Lock()
	defer mu.Unlock()

	if err := ensureConfiguredLocked(); err != nil {
		// Audit writes must never block command execution, verify, or lifecycle flows.
		// If the log path is unavailable, we degrade to a no-op logger for this attempt.
		disableLockedState()
		return nil
	}

	record := slog.NewRecord(nowUTC(), slog.LevelInfo, event, 0)
	for _, attr := range attrsFor(fields) {
		record.AddAttrs(attr)
	}
	if err := currentHandler.Handle(context.Background(), record); err != nil {
		// lumberjack opens the file lazily on first write, so permission failures can show
		// up here even when ConfigureDefault succeeded earlier. Keep audit best-effort.
		disableLockedState()
		return nil
	}
	return nil
}

func newRollingWriter(path string, sizeMB int, backups int) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    sizeMB,
		MaxBackups: backups,
		LocalTime:  false,
		Compress:   false,
	}
}

func MustAppend(event string, fields map[string]any) {
	_ = Append(event, fields)
}

func WithTestConfig(home string, sizeMB int, backups int) func() {
	mu.Lock()
	defer mu.Unlock()

	prevHome := userHomeDir
	prevNow := nowUTC
	prevSize := maxSizeMB
	prevBackups := maxBackups
	counter := 0

	userHomeDir = func() (string, error) { return home, nil }
	nowUTC = func() time.Time {
		counter++
		return time.Unix(int64(counter), 0).UTC()
	}
	maxSizeMB = sizeMB
	maxBackups = backups
	disableLockedState()

	return func() {
		mu.Lock()
		defer mu.Unlock()
		userHomeDir = prevHome
		nowUTC = prevNow
		maxSizeMB = prevSize
		maxBackups = prevBackups
		disableLockedState()
	}
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	disableLockedState()
}

func ensureConfiguredLocked() error {
	if currentHandler != nil {
		return nil
	}
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	closeCurrentWriterLocked()
	currentWriter = newRollingWriter(path, maxSizeMB, maxBackups)
	currentHandler = slog.NewJSONHandler(currentWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Value = slog.StringValue(nowUTC().Format(auditTimeFormat))
			}
			return attr
		},
	})
	currentLogger = slog.New(currentHandler)
	slog.SetDefault(currentLogger)
	return nil
}

func closeCurrentWriterLocked() {
	if currentWriter != nil {
		_ = currentWriter.Close()
		currentWriter = nil
	}
}

func disableLockedState() {
	closeCurrentWriterLocked()
	currentLogger = nil
	currentHandler = nil
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func attrsFor(fields map[string]any) []slog.Attr {
	if len(fields) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(fields))

	attrs := make([]slog.Attr, 0, len(keys))
	for _, key := range keys {
		attrs = append(attrs, slog.Any(key, fields[key]))
	}
	return attrs
}
