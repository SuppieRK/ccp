package filtertrust

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("project filter trust", func() {
	var (
		home    string
		project string
		filters string
	)

	BeforeEach(func() {
		home = GinkgoT().TempDir()
		project = GinkgoT().TempDir()
		filters = filepath.Join(project, ".ccp", "filters")
		Expect(os.MkdirAll(filters, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filters, "git.yaml"), []byte("version: 1\nfilter: git\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filters, ".mappings.yaml"), []byte("version: 1\nmap:\n  gs: git\n"), 0o644)).To(Succeed())
		restore := WithTestHome(home)
		DeferCleanup(restore)
	})

	It("moves from untrusted to trusted and changed as complete source bytes change", func() {
		decision, err := Evaluate(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(decision.State).To(Equal(StateUntrusted))

		trusted, err := Trust(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(trusted.State).To(Equal(StateTrusted))
		decision, err = Evaluate(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(decision).To(Equal(trusted))

		Expect(os.WriteFile(filepath.Join(filters, "git.yaml"), []byte("version: 1\nfilter: git\nabout: changed\n"), 0o644)).To(Succeed())
		decision, err = Evaluate(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(decision.State).To(Equal(StateChanged))
	})

	DescribeTable("invalidates approval for every source-set mutation",
		func(mutate func()) {
			_, err := Trust(project)
			Expect(err).NotTo(HaveOccurred())
			mutate()

			decision, err := Evaluate(project)

			Expect(err).NotTo(HaveOccurred())
			Expect(decision.State).To(Equal(StateChanged))
		},
		Entry("mapping content", func() {
			Expect(os.WriteFile(filepath.Join(filters, ".mappings.yaml"), []byte("version: 1\nmap:\n  git: git\n"), 0o644)).To(Succeed())
		}),
		Entry("added filter", func() {
			Expect(os.WriteFile(filepath.Join(filters, "go.yaml"), []byte("version: 1\nfilter: go\n"), 0o644)).To(Succeed())
		}),
		Entry("removed filter", func() {
			Expect(os.Remove(filepath.Join(filters, "git.yaml"))).To(Succeed())
		}),
		Entry("renamed filter", func() {
			Expect(os.Rename(filepath.Join(filters, "git.yaml"), filepath.Join(filters, "renamed.yaml"))).To(Succeed())
		}),
	)

	It("does not transfer trust between canonical projects with identical bytes", func() {
		other := GinkgoT().TempDir()
		otherFilters := filepath.Join(other, ".ccp", "filters")
		Expect(os.MkdirAll(otherFilters, 0o755)).To(Succeed())
		for _, name := range []string{"git.yaml", ".mappings.yaml"} {
			raw, err := os.ReadFile(filepath.Join(filters, name))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(otherFilters, name), raw, 0o644)).To(Succeed())
		}
		_, err := Trust(project)
		Expect(err).NotTo(HaveOccurred())

		decision, err := Evaluate(other)

		Expect(err).NotTo(HaveOccurred())
		Expect(decision.State).To(Equal(StateUntrusted))
	})

	It("uses the canonical project identity for root aliases", func() {
		alias := filepath.Join(GinkgoT().TempDir(), "project-link")
		if err := os.Symlink(project, alias); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		_, err := Trust(alias)
		Expect(err).NotTo(HaveOccurred())
		decision, err := Evaluate(project)

		Expect(err).NotTo(HaveOccurred())
		Expect(decision.State).To(Equal(StateTrusted))
		Expect(decision.Root).To(Equal(project))
	})

	It("rejects symlinked project filter sources and files", func() {
		outside := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(outside, "git.yaml"), []byte("version: 1\nfilter: git\n"), 0o644)).To(Succeed())
		Expect(os.RemoveAll(filters)).To(Succeed())
		if err := os.Symlink(outside, filters); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		decision, err := Evaluate(project)

		Expect(err).To(HaveOccurred())
		Expect(decision.State).To(Equal(StateUnsafe))
	})

	It("untrusts without deleting project files", func() {
		_, err := Trust(project)
		Expect(err).NotTo(HaveOccurred())

		decision, err := Untrust(project)

		Expect(err).NotTo(HaveOccurred())
		Expect(decision.State).To(Equal(StateUntrusted))
		Expect(filepath.Join(filters, "git.yaml")).To(BeAnExistingFile())
	})

	It("writes a private deterministic trust store", func() {
		_, err := Trust(project)
		Expect(err).NotTo(HaveOccurred())
		path, err := DefaultPath()
		Expect(err).NotTo(HaveOccurred())

		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		if runtime.GOOS != "windows" {
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
			dirInfo, statErr := os.Stat(filepath.Dir(path))
			Expect(statErr).NotTo(HaveOccurred())
			Expect(dirInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))
		}
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring(`"version": 1`))
		Expect(string(raw)).NotTo(ContainSubstring("version: 1\nfilter: git"))
	})

	It("handles absent sources and rejects non-directory roots", func() {
		empty := GinkgoT().TempDir()
		decision, err := Evaluate(empty)
		Expect(err).NotTo(HaveOccurred())
		Expect(decision.State).To(Equal(StateAbsent))
		_, err = Trust(empty)
		Expect(err).To(MatchError(ContainSubstring("no project filters found")))

		file := filepath.Join(GinkgoT().TempDir(), "not-a-directory")
		Expect(os.WriteFile(file, []byte("x"), 0o600)).To(Succeed())
		_, err = CanonicalRoot(file)
		Expect(err).To(MatchError(ContainSubstring("not a directory")))
		_, err = CanonicalRoot(filepath.Join(empty, "missing"))
		Expect(err).To(HaveOccurred())
	})

	It("replaces an existing approval deterministically", func() {
		first, err := Trust(project)
		Expect(err).NotTo(HaveOccurred())
		second, err := Trust(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))
	})

	DescribeTable("rejects invalid persisted trust state",
		func(contents string, expected string) {
			path, err := DefaultPath()
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())
			Expect(os.WriteFile(path, []byte(contents), 0o600)).To(Succeed())

			decision, err := Evaluate(project)

			Expect(err).To(MatchError(ContainSubstring(expected)))
			Expect(decision.State).To(Equal(StateUnsafe))
		},
		Entry("malformed JSON", "{", "decode filter trust store"),
		Entry("unsupported version", `{"version":2}`, "version must be exactly 1"),
	)

	It("propagates home resolution failures through trust operations", func() {
		previous := userHomeDir
		userHomeDir = func() (string, error) { return "", errors.New("home unavailable") }
		DeferCleanup(func() { userHomeDir = previous })

		_, err := DefaultPath()
		Expect(err).To(MatchError("home unavailable"))
		_, err = Trust(project)
		Expect(err).To(MatchError("home unavailable"))
		_, err = Untrust(project)
		Expect(err).To(MatchError("home unavailable"))
	})

	It("refuses a symlinked trust store target", func() {
		path, err := DefaultPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())
		outside := filepath.Join(home, "outside.json")
		Expect(os.WriteFile(outside, []byte(`{"version":1,"projects":[]}`), 0o600)).To(Succeed())
		if err := os.Symlink(outside, path); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		_, err = Trust(project)

		Expect(err).To(MatchError(ContainSubstring("refuse unsafe filter trust path")))
	})
})
