package lifecycle

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"
	"github.com/SuppieRK/cmdshape/internal/version"
	"github.com/SuppieRK/cmdshape/internal/workspaces"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type testAdapter struct {
	id   string
	plan []agents.PlannedArtifact
}

type noopInstallAdapter struct {
	id   string
	plan []agents.PlannedArtifact
}

type emptyInstallAdapter struct {
	id string
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

func (n noopInstallAdapter) ID() string                         { return n.id }
func (n noopInstallAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }
func (n noopInstallAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{Noop: max(1, len(n.plan))}, nil
}
func (n noopInstallAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return n.plan }
func (n noopInstallAdapter) Verify(_ agents.Context) error                  { return nil }

func (n emptyInstallAdapter) ID() string                         { return n.id }
func (n emptyInstallAdapter) DetectRoot(scopeRoot string) string { return scopeRoot }
func (n emptyInstallAdapter) Install(_ agents.Context, _ agents.WriterFunc) (agents.InstallResult, error) {
	return agents.InstallResult{}, nil
}
func (n emptyInstallAdapter) Plan(_ agents.Context) []agents.PlannedArtifact { return nil }
func (n emptyInstallAdapter) Verify(_ agents.Context) error                  { return nil }

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

		root, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(resolvedPath(root)).To(Equal(resolvedPath(tmp)))
	})

	It("creates managed files without backup artifacts", func() {
		tmp, err := os.MkdirTemp("", "lifecycle-managed-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(tmp) })

		path := filepath.Join(tmp, "cfg", "managed.txt")
		changed, err := writeManagedBytes(path, []byte("v1\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())

		changed, err = writeManagedBytes(path, []byte("v1\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())

		changed, err = writeManagedBytes(path, []byte("v2\n"), 0o644)
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

		changed, err := writeManagedBytes(filepath.Join(linkDir, "managed.txt"), []byte("overwrite\n"), 0o644)
		Expect(err).To(HaveOccurred())
		Expect(changed).To(BeFalse())

		body, readErr := os.ReadFile(outsideFile)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me\n"))
	})
})

var _ = Describe("Adapter application", func() {
	captureStdout := func(run func() ([]toolState, error)) (string, []toolState, error) {
		orig := os.Stdout
		r, w, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		os.Stdout = w
		DeferCleanup(func() { os.Stdout = orig })

		states, runErr := run()
		Expect(w.Close()).To(Succeed())

		var buf strings.Builder
		_, err = io.Copy(&buf, r)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Close()).To(Succeed())
		return buf.String(), states, runErr
	}

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

	It("reports planned noop installs as noop states", func() {
		tmp := GinkgoT().TempDir()
		adapter := noopInstallAdapter{
			id: "alpha",
			plan: []agents.PlannedArtifact{
				{Kind: agents.ArtifactHook, Path: filepath.Join(tmp, "hook.sh"), Content: "echo", Perm: 0o644},
			},
		}

		stdout, states, err := captureStdout(func() ([]toolState, error) {
			return applyAdapters(agents.Context{ScopeRoot: tmp, HomeDir: tmp}, []string{"alpha"}, map[string]agents.Adapter{
				"alpha": adapter,
			})
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(states).To(Equal([]toolState{{
			Tool:   "alpha",
			Status: "noop",
			Reason: "applied=0 noop=1",
		}}))
		Expect(stdout).To(ContainSubstring("cmdshape init: planned 1 changes for alpha"))
		Expect(stdout).To(ContainSubstring("cmdshape init: [alpha] status=noop (applied=0 noop=1)"))
	})

	It("keeps zero-result installs in the applied state without a planned line", func() {
		tmp := GinkgoT().TempDir()
		adapter := emptyInstallAdapter{id: "alpha"}

		stdout, states, err := captureStdout(func() ([]toolState, error) {
			return applyAdapters(agents.Context{ScopeRoot: tmp, HomeDir: tmp}, []string{"alpha"}, map[string]agents.Adapter{
				"alpha": adapter,
			})
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(states).To(Equal([]toolState{{
			Tool:   "alpha",
			Status: "applied",
			Reason: "applied=0 noop=0",
		}}))
		Expect(stdout).NotTo(ContainSubstring("planned"))
		Expect(stdout).To(ContainSubstring("cmdshape init: [alpha] status=applied (applied=0 noop=0)"))
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

var _ = Describe("repair", func() {
	var ws lifecycleWorkspace

	BeforeEach(func() {
		ws = newLifecycleWorkspaceSpec()
	})

	It("rewrites managed home state with --yes", func() {
		configDir := filepath.Join(ws.home, ".config", "cmdshape")
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
		Expect(workspaces.UpsertPath(registryPath, filepath.Join(ws.work, "repo"), filepath.Join(ws.work, "repo", ".cmdshape", "gain.db"))).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			".mappings.yaml": "version: 1\nmap:\n  git: git\n",
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
		})

		Expect(RunRepair([]string{"--yes"})).To(Succeed())

		entries, err := workspaces.ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
	})

	DescribeTable("uses the lifecycle lock for repair modes",
		func(args []string) {
			lockPath, err := startupMaintenanceLockPath()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(filepath.Dir(lockPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(lockPath, []byte("held"), 0o644)).To(Succeed())

			stubMaterializeHomeFiltersForSpec(map[string]string{
				".mappings.yaml": "version: 1\nmap:\n  git: git\n",
				"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
			})

			err = RunRepair(args)

			Expect(err).To(HaveOccurred())
			_, statErr := os.Stat(filepath.Join(ws.home, ".config", "cmdshape", "filters", "git.yaml"))
			Expect(statErr).To(MatchError(os.ErrNotExist))
		},
		Entry("--yes", []string{"--yes"}),
		Entry("--no", []string{"--no"}),
	)

	It("adds missing filters and mappings when the interactive prompt is declined", func() {
		prevIn := repairStdin
		prevOut := repairStdout
		repairStdin = strings.NewReader("n\n")
		repairStdout = io.Discard
		DeferCleanup(func() {
			repairStdin = prevIn
			repairStdout = prevOut
		})

		configDir := filepath.Join(ws.home, ".config", "cmdshape")
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

		filtersDir := filepath.Join(ws.home, ".config", "cmdshape", "filters")
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
		filtersDir := filepath.Join(ws.home, ".config", "cmdshape", "filters")
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

	It("rejects mappings that omit the required version", func() {
		path := filepath.Join(GinkgoT().TempDir(), ".mappings.yaml")
		Expect(os.WriteFile(path, []byte("map:\n  npm: npm\n"), 0o644)).To(Succeed())

		_, err := readLifecycleMappings(path)

		Expect(err).To(MatchError(ContainSubstring("version must be exactly 1")))
	})

	It("returns normalized mappings without mutating source bytes", func() {
		path := filepath.Join(GinkgoT().TempDir(), ".mappings.yaml")
		original := []byte("version: 1\nmap:\n  \" npm \" : \" npm-target \"\n")
		Expect(os.WriteFile(path, original, 0o644)).To(Succeed())

		payload, err := readLifecycleMappings(path)

		Expect(err).NotTo(HaveOccurred())
		Expect(payload).To(Equal(lifecycleMappingsFile{Version: 1, Map: map[string]string{"npm": "npm-target"}}))
		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(original))
	})

	It("rejects zero-version destination mappings before merging missing aliases", func() {
		srcDir := GinkgoT().TempDir()
		srcPath := filepath.Join(srcDir, ".mappings.yaml")
		dstPath := filepath.Join(ws.home, ".config", "cmdshape", "filters", ".mappings.yaml")
		Expect(os.MkdirAll(filepath.Dir(dstPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(srcPath, []byte("version: 1\nmap:\n  git: git\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(dstPath, []byte("map:\n  custom: custom\n"), 0o644)).To(Succeed())

		err := mergeMissingMappings(srcPath, dstPath)

		Expect(err).To(MatchError(ContainSubstring("version must be exactly 1")))
	})

	It("preserves invalid user mappings when add-missing mode encounters parse errors", func() {
		srcDir := GinkgoT().TempDir()
		srcPath := filepath.Join(srcDir, ".mappings.yaml")
		dstPath := filepath.Join(ws.home, ".config", "cmdshape", "filters", ".mappings.yaml")
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

	It("returns add-missing sync errors without printing a success message", func() {
		prevMaterialize := materializeHomeFilters
		prevStdout := repairStdout
		materializeHomeFilters = func(string) error { return errors.New("sync failed") }
		repairStdout = io.Discard
		DeferCleanup(func() {
			materializeHomeFilters = prevMaterialize
			repairStdout = prevStdout
		})

		err := addMissingPackagedFilters()

		Expect(err).To(MatchError(ContainSubstring("sync failed")))
	})

	It("returns rewrite sync errors after acquiring the repair lock", func() {
		prevMaterialize := materializeHomeFilters
		prevStdout := repairStdout
		materializeHomeFilters = func(string) error { return errors.New("rewrite failed") }
		repairStdout = io.Discard
		DeferCleanup(func() {
			materializeHomeFilters = prevMaterialize
			repairStdout = prevStdout
		})

		err := rewriteManagedRepairState()

		Expect(err).To(MatchError(ContainSubstring("rewrite failed")))
	})

	DescribeTable("confirming repair through prompt I/O",
		func(stdin io.Reader, stdout io.Writer, expected bool, expectedErr string) {
			prevIn := repairStdin
			prevOut := repairStdout
			repairStdin = stdin
			repairStdout = stdout
			DeferCleanup(func() {
				repairStdin = prevIn
				repairStdout = prevOut
			})

			ok, err := confirmRepair()

			if expectedErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErr))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(Equal(expected))
		},
		Entry("accepts yes at EOF without a trailing newline", strings.NewReader("yes"), io.Discard, true, ""),
		Entry("propagates intro write failures", strings.NewReader("y\n"), &failingRepairWriter{failAt: 1, err: errors.New("write failed")}, false, "write failed"),
		Entry("propagates prompt write failures", strings.NewReader("y\n"), &failingRepairWriter{failAt: 2, err: errors.New("prompt failed")}, false, "prompt failed"),
		Entry("propagates non-EOF read failures", failingRepairReader{err: errors.New("read failed")}, io.Discard, false, "read failed"),
	)
})

var _ = Describe("release filter bootstrap", func() {
	var ws lifecycleWorkspace

	BeforeEach(func() {
		ws = newLifecycleWorkspaceSpec()
		previous := version.Version
		version.Version = "1.0.0"
		DeferCleanup(func() { version.Version = previous })
	})

	It("adds shipped filters on first use", func() {
		stubMaterializeHomeFiltersForSpec(map[string]string{
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
			".mappings.yaml": "version: 1\nmap:\n  git: git\n",
		})

		Expect(RunFilterBootstrap()).To(Succeed())

		filtersDir := filepath.Join(ws.home, ".config", "cmdshape", "filters")
		Expect(filepath.Join(filtersDir, "git.yaml")).To(BeAnExistingFile())
		Expect(filepath.Join(filtersDir, ".mappings.yaml")).To(BeAnExistingFile())
	})

	It("adds newly shipped filters and aliases to existing current state", func() {
		filtersDir := filepath.Join(ws.home, ".config", "cmdshape", "filters")
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		mappingsPath := filepath.Join(filtersDir, ".mappings.yaml")
		customPath := filepath.Join(filtersDir, "custom.yaml")
		Expect(os.WriteFile(mappingsPath, []byte("version: 1\nmap:\n  custom: custom\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(customPath, []byte("user-authored\n"), 0o644)).To(Succeed())
		stubMaterializeHomeFiltersForSpec(map[string]string{
			"git.yaml":       "version: 1\nfilter: git\nabout: test\n",
			".mappings.yaml": "version: 1\nmap:\n  git: git\n",
		})

		Expect(RunFilterBootstrap()).To(Succeed())

		body, err := os.ReadFile(customPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("user-authored\n"))
		Expect(filepath.Join(filtersDir, "git.yaml")).To(BeAnExistingFile())
		body, err = os.ReadFile(mappingsPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("custom: custom"))
		Expect(string(body)).To(ContainSubstring("git: git"))
	})

	It("does not block execution while another process holds the lifecycle lock", func() {
		lockPath, err := startupMaintenanceLockPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Dir(lockPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(lockPath, []byte("held"), 0o644)).To(Succeed())

		Expect(RunFilterBootstrap()).To(Succeed())
		Expect(filepath.Join(ws.home, ".config", "cmdshape", "filters", ".mappings.yaml")).NotTo(BeAnExistingFile())
	})
})

type failingRepairWriter struct {
	writes int
	failAt int
	err    error
}

func (w *failingRepairWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return len(p), nil
}

type failingRepairReader struct {
	err error
}

func (r failingRepairReader) Read([]byte) (int, error) {
	return 0, r.err
}

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
