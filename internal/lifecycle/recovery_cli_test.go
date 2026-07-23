package lifecycle

import (
	"os"
	"path/filepath"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/recovery"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("recovery commands", func() {
	BeforeEach(func() {
		configRoot := GinkgoT().TempDir()
		GinkgoT().Setenv("HOME", configRoot)
		GinkgoT().Setenv("XDG_CONFIG_HOME", configRoot)
		GinkgoT().Setenv("AppData", configRoot)
	})

	It("validates actions and renders help", func() {
		Expect(RunRecovery(nil)).To(MatchError("usage: ccp recovery enable|disable|list|purge"))
		Expect(RunRecovery([]string{"enable", "extra"})).To(MatchError("ccp recovery enable does not accept arguments"))
		Expect(RunRecovery([]string{"unknown"})).To(MatchError(`unknown recovery action "unknown"`))

		out, err := captureStdout(func() error { return RunRecovery([]string{"help"}) })
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("ccp recovery - manage opt-in bounded raw failure recovery"))
		Expect(out).To(ContainSubstring("disabled by default"))
	})

	It("enables, lists, disables, and purges recovery artifacts", func() {
		out, err := captureStdout(func() error { return RunRecovery([]string{"enable"}) })
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("Recovery enabled.\n"))

		artifact, err := recovery.Store(
			[]string{"demo", "test"},
			[]recovery.Event{{Sequence: 0, Stream: contracts.StreamStderr, Data: []byte("failed\n")}},
			9,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(artifact).NotTo(BeNil())

		out, err = captureStdout(func() error { return RunRecovery([]string{"list"}) })
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("exit=9"))
		Expect(out).To(ContainSubstring(filepath.Base(artifact.Path)))

		out, err = captureStdout(func() error { return RunRecovery([]string{"purge"}) })
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("Purged 1 recovery artifacts.\n"))

		out, err = captureStdout(func() error { return RunRecovery([]string{"list"}) })
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("No recovery artifacts.\n"))

		out, err = captureStdout(func() error { return RunRecovery([]string{"disable"}) })
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("Recovery disabled.\n"))

		configPath, err := recovery.ConfigPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(configPath).To(BeAnExistingFile())
		_, err = os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
	})
})
