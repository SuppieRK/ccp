package agents

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("pi adapter", func() {
	var ctx Context

	ginkgo.BeforeEach(func() {
		tmp := ginkgo.GinkgoT().TempDir()
		ctx = Context{ScopeRoot: filepath.Join(tmp, "repo"), HomeDir: filepath.Join(tmp, "home")}
		Expect(os.MkdirAll(filepath.Join(ctx.ScopeRoot, ".pi"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(ctx.HomeDir, 0o755)).To(Succeed())
	})

	ginkgo.It("installs cmdshape guidance into Pi-specific append system instructions instead of root AGENTS.md", func() {
		adapter := PiAdapter{}

		plan := adapter.Plan(ctx)
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Path).To(Equal(filepath.Join(ctx.ScopeRoot, ".pi", "APPEND_SYSTEM.md")))

		res, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Applied).To(Equal(1))
		Expect(adapter.Verify(ctx)).To(Succeed())

		body, err := os.ReadFile(filepath.Join(ctx.ScopeRoot, ".pi", "APPEND_SYSTEM.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("<!-- BEGIN: CMDSHAPE MANAGED BLOCK -->"))
		_, err = os.Stat(filepath.Join(ctx.ScopeRoot, "AGENTS.md"))
		Expect(err).To(MatchError(os.ErrNotExist))
	})

	ginkgo.It("removes current Pi guidance and legacy root AGENTS.md managed blocks", func() {
		adapter := PiAdapter{}
		installRes, err := adapter.Install(ctx, writeFileWriter)
		Expect(err).NotTo(HaveOccurred())
		Expect(installRes).To(Equal(InstallResult{Applied: 1}))
		legacyAgents := filepath.Join(ctx.ScopeRoot, "AGENTS.md")
		Expect(os.WriteFile(legacyAgents, []byte("team notes\n\n<!-- BEGIN: CCP MANAGED BLOCK -->\nlegacy\n<!-- END: CCP MANAGED BLOCK -->\n"), 0o644)).To(Succeed())

		res, err := adapter.Uninstall(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Applied).To(Equal(2))

		_, err = os.Stat(filepath.Join(ctx.ScopeRoot, ".pi", "APPEND_SYSTEM.md"))
		Expect(err).To(MatchError(os.ErrNotExist))
		legacyBody, err := os.ReadFile(legacyAgents)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(string(legacyBody))).To(Equal("team notes"))
	})
})
