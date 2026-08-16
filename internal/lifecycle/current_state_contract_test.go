package lifecycle

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/SuppieRK/cmdshape/internal/workspaces"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("canonical cmdshape lifecycle state", func() {
	var ws lifecycleWorkspace

	BeforeEach(func() {
		ws = newLifecycleWorkspaceSpec()
	})

	It("treats a lifecycle lock at the maximum age boundary as stale", func() {
		lockPath, err := startupMaintenanceLockPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Dir(lockPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(lockPath, []byte("held"), 0o644)).To(Succeed())
		now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
		previousNow := startupMaintenanceNow
		startupMaintenanceNow = func() time.Time { return now }
		DeferCleanup(func() { startupMaintenanceNow = previousNow })
		Expect(os.Chtimes(lockPath, now.Add(-startupMaintenanceLockMaxAge), now.Add(-startupMaintenanceLockMaxAge))).To(Succeed())

		removed, err := removeStaleStartupMaintenanceLock(lockPath)

		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(BeTrue())
		Expect(lockPath).NotTo(BeAnExistingFile())
	})

	It("reclaims a stale lifecycle lock before canonical repair", func() {
		lockPath, err := startupMaintenanceLockPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Dir(lockPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(lockPath, []byte("stale"), 0o644)).To(Succeed())
		stale := time.Now().Add(-startupMaintenanceLockMaxAge - time.Second)
		Expect(os.Chtimes(lockPath, stale, stale)).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "version: 1\nmap:\n  gradlew: gradle\n",
			"gradle.yaml":    "version: 1\nfilter: gradle\nabout: test\n",
		})

		Expect(rewriteManagedRepairState()).To(Succeed())

		Expect(lockPath).NotTo(BeAnExistingFile())
		Expect(filepath.Join(ws.home, ".config", "cmdshape", "filters", "gradle.yaml")).To(BeAnExistingFile())
	})

	It("rewrites canonical state from the embedded shipped filter set", func() {
		previousStdout := repairStdout
		repairStdout = io.Discard
		DeferCleanup(func() { repairStdout = previousStdout })

		Expect(rewriteManagedRepairState()).To(Succeed())

		filtersDir := filepath.Join(ws.home, ".config", "cmdshape", "filters")
		Expect(filepath.Join(filtersDir, ".mappings.yaml")).To(BeAnExistingFile())
		Expect(filepath.Join(filtersDir, "gradle.yaml")).To(BeAnExistingFile())
	})

	It("preserves current registry, trust, and recovery state while removing unowned managed-home children", func() {
		configDir := filepath.Join(ws.home, ".config", "cmdshape")
		filtersDir := filepath.Join(configDir, "filters")
		recoveryDir := filepath.Join(configDir, "recovery")
		registryPath := workspaces.PathForHome(ws.home)
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(recoveryDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, startupMaintenanceLockName), []byte("held"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "filter-trust.json"), []byte("{}"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "recovery.json"), []byte(`{"enabled":true}`), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(recoveryDir, "failure.yaml"), []byte("retained"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filtersDir, "custom.yaml"), []byte("user-authored"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "stale.json"), []byte("stale"), 0o644)).To(Succeed())
		Expect(workspaces.UpsertPath(registryPath, ws.work, filepath.Join(ws.work, ".cmdshape", "gain.db"))).To(Succeed())

		Expect(cleanupManagedConfigDirWithPolicy(ws.home, true)).To(Succeed())

		for _, retained := range []string{
			filepath.Join(configDir, startupMaintenanceLockName),
			registryPath,
			filepath.Join(configDir, "filter-trust.json"),
			filepath.Join(configDir, "recovery.json"),
			filepath.Join(recoveryDir, "failure.yaml"),
			filepath.Join(filtersDir, "custom.yaml"),
		} {
			Expect(retained).To(BeAnExistingFile())
		}
		Expect(filepath.Join(configDir, "stale.json")).NotTo(BeAnExistingFile())
	})

	It("removes filters during rewrite cleanup while preserving other canonical state", func() {
		configDir := filepath.Join(ws.home, ".config", "cmdshape")
		filtersDir := filepath.Join(configDir, "filters")
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filtersDir, "custom.yaml"), []byte("custom"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "recovery.json"), []byte("{}"), 0o600)).To(Succeed())

		Expect(cleanupManagedConfigDir(ws.home)).To(Succeed())

		Expect(filtersDir).NotTo(BeAnExistingFile())
		Expect(filepath.Join(configDir, "recovery.json")).To(BeAnExistingFile())
	})

	It("treats a missing managed directory as already clean and reports invalid directory state", func() {
		configDir := filepath.Join(ws.home, ".config", "cmdshape")
		Expect(removeAllChildrenExcept(configDir)).To(Succeed())
		Expect(os.MkdirAll(filepath.Dir(configDir), 0o755)).To(Succeed())
		Expect(os.WriteFile(configDir, []byte("not a directory"), 0o644)).To(Succeed())

		Expect(removeAllChildrenExcept(configDir)).To(HaveOccurred())
	})
})
