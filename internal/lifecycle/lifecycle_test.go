package lifecycle

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go-command-compression-proxy/internal/lifecycle/agents"
	"go-command-compression-proxy/internal/workspaces"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type testAdapter struct {
	id   string
	plan []agents.PlannedArtifact
}

type failingInstallAdapter struct{}

type failingVerifyAdapter struct{}

func (t testAdapter) ID() string                         { return t.id }
func (t testAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }
func (t testAdapter) Install(_ agents.Context, write agents.WriterFunc) (agents.InstallResult, error) {
	if len(t.plan) == 0 {
		return agents.InstallResult{}, nil
	}
	_, err := write(t.plan[0].Path, []byte(t.plan[0].Content), t.plan[0].Perm)
	if err != nil {
		return agents.InstallResult{}, err
	}
	return agents.InstallResult{Applied: 1}, nil
}
func (t testAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return t.plan }
func (t testAdapter) Verify(_ agents.Context) error                  { return nil }

func (failingInstallAdapter) ID() string                         { return "broken-install" }
func (failingInstallAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }
func (failingInstallAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{}, errors.New("install failed")
}
func (failingInstallAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return nil }
func (failingInstallAdapter) Verify(_ agents.Context) error                  { return nil }

func (failingVerifyAdapter) ID() string                         { return "broken-verify" }
func (failingVerifyAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }
func (failingVerifyAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{Applied: 1}, nil
}
func (failingVerifyAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return nil }
func (failingVerifyAdapter) Verify(_ agents.Context) error                  { return errors.New("verify failed") }

var _ = Describe("Lifecycle helpers", func() {
	It("sorts and deduplicates parsed tools", func() {
		Expect(parseTools("Zeta,alpha, Alpha , zeta,,")).To(Equal([]string{"alpha", "zeta"}))
	})

	It("uses the working directory as the init detection root", func() {
		tmp, err := os.MkdirTemp("", "lifecycle-root-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(tmp) })

		withWorkingDir(tmp)

		root, err := initDetectRoot()
		Expect(err).NotTo(HaveOccurred())
		Expect(resolvedPath(root)).To(Equal(resolvedPath(tmp)))
	})

	It("creates managed files without backup artifacts", func() {
		tmp, err := os.MkdirTemp("", "lifecycle-managed-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(tmp) })

		path := filepath.Join(tmp, "cfg", "managed.txt")
		changed, err := writeManagedFile(path, []byte("v1\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		changed, err = writeManagedFile(path, []byte("v1\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		changed, err = writeManagedFile(path, []byte("v2\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		matches, err := filepath.Glob(path + ".bak.*")
		Expect(err).NotTo(HaveOccurred())
		Expect(matches).To(BeEmpty())
	})

	It("refuses to overwrite managed files through symlinked paths", func() {
		tmp, err := os.MkdirTemp("", "lifecycle-managed-symlink-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(tmp) })

		outsideDir := filepath.Join(tmp, "outside")
		Expect(os.MkdirAll(outsideDir, 0o755)).To(Succeed())
		outsideFile := filepath.Join(outsideDir, "managed.txt")
		Expect(os.WriteFile(outsideFile, []byte("keep me\n"), 0o644)).To(Succeed())

		linkDir := filepath.Join(tmp, "cfg")
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		changed, err := writeManagedFile(filepath.Join(linkDir, "managed.txt"), []byte("overwrite\n"), 0o644)
		Expect(err).To(HaveOccurred())
		Expect(changed).To(BeFalse())

		body, readErr := os.ReadFile(outsideFile)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me\n"))
	})
})

var _ = Describe("Adapter application", func() {
	It("applies a successful adapter and records state", func() {
		tmp, err := os.MkdirTemp("", "lifecycle-adapter-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(tmp) })

		adapter := testAdapter{
			id: "alpha",
			plan: []agents.PlannedArtifact{
				{Kind: agents.ArtifactHook, Path: filepath.Join(tmp, "hook.sh"), Content: "echo", Perm: 0o644},
			},
		}
		states, err := applyAdapters(agents.Context{ScopeRoot: tmp, HomeDir: tmp}, []string{"alpha"}, map[string]agents.Adapter{
			"alpha": adapter,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(states).To(HaveLen(1))
		Expect(states[0].Tool).To(Equal("alpha"))
	})

	It("returns failed state for install errors", func() {
		states, err := applyAdapters(
			agents.Context{ScopeRoot: GinkgoT().TempDir(), HomeDir: GinkgoT().TempDir()},
			[]string{"broken-install"},
			map[string]agents.Adapter{"broken-install": failingInstallAdapter{}},
		)
		Expect(err).To(MatchError(ContainSubstring("install failed")))
		expectSingleFailedStateWithReason(states, "install failed")
	})

	It("returns failed state for verify errors", func() {
		states, err := applyAdapters(
			agents.Context{ScopeRoot: GinkgoT().TempDir(), HomeDir: GinkgoT().TempDir()},
			[]string{"broken-verify"},
			map[string]agents.Adapter{"broken-verify": failingVerifyAdapter{}},
		)
		Expect(err).To(MatchError(ContainSubstring("verify failed")))
		expectSingleFailedStateWithReason(states, "verify failed")
	})
})

var _ = Describe("Startup maintenance", func() {
	var ws lifecycleWorkspace

	BeforeEach(func() {
		ws = newLifecycleWorkspaceSpec()
	})

	Context("when refreshing the managed home layout", func() {
		BeforeEach(func() {
			stubMaterializeHomeFiltersForSpec(map[string]string{
				".mappings.yaml": "version: 1\nmap:\n  fake: fake\n",
				"fake.yaml":      "version: 1\nfilter: fake\nabout: test\n",
			})
			Expect(os.MkdirAll(filepath.Join(ws.home, ".ccp"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(ws.home, ".ccp", "stale.txt"), []byte("stale"), 0o644)).To(Succeed())
		})

		It("replaces stale home files with the materialized filter set", func() {
			Expect(RunStartupMaintenance()).To(Succeed())

			_, err := os.Stat(filepath.Join(ws.home, ".config", "ccp", "filters", "fake.yaml"))
			Expect(err).NotTo(HaveOccurred())
			_, err = os.Stat(filepath.Join(ws.home, ".ccp", "stale.txt"))
			Expect(err).To(MatchError(os.ErrNotExist))
		})

		Context("and the maintenance lock is already held", func() {
			BeforeEach(func() {
				lockPath, err := startupMaintenanceLockPath()
				Expect(err).NotTo(HaveOccurred())
				Expect(os.MkdirAll(filepath.Dir(lockPath), 0o755)).To(Succeed())
				Expect(os.WriteFile(lockPath, []byte("held"), 0o644)).To(Succeed())
			})

			It("skips the refresh", func() {
				Expect(RunStartupMaintenance()).To(Succeed())

				_, err := os.Stat(filepath.Join(ws.home, ".config", "ccp", "filters", "fake.yaml"))
				Expect(err).To(MatchError(os.ErrNotExist))
			})
		})

		Context("and the maintenance lock is stale", func() {
			BeforeEach(func() {
				lockPath, err := startupMaintenanceLockPath()
				Expect(err).NotTo(HaveOccurred())
				Expect(os.MkdirAll(filepath.Dir(lockPath), 0o755)).To(Succeed())
				Expect(os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0o644)).To(Succeed())

				staleTime := time.Now().Add(-startupMaintenanceLockMaxAge - time.Second)
				Expect(os.Chtimes(lockPath, staleTime, staleTime)).To(Succeed())
			})

			It("reclaims the stale lock and refreshes filters", func() {
				lockPath, err := startupMaintenanceLockPath()
				Expect(err).NotTo(HaveOccurred())

				Expect(RunStartupMaintenance()).To(Succeed())

				_, err = os.Stat(lockPath)
				Expect(err).To(MatchError(os.ErrNotExist))
				_, err = os.Stat(filepath.Join(ws.home, ".config", "ccp", "filters", "fake.yaml"))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("when cleaning filesystem state outside init reconciliation", func() {
		It("cleans legacy project init state but preserves gain db", func() {
			ccpDir := filepath.Join(ws.work, ".ccp")
			Expect(os.MkdirAll(ccpDir, 0o755)).To(Succeed())

			stale := filepath.Join(ccpDir, initConfigFileName)
			backup := stale + ".bak.123"
			gainDB := filepath.Join(ccpDir, "gain.db")
			for _, file := range []string{stale, backup, gainDB} {
				Expect(os.WriteFile(file, []byte("x"), 0o644)).To(Succeed())
			}

			Expect(RunStartupMaintenance()).To(Succeed())

			_, err := os.Stat(stale)
			Expect(err).To(MatchError(os.ErrNotExist))

			_, err = os.Stat(backup)
			Expect(err).To(MatchError(os.ErrNotExist))

			_, err = os.Stat(gainDB)
			Expect(err).NotTo(HaveOccurred())
		})

		It("replaces managed config contents and refreshes home filters", func() {
			configDir := filepath.Join(ws.home, ".config", "ccp")
			customFilter := filepath.Join(configDir, "filters", "custom.yaml")
			Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(configDir, "state.json"), []byte("stale"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(configDir, "old.txt"), []byte("stale"), 0o644)).To(Succeed())
			Expect(os.MkdirAll(filepath.Dir(customFilter), 0o755)).To(Succeed())
			Expect(os.WriteFile(customFilter, []byte("version: 1\nfilter: custom\nabout: user\n"), 0o644)).To(Succeed())

			oldHomeFilters := filepath.Join(ws.home, ".ccp", "filters")
			Expect(os.MkdirAll(oldHomeFilters, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(oldHomeFilters, ".mappings.yaml"), []byte("old: old\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(oldHomeFilters, "old.yaml"), []byte("old"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(ws.home, ".ccp", "backup.txt"), []byte("stale"), 0o644)).To(Succeed())

			stubMaterializeHomeFiltersForSpec(map[string]string{
				".mappings.yaml": "version: 1\nmap:\n  npm: npm\n",
				"npm.yaml":       "version: 1\nfilter: npm\nabout: test\n",
			})

			Expect(RunStartupMaintenance()).To(Succeed())

			for _, removed := range []string{
				filepath.Join(configDir, "state.json"),
				filepath.Join(configDir, "old.txt"),
				filepath.Join(ws.home, ".ccp", "backup.txt"),
				filepath.Join(ws.home, ".ccp", "filters", "old.yaml"),
			} {
				_, err := os.Stat(removed)
				Expect(err).To(MatchError(os.ErrNotExist))
			}

			for _, copied := range []string{
				filepath.Join(ws.home, ".config", "ccp", "filters", ".mappings.yaml"),
				filepath.Join(ws.home, ".config", "ccp", "filters", "npm.yaml"),
				customFilter,
			} {
				_, err := os.Stat(copied)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("preserves the global workspace registry during sync", func() {
			configDir := filepath.Join(ws.home, ".config", "ccp")
			Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
			registryPath := workspaces.PathForHome(ws.home)
			Expect(workspaces.UpsertPath(registryPath, filepath.Join(ws.work, "repo"), filepath.Join(ws.work, "repo", ".ccp", "gain.db"))).To(Succeed())

			stubMaterializeHomeFiltersForSpec(map[string]string{
				".mappings.yaml": "version: 1\nmap:\n  npm: npm\n",
				"npm.yaml":       "version: 1\nfilter: npm\nabout: test\n",
			})

			Expect(syncCanonicalHomeLayout()).To(Succeed())

			entries, err := workspaces.ListPath(registryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
		})
	})

	It("rewrites managed state with embedded shipped filters on repair", func() {
		ws := newLifecycleWorkspaceSpec()
		configDir := filepath.Join(ws.home, ".config", "ccp")
		customFilter := filepath.Join(configDir, "filters", "custom.yaml")
		Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "old.txt"), []byte("stale"), 0o644)).To(Succeed())
		Expect(os.MkdirAll(filepath.Dir(customFilter), 0o755)).To(Succeed())
		Expect(os.WriteFile(customFilter, []byte("version: 1\nfilter: custom\nabout: user\n"), 0o644)).To(Succeed())

		Expect(RunRepair([]string{"--yes"})).To(Succeed())

		_, err := os.Stat(filepath.Join(configDir, "old.txt"))
		Expect(err).To(MatchError(os.ErrNotExist))
		_, err = os.Stat(filepath.Join(ws.home, ".config", "ccp", initConfigFileName))
		Expect(err).To(MatchError(os.ErrNotExist))
		for _, path := range []string{
			filepath.Join(ws.home, ".config", "ccp", "filters", ".mappings.yaml"),
			filepath.Join(ws.home, ".config", "ccp", "filters", "git.yaml"),
			filepath.Join(ws.home, ".config", "ccp", "filters", "npm.yaml"),
		} {
			_, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred(), path)
		}
		_, err = os.Stat(customFilter)
		Expect(err).To(MatchError(os.ErrNotExist))
	})
})

var _ = Describe("repair", func() {
	var ws lifecycleWorkspace

	BeforeEach(func() {
		ws = newLifecycleWorkspaceSpec()
	})

	It("rewrites managed home state with --yes", func() {
		configDir := filepath.Join(ws.home, ".config", "ccp")
		Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "old.txt"), []byte("stale"), 0o644)).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "git: git\n",
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
		})

		Expect(RunRepair([]string{"--yes"})).To(Succeed())

		_, err := os.Stat(filepath.Join(configDir, "old.txt"))
		Expect(err).To(MatchError(os.ErrNotExist))
		_, err = os.Stat(filepath.Join(configDir, "filters", "git.yaml"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("preserves the global workspace registry when rewriting state", func() {
		registryPath := workspaces.PathForHome(ws.home)
		Expect(workspaces.UpsertPath(registryPath, filepath.Join(ws.work, "repo"), filepath.Join(ws.work, "repo", ".ccp", "gain.db"))).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "version: 1\nmap:\n  git: git\n",
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
		})

		Expect(RunRepair([]string{"--yes"})).To(Succeed())

		entries, err := workspaces.ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
	})

	It("adds missing filters and mappings when the interactive prompt is declined", func() {
		prevIn := repairStdin
		prevOut := repairStdout
		repairStdin = strings.NewReader("n\n")
		repairStdout = io.Discard
		DeferCleanup(func() {
			repairStdin = prevIn
			repairStdout = prevOut
		})

		configDir := filepath.Join(ws.home, ".config", "ccp")
		filtersDir := filepath.Join(configDir, "filters")
		customFilter := filepath.Join(filtersDir, "custom.yaml")
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(configDir, "old.txt"), []byte("keep"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filtersDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  custom: custom\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(customFilter, []byte("version: 1\nfilter: custom\nabout: user\n"), 0o644)).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "version: 1\nmap:\n  git: git\n",
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
		})

		Expect(RunRepair(nil)).To(Succeed())

		_, err := os.Stat(filepath.Join(configDir, "old.txt"))
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Stat(filepath.Join(filtersDir, "git.yaml"))
		Expect(err).NotTo(HaveOccurred())
		body, err := os.ReadFile(customFilter)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("filter: custom"))
		body, err = os.ReadFile(filepath.Join(filtersDir, ".mappings.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("custom: custom"))
		Expect(string(body)).To(ContainSubstring("git: git"))
	})

	It("adds missing filters and mappings with --no without prompting", func() {
		prevIn := repairStdin
		prevOut := repairStdout
		repairStdin = strings.NewReader("y\n")
		repairStdout = io.Discard
		DeferCleanup(func() {
			repairStdin = prevIn
			repairStdout = prevOut
		})

		filtersDir := filepath.Join(ws.home, ".config", "ccp", "filters")
		customFilter := filepath.Join(filtersDir, "custom.yaml")
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(customFilter, []byte("version: 1\nfilter: custom\nabout: user\n"), 0o644)).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "version: 1\nmap:\n  git: git\n",
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
		})

		Expect(RunRepair([]string{"--no"})).To(Succeed())

		body, err := os.ReadFile(customFilter)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("filter: custom"))
		_, err = os.Stat(filepath.Join(filtersDir, "git.yaml"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("syncs missing shipped filters without overwriting existing user files", func() {
		filtersDir := filepath.Join(ws.home, ".config", "ccp", "filters")
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filtersDir, "git.yaml"), []byte("user override\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filtersDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  custom: custom\n"), 0o644)).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "version: 1\nmap:\n  git: git\n  npm: npm\n",
			"git.yaml":       "version: 1\nfilter: git\nabout: shipped\n",
			"npm.yaml":       "version: 1\nfilter: npm\nabout: shipped\n",
		})

		Expect(syncMissingPackagedFilters(ws.home)).To(Succeed())

		gitBody, err := os.ReadFile(filepath.Join(filtersDir, "git.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(gitBody)).To(Equal("user override\n"))

		npmBody, err := os.ReadFile(filepath.Join(filtersDir, "npm.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(npmBody)).To(ContainSubstring("filter: npm"))

		mappingsBody, err := os.ReadFile(filepath.Join(filtersDir, ".mappings.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(mappingsBody)).To(ContainSubstring("custom: custom"))
		Expect(string(mappingsBody)).To(ContainSubstring("git: git"))
		Expect(string(mappingsBody)).To(ContainSubstring("npm: npm"))
	})

	It("preserves invalid user mappings when add-missing mode encounters parse errors", func() {
		srcDir := GinkgoT().TempDir()
		srcPath := filepath.Join(srcDir, ".mappings.yaml")
		dstPath := filepath.Join(ws.home, ".config", "ccp", "filters", ".mappings.yaml")
		Expect(os.MkdirAll(filepath.Dir(dstPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(srcPath, []byte("version: 1\nmap:\n  git: git\n"), 0o644)).To(Succeed())
		original := []byte("not: [valid")
		Expect(os.WriteFile(dstPath, original, 0o644)).To(Succeed())

		err := mergeMissingMappings(srcPath, dstPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("decode mappings"))

		body, err := os.ReadFile(dstPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(original))
	})
})

func stubMaterializeHomeFiltersForSpec(files map[string]string) {
	prevMaterialize := materializeHomeFilters
	materializeHomeFilters = func(dst string) error {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dst, name), []byte(content), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	DeferCleanup(func() { materializeHomeFilters = prevMaterialize })
}

func expectSingleFailedStateWithReason(states []toolState, reason string) {
	Expect(states).To(HaveLen(1))
	Expect(states[0].Status).To(Equal("failed"))
	Expect(states[0].Reason).To(ContainSubstring(reason))
}

func newLifecycleWorkspaceSpec() lifecycleWorkspace {
	root, err := os.MkdirTemp("", "lifecycle-workspace-*")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(root) })

	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	Expect(os.MkdirAll(home, 0o755)).To(Succeed())
	Expect(os.MkdirAll(work, 0o755)).To(Succeed())
	setHomeDirForSpec(home)
	withWorkingDir(work)
	return lifecycleWorkspace{root: root, home: home, work: work}
}

func setHomeDirForSpec(home string) {
	restoreEnv("HOME", home)
	if runtime.GOOS != "windows" {
		return
	}

	restoreEnv("USERPROFILE", home)
	if vol := filepath.VolumeName(home); vol != "" {
		restoreEnv("HOMEDRIVE", vol)
		restoreEnv("HOMEPATH", strings.TrimPrefix(home, vol))
	}
}

func restoreEnv(key, value string) {
	old, had := os.LookupEnv(key)
	Expect(os.Setenv(key, value)).To(Succeed())
	DeferCleanup(func() {
		if had {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func withWorkingDir(dir string) {
	oldWd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chdir(dir)).To(Succeed())
	DeferCleanup(func() { _ = os.Chdir(oldWd) })
}

func resolvedPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}
