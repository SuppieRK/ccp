package agents

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("rule files family", func() {
	var (
		tmpDir string
		home   string
		repo   string
		ctx    Context
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		home = filepath.Join(tmpDir, "home")
		repo = filepath.Join(tmpDir, "repo")
		ctx = Context{ScopeRoot: repo, HomeDir: home}
	})

	ginkgo.Describe("ManagedHomeRuleFileAdapter", func() {
		var adapter ManagedHomeRuleFileAdapter

		ginkgo.BeforeEach(func() {
			adapter = NewManagedHomeRuleFileAdapter(
				"alpha",
				".alpha",
				filepath.Join(".alpha", "rules", "cmdshape.md"),
				"missing alpha rule file: %s",
				"missing alpha managed guidance in %s",
				func() string { return "cmdshape-managed\n" },
				[]string{"cmdshape-managed"},
			)
		})

		ginkgo.It("detects in the repo while installing into the home-scoped target", func() {
			Expect(adapter.DetectRoot(repo)).To(Equal(filepath.Join(repo, ".alpha")))

			plan := adapter.Plan(ctx)
			Expect(plan).To(HaveLen(1))
			Expect(plan[0].Path).To(HavePrefix(home))

			_, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter.Verify(ctx)).To(Succeed())
		})

		ginkgo.It("reports reinstalling unchanged managed content as noop", func() {
			first, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(first.Applied).To(Equal(1))

			second, err := adapter.Install(ctx, writeFileWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(second).To(Equal(InstallResult{Noop: 1}))
		})
	})

	ginkgo.Describe("ManagedRepoRuleFileAdapter", func() {
		var (
			adapter ManagedRepoRuleFileAdapter
			target  string
			sibling string
		)

		ginkgo.BeforeEach(func() {
			adapter = NewManagedRepoRuleFileAdapter(
				"alpha",
				".alpha",
				filepath.Join(".alpha", "rules", "cmdshape.md"),
				"missing alpha rule file: %s",
				"missing alpha managed guidance in %s",
				func() string { return "cmdshape-managed\n" },
				[]string{"cmdshape-managed"},
			)
			target = filepath.Join(repo, ".alpha", "rules", "cmdshape.md")
			sibling = filepath.Join(repo, ".alpha", "rules", "user.md")
		})

		ginkgo.It("removes only the managed rule file and preserves sibling files", func() {
			Expect(os.MkdirAll(filepath.Dir(target), 0o755)).To(Succeed())
			Expect(os.WriteFile(target, []byte("cmdshape-managed\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(sibling, []byte("keep-me\n"), 0o644)).To(Succeed())

			res, err := adapter.Uninstall(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Applied).To(Equal(1))
			_, statErr := os.Stat(target)
			Expect(errors.Is(statErr, os.ErrNotExist)).To(BeTrue())

			got, err := os.ReadFile(sibling)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got)).To(Equal("keep-me\n"))
		})

		ginkgo.It("fails verification when required managed guidance is missing", func() {
			Expect(os.MkdirAll(filepath.Dir(target), 0o755)).To(Succeed())
			Expect(os.WriteFile(target, []byte("user-only\n"), 0o644)).To(Succeed())

			err := adapter.Verify(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing alpha managed guidance"))
		})
	})
})
