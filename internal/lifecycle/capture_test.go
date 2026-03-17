package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	core "go-command-compression-proxy/internal"
	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/replay"
)

type stubCaptureRunner struct {
	gotArgs   []string
	gotEvents []replay.Event
	output    string
	err       error
}

func (s *stubCaptureRunner) Replay(args []string, events []replay.Event) (core.ReplayResult, error) {
	s.gotArgs = append([]string(nil), args...)
	s.gotEvents = append([]replay.Event(nil), events...)
	return core.ReplayResult{Output: s.output}, s.err
}

var _ = Describe("capture", func() {
	It("writes command, sequenced streams, and CCP output artifacts", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "proxy output\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		Expect(RunCapture([]string{
			"--dir", tmp,
			"--",
			"sh", "-c", "printf 'native stdout\\n'; printf 'native stderr\\n' >&2",
		})).To(Succeed())

		stdoutData, err := os.ReadFile(filepath.Join(tmp, captureStdoutFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stdoutData)).To(HaveSuffix("|native stdout\n"))

		stderrData, err := os.ReadFile(filepath.Join(tmp, captureStderrFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stderrData)).To(HaveSuffix("|native stderr\n"))

		commandData, err := os.ReadFile(filepath.Join(tmp, replay.CommandFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(commandData)).To(ContainSubstring(`argv: ["sh", "-c", "printf 'native stdout\\n'; printf 'native stderr\\n' >&2"]`))

		outputData, err := os.ReadFile(filepath.Join(tmp, captureOutputFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputData)).To(Equal("proxy output\n"))
		Expect(stub.gotArgs).To(Equal([]string{"sh", "-c", "printf 'native stdout\\n'; printf 'native stderr\\n' >&2"}))
		Expect(replay.ValidateSequence(stub.gotEvents)).To(Succeed())
		Expect(replay.CombinedInput(stub.gotEvents)).To(Or(
			Equal("native stdout\nnative stderr\n"),
			Equal("native stderr\nnative stdout\n"),
		))
		Expect(slices.ContainsFunc(stub.gotEvents, func(event replay.Event) bool {
			return event.Stream == contracts.StreamStdout && event.Line == "native stdout\n"
		})).To(BeTrue())
		Expect(slices.ContainsFunc(stub.gotEvents, func(event replay.Event) bool {
			return event.Stream == contracts.StreamStderr && event.Line == "native stderr\n"
		})).To(BeTrue())
	})

	It("keeps artifacts when the native command exits non-zero", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "captured failure\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		err := RunCapture([]string{
			"--dir", tmp,
			"--",
			"sh", "-c", "printf 'boom\\n'; exit 3",
		})

		Expect(err).To(MatchError(ContainSubstring("exited with code 3")))
		Expect(filepath.Join(tmp, replay.CommandFileName)).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, captureStdoutFileName)).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, captureStderrFileName)).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, captureOutputFileName)).To(BeAnExistingFile())
	})

	It("records capture outcomes in the audit log", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		auditHome := GinkgoT().TempDir()
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "proxy output\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		Expect(RunCapture([]string{
			"--dir", tmp,
			"--",
			"sh", "-c", "printf 'native stdout\\n'",
		})).To(Succeed())

		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"capture_invocation_start"`))
		Expect(string(auditData)).To(ContainSubstring(`"msg":"capture_invocation_finish"`))
		Expect(string(auditData)).To(ContainSubstring(`"success":true`))
		Expect(string(auditData)).To(ContainSubstring(`"command_path":"` + filepath.Join(tmp, replay.CommandFileName) + `"`))
		Expect(string(auditData)).To(ContainSubstring(`"output_path":"` + filepath.Join(tmp, captureOutputFileName) + `"`))
	})
})
