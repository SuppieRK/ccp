package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	core "go-command-compression-proxy/internal"
	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/replay"
)

type stubCaptureRunner struct {
	gotArgs     []string
	gotEvents   []replay.Event
	gotExitCode int
	output      string
	err         error
}

func (s *stubCaptureRunner) Replay(args []string, events []replay.Event) (core.ReplayResult, error) {
	return s.ReplayWithExitCode(args, events, 0)
}

func (s *stubCaptureRunner) ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (core.ReplayResult, error) {
	s.gotArgs = append([]string(nil), args...)
	s.gotEvents = append([]replay.Event(nil), events...)
	s.gotExitCode = exitCode
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
		Expect(normalizeCaptureLineEndings(string(stdoutData))).To(HaveSuffix("|" + stdoutLine))

		stderrData, err := os.ReadFile(filepath.Join(tmp, captureStderrFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(normalizeCaptureLineEndings(string(stderrData))).To(HaveSuffix("|" + stderrLine))

		commandData, err := os.ReadFile(filepath.Join(tmp, replay.CommandFileName))
		Expect(err).NotTo(HaveOccurred())
		for _, arg := range commandArgs {
			Expect(string(commandData)).To(ContainSubstring(strconv.Quote(arg)))
		}
		Expect(string(commandData)).NotTo(ContainSubstring("exit_code:"))

		outputData, err := os.ReadFile(filepath.Join(tmp, captureOutputFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputData)).To(Equal("proxy output\n"))
		Expect(stub.gotArgs).To(Equal(commandArgs))
		Expect(stub.gotExitCode).To(BeZero())
		Expect(replay.ValidateSequence(stub.gotEvents)).To(Succeed())
		Expect(normalizeCaptureLineEndings(replay.CombinedInput(stub.gotEvents))).To(Or(
			Equal(stdoutLine+stderrLine),
			Equal(stderrLine+stdoutLine),
		))
		Expect(slices.ContainsFunc(stub.gotEvents, func(event replay.Event) bool {
			return event.Stream == contracts.StreamStdout && normalizeCaptureLineEndings(event.Line) == stdoutLine
		})).To(BeTrue())
		Expect(slices.ContainsFunc(stub.gotEvents, func(event replay.Event) bool {
			return event.Stream == contracts.StreamStderr && normalizeCaptureLineEndings(event.Line) == stderrLine
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
		commandData, readErr := os.ReadFile(filepath.Join(tmp, replay.CommandFileName))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(commandData)).To(ContainSubstring("exit_code: 3"))
		Expect(stub.gotExitCode).To(Equal(3))
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

	It("preserves carriage-return progress output in captured events", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		events, exitCode, err := runNativeCapture([]string{"sh", "-c", "printf '\rstep 1\rstep 2\rdone\n'"})

		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(BeZero())
		Expect(replay.CombinedInput(events)).To(Equal("\rstep 1\rstep 2\rdone\n"))
		Expect(events).To(ContainElement(replay.Event{
			Sequence: 0,
			Stream:   contracts.StreamStdout,
			Line:     "\rstep 1\rstep 2\rdone\n",
		}))
	})

	It("refuses to overwrite symlinked capture artifacts", func() {
		tmp := GinkgoT().TempDir()
		target := filepath.Join(GinkgoT().TempDir(), "outside-output.txt")
		Expect(os.WriteFile(target, []byte("keep me"), 0o644)).To(Succeed())
		if err := os.Symlink(target, filepath.Join(tmp, captureOutputFileName)); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		stub := &stubCaptureRunner{output: "proxy output\n"}
		prev := newCaptureRunner
		newCaptureRunner = func() captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		commandArgs, _, _ := captureStdoutOnlyCommand()
		err := RunCapture(append([]string{"--dir", tmp, "--"}, commandArgs...))

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(target)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me"))
	})
})

func captureSuccessCommand() ([]string, string, string) {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "(echo native stdout)&((echo native stderr) 1>&2)"}, "native stdout\n", "native stderr\n"
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

func normalizeCaptureLineEndings(v string) string {
	return strings.ReplaceAll(v, "\r\n", "\n")
}
