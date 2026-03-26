package agents

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("ManagedContextFileAdapter", func() {
	var (
		tmpDir  string
		home    string
		ctx     Context
		adapter ManagedContextFileAdapter
		target  string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		home = filepath.Join(tmpDir, "home")
		ctx = Context{ScopeRoot: filepath.Join(tmpDir, "repo"), HomeDir: home}
		adapter = NewManagedContextFileAdapter(
			"alpha",
			".alpha",
			filepath.Join(".alpha", "AGENTS.md"),
			"missing alpha agents file: %s",
			"missing alpha managed markers in %s",
		)
		target = filepath.Join(home, ".alpha", "AGENTS.md")
		Expect(os.MkdirAll(filepath.Dir(target), 0o755)).To(Succeed())
	})

	ginkgo.It("preserves user content when uninstalling the managed block", func() {
		Expect(os.WriteFile(target, []byte("user header\n\n"+ccpManagedBlockTemplate()), 0o644)).To(Succeed())

		res, err := adapter.Uninstall(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Applied).To(Equal(1))

		got, err := os.ReadFile(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal("user header\n"))
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

var _ = ginkgo.Describe("managed instruction block helpers", func() {
	var (
		tmpDir string
		path   string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		path = filepath.Join(tmpDir, agentsFileName)
	})

	ginkgo.It("upserts the canonical block into a missing file", func() {
		updated, err := upsertManagedContextBlock(filepath.Join(tmpDir, "missing", agentsFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(updated).To(ContainSubstring(ccpManagedBlockStart))
		Expect(updated).To(ContainSubstring("Use `ccp` as the command prefix for every executable in shell commands, including chained (`&&`, `||`) and piped (`|`) expressions."))
		Expect(updated).To(ContainSubstring("`ccp echo chain-ok && ccp echo chain-done`"))
	})

	ginkgo.It("normalizes managed file content", func() {
		Expect(normalizeManagedFile("hello\n")).To(Equal("hello\n"))
	})

	ginkgo.It("requires the raw escape hatch when verifying managed files", func() {
		content := ccpManagedBlockStart + "\nmanaged guidance without raw retry\n" + ccpManagedBlockEnd + "\n"
		Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())

		err := verifyManagedContextBlock(path, "missing file: %s", "missing markers in %s")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing markers in "))
	})

	ginkgo.It("removes the managed block while preserving surrounding content", func() {
		Expect(os.WriteFile(path, []byte("start\n"+ccpManagedBlockTemplate()+"tail\n"), 0o644)).To(Succeed())

		out, changed, removeAll, err := removeManagedContextBlock(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(removeAll).To(BeFalse())
		Expect(out).NotTo(ContainSubstring(ccpManagedBlockStart))
	})

	ginkgo.It("appends the managed block when the file has no block yet", func() {
		Expect(os.WriteFile(path, []byte("prefix\nsuffix\n"), 0o644)).To(Succeed())

		out, err := upsertManagedContextBlock(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(ccpManagedBlockStart))
		Expect(out).To(ContainSubstring("prefix"))
	})

	ginkgo.It("uses the canonical template for an existing empty file", func() {
		Expect(os.WriteFile(path, nil, 0o644)).To(Succeed())

		out, err := upsertManagedContextBlock(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(ccpManagedBlockTemplate()))
	})

	ginkgo.It("replaces an existing managed block without duplicating it", func() {
		withBlock := "before\n" + ccpManagedBlockTemplate() + "\nafter\n"
		Expect(os.WriteFile(path, []byte(withBlock), 0o644)).To(Succeed())

		out, err := upsertManagedContextBlock(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(out, ccpManagedBlockStart)).To(Equal(1))
	})

	ginkgo.It("replaces an empty managed block without duplicating it", func() {
		withBlock := "before\n" + ccpManagedBlockStart + "\n" + ccpManagedBlockEnd + "\nafter\n"
		Expect(os.WriteFile(path, []byte(withBlock), 0o644)).To(Succeed())

		out, err := upsertManagedContextBlock(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(out, ccpManagedBlockStart)).To(Equal(1))
		Expect(out).To(ContainSubstring(ccpRawEscapeHatch))
		Expect(out).To(ContainSubstring("before"))
		Expect(out).To(ContainSubstring("after"))
	})

	ginkgo.It("treats malformed marker ordering as missing during upsert", func() {
		withBlock := "before\n" + ccpManagedBlockEnd + "\n" + ccpManagedBlockStart + "\nafter\n"
		Expect(os.WriteFile(path, []byte(withBlock), 0o644)).To(Succeed())

		out, err := upsertManagedContextBlock(path)

		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(out, ccpManagedBlockStart)).To(Equal(2))
		Expect(strings.Count(out, ccpManagedBlockEnd)).To(Equal(2))
		Expect(strings.TrimSuffix(out, "\n")).To(HaveSuffix(strings.TrimSuffix(ccpManagedBlockTemplate(), "\n")))
	})

	ginkgo.It("uses the canonical template when the file is missing", func() {
		out, err := upsertManagedContextBlock(filepath.Join(tmpDir, "missing", agentsFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(ccpManagedBlockTemplate()))
	})

	ginkgo.It("removes a file that contains only an empty managed block", func() {
		Expect(os.WriteFile(path, []byte(ccpManagedBlockStart+"\n"+ccpManagedBlockEnd+"\n"), 0o644)).To(Succeed())

		out, changed, removeAll, err := removeManagedContextBlock(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(removeAll).To(BeTrue())
		Expect(out).To(BeEmpty())
	})

	ginkgo.It("ignores malformed marker ordering during removal", func() {
		withBlock := "before\n" + ccpManagedBlockEnd + "\n" + ccpManagedBlockStart + "\nafter\n"
		Expect(os.WriteFile(path, []byte(withBlock), 0o644)).To(Succeed())

		out, changed, removeAll, err := removeManagedContextBlock(path)

		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeFalse())
		Expect(removeAll).To(BeFalse())
		Expect(out).To(BeEmpty())
	})

	ginkgo.DescribeTable("skipping a single trailing newline",
		func(input string, idx int, expected int) {
			Expect(skipSingleLF(input, idx)).To(Equal(expected))
		},
		ginkgo.Entry("advances when a newline is present", "block\nsuffix", len("block"), len("block")+1),
		ginkgo.Entry("keeps the same index when the next byte is not a newline", "blocksuffix", len("block"), len("block")),
		ginkgo.Entry("keeps the same index at end of input", "block", len("block"), len("block")),
	)
})
