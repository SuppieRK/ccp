package audit

import (
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
	})

	Context("when rotating audit logs", func() {
		var payload string

		BeforeEach(func() {
			maxSizeMB = 1
			payload = strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8000)
		})

		It("rotates once the active log reaches the configured size limit", func() {
			for i := 0; i < 6; i++ {
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
				for i := 0; i < 18; i++ {
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
})

func cleanupAuditHome(home string) error {
	Reset()
	auditDir := filepath.Join(home, ".config", "ccp", "audit")
	var lastErr error
	for range 10 {
		if err := os.RemoveAll(auditDir); err == nil || os.IsNotExist(err) {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return lastErr
}
