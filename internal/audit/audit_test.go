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
	It("uses ~/.config/ccp/audit/audit.log", func() {
		home := GinkgoT().TempDir()
		restore := WithTestConfig(home, 8, 7)
		DeferCleanup(restore)
		DeferCleanup(Reset)
		DeferCleanup(cleanupAuditHome, home)

		path, err := DefaultPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(filepath.Join(home, ".config", "ccp", "audit", "audit.log")))
	})

	It("rotates once the active log reaches the configured size limit", func() {
		home := GinkgoT().TempDir()
		restore := WithTestConfig(home, 1, 3)
		DeferCleanup(restore)
		DeferCleanup(Reset)
		DeferCleanup(cleanupAuditHome, home)

		payload := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8000)
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

	It("retains a bounded number of backup files", func() {
		home := GinkgoT().TempDir()
		restore := WithTestConfig(home, 1, 2)
		DeferCleanup(restore)
		DeferCleanup(Reset)
		DeferCleanup(cleanupAuditHome, home)

		payload := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8000)
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
