package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"

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
		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "proxy output\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		commandArgs, stdoutLine, stderrLine := captureSuccessCommand()
		Expect(RunCapture(append([]string{"--dir", tmp, "--"}, commandArgs...))).To(Succeed())

		stdoutData, err := os.ReadFile(filepath.Join(tmp, captureStdoutFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stdoutData)).To(HaveSuffix("|" + stdoutLine))

		stderrData, err := os.ReadFile(filepath.Join(tmp, captureStderrFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stderrData)).To(HaveSuffix("|" + stderrLine))

		commandData, err := os.ReadFile(filepath.Join(tmp, replay.CommandFileName))
		Expect(err).NotTo(HaveOccurred())
		for _, arg := range commandArgs {
			Expect(string(commandData)).To(ContainSubstring(strconv.Quote(arg)))
		}

		outputData, err := os.ReadFile(filepath.Join(tmp, captureOutputFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputData)).To(Equal("proxy output\n"))
		Expect(stub.gotArgs).To(Equal(commandArgs))
		Expect(replay.ValidateSequence(stub.gotEvents)).To(Succeed())
		Expect(replay.CombinedInput(stub.gotEvents)).To(Or(
			Equal(stdoutLine+stderrLine),
			Equal(stderrLine+stdoutLine),
		))
		Expect(slices.ContainsFunc(stub.gotEvents, func(event replay.Event) bool {
			return event.Stream == contracts.StreamStdout && event.Line == stdoutLine
		})).To(BeTrue())
		Expect(slices.ContainsFunc(stub.gotEvents, func(event replay.Event) bool {
			return event.Stream == contracts.StreamStderr && event.Line == stderrLine
		})).To(BeTrue())
	})

	It("keeps artifacts when the native command exits non-zero", func() {
		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "captured failure\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		err := RunCapture(append([]string{"--dir", tmp, "--"}, captureFailureCommand()...))

		Expect(err).To(MatchError(ContainSubstring("exited with code 3")))
		Expect(filepath.Join(tmp, replay.CommandFileName)).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, captureStdoutFileName)).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, captureStderrFileName)).To(BeAnExistingFile())
		Expect(filepath.Join(tmp, captureOutputFileName)).To(BeAnExistingFile())
	})

	It("records capture outcomes in the audit log", func() {
		auditHome := GinkgoT().TempDir()
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "proxy output\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		commandArgs, _, _ := captureStdoutOnlyCommand()
		Expect(RunCapture(append([]string{"--dir", tmp, "--"}, commandArgs...))).To(Succeed())

		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"capture_invocation_start"`))
		Expect(string(auditData)).To(ContainSubstring(`"msg":"capture_invocation_finish"`))
		Expect(string(auditData)).To(ContainSubstring(`"success":true`))
		Expect(string(auditData)).To(ContainSubstring(`"command_path":` + strconv.Quote(filepath.Join(tmp, replay.CommandFileName))))
		Expect(string(auditData)).To(ContainSubstring(`"output_path":` + strconv.Quote(filepath.Join(tmp, captureOutputFileName))))
	})
})

func captureSuccessCommand() ([]string, string, string) {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "(echo native stdout) & (echo native stderr 1>&2)"}, "native stdout\n", "native stderr\n"
	}
	return []string{"sh", "-c", "printf 'native stdout\\n'; printf 'native stderr\\n' >&2"}, "native stdout\n", "native stderr\n"
}

func captureFailureCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "echo boom & exit /b 3"}
	}
	return []string{"sh", "-c", "printf 'boom\\n'; exit 3"}
}

func captureStdoutOnlyCommand() ([]string, string, string) {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "echo native stdout"}, "native stdout\n", ""
	}
	return []string{"sh", "-c", "printf 'native stdout\\n'"}, "native stdout\n", ""
}
