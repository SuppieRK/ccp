package audit

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("audit logging", func() {
	var (
		home         string
		maxSizeMB    int
		maxBackups   int
		cleanupAudit bool
	)

	BeforeEach(func() {
		home = GinkgoT().TempDir()
		maxSizeMB = 8
		maxBackups = 7
		cleanupAudit = true
	})

	JustBeforeEach(func() {
		restore := WithTestConfig(home, maxSizeMB, maxBackups)
		DeferCleanup(restore)
		DeferCleanup(Reset)
		if cleanupAudit {
			DeferCleanup(cleanupAuditHome, home)
		}
	})

	Context("when resolving the default log path", func() {
		It("uses ~/.config/ccp/audit/audit.log", func() {
			path, err := DefaultPath()
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal(filepath.Join(home, ".config", "ccp", "audit", "audit.log")))
		})

		It("returns the home directory error", func() {
			prevHome := userHomeDir
			userHomeDir = func() (string, error) { return "", os.ErrPermission }
			DeferCleanup(func() { userHomeDir = prevHome })

			_, err := DefaultPath()

			Expect(err).To(MatchError(os.ErrPermission))
		})
	})

	Context("when configuring the default logger", func() {
		It("degrades to a no-op logger when the home directory cannot be resolved", func() {
			prevHome := userHomeDir
			userHomeDir = func() (string, error) { return "", os.ErrPermission }
			DeferCleanup(func() { userHomeDir = prevHome })

			Expect(ConfigureDefault()).To(Succeed())
			Expect(currentHandler).To(BeNil())
			Expect(currentWriter).To(BeNil())
			Expect(MustAppend).NotTo(BeNil())
			Expect(Append("fallback", map[string]any{"ok": true})).To(Succeed())
		})
	})

	Context("when configuring a specific audit path", func() {
		It("creates the configured log file on first append", func() {
			path := filepath.Join(home, "logs", "audit.log")

			Expect(ConfigurePath(path, maxSizeMB, maxBackups)).To(Succeed())
			Expect(Append("configured", map[string]any{"ok": true})).To(Succeed())

			_, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			Reset()
		})
	})

	Context("when rotating audit logs", func() {
		var payload string

		BeforeEach(func() {
			maxSizeMB = 1
			payload = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8000)
		})

		It("rotates once the active log reaches the configured size limit", func() {
			for i := range 6 {
				Expect(Append("bulk", map[string]any{"index": i, "payload": payload})).To(Succeed())
			}
			Reset()

			active := filepath.Join(home, ".config", "ccp", "audit", "audit.log")
			entries, err := os.ReadDir(filepath.Dir(active))
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(2))
			names := []string{entries[0].Name(), entries[1].Name()}
			slices.Sort(names)
			Expect(names).To(ContainElement("audit.log"))
			rotated := 0
			for _, name := range names {
				if strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".log") {
					rotated++
				}
			}
			Expect(rotated).To(Equal(1))
		})

		Context("when backup retention is tightened", func() {
			BeforeEach(func() {
				maxBackups = 2
			})

			It("retains a bounded number of backup files", func() {
				for i := range 18 {
					Expect(Append("bulk", map[string]any{"index": i, "payload": payload})).To(Succeed())
				}
				Reset()

				auditDir := filepath.Join(home, ".config", "ccp", "audit")
				entries, err := os.ReadDir(auditDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(entries)).To(BeNumerically("<=", 4))
				names := make([]string, 0, len(entries))
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				slices.Sort(names)
				Expect(names).To(ContainElement("audit.log"))
				rotated := 0
				for _, name := range names {
					if strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".log") {
						rotated++
					}
				}
				Expect(rotated).To(BeNumerically("<=", 3))
			})
		})
	})

	Context("when the audit directory cannot be created", func() {
		BeforeEach(func() {
			cleanupAudit = false
		})

		It("degrades to no-op", func() {
			Expect(os.WriteFile(filepath.Join(home, ".config"), []byte("block"), 0o644)).To(Succeed())

			Expect(ConfigureDefault()).To(Succeed())
			Expect(Append("blocked", map[string]any{"ok": true})).To(Succeed())

			_, err := os.Stat(filepath.Join(home, ".config", "ccp", "audit", "audit.log"))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when appending through the must helper", func() {
		It("writes without surfacing errors", func() {
			Expect(ConfigureDefault()).To(Succeed())

			Expect(func() {
				MustAppend("must", map[string]any{"value": 1})
			}).NotTo(Panic())

			body, err := os.ReadFile(filepath.Join(home, ".config", "ccp", "audit", "audit.log"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`"msg":"must"`))
		})
	})

	Context("when test helpers configure deterministic time", func() {
		It("advances the synthetic clock on each read", func() {
			first := nowUTC()
			second := nowUTC()

			Expect(first).To(Equal(time.Unix(1, 0).UTC()))
			Expect(second).To(Equal(time.Unix(2, 0).UTC()))
		})
	})

	Context("when the handler fails during append", func() {
		It("degrades back to the disabled state", func() {
			boom := errors.New("handler boom")
			currentHandler = failingAuditHandler{err: boom}
			currentWriter = newRollingWriter(filepath.Join(home, "logs", "audit.log"), maxSizeMB, maxBackups)

			Expect(Append("broken", map[string]any{"ok": false})).To(Succeed())
			Expect(currentHandler).To(BeNil())
			Expect(currentWriter).To(BeNil())
		})
	})
})

type failingAuditHandler struct {
	err error
}

func (h failingAuditHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h failingAuditHandler) Handle(context.Context, slog.Record) error { return h.err }

func (h failingAuditHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h failingAuditHandler) WithGroup(string) slog.Handler { return h }

func cleanupAuditHome(home string) error {
	Reset()
	var lastErr error
	for range 10 {
		if err := os.RemoveAll(home); err == nil || os.IsNotExist(err) {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return lastErr
}
