package lifecycle

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

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

	It("orders partial cross-stream lines by first observed byte", func() {
		stdout := newScriptedCaptureReader()
		stderr := newScriptedCaptureReader()

		var (
			wg       sync.WaitGroup
			mu       sync.Mutex
			events   []replay.Event
			sequence atomic.Int64
		)
		record := func(seq int, stream contracts.Stream, line string) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, replay.Event{Sequence: seq, Stream: stream, Line: line})
		}
		errCh := make(chan error, 2)

		wg.Go(func() {
			errCh <- readSequencedCapture(stdout, contracts.StreamStdout, &sequence, record)
		})
		wg.Go(func() {
			errCh <- readSequencedCapture(stderr, contracts.StreamStderr, &sequence, record)
		})

		stdout.sendByte('o')
		stderr.sendByte('e')
		stderr.sendByte('\n')
		stdout.sendByte('k')
		stdout.sendByte('\n')
		stdout.finish()
		stderr.finish()
		wg.Wait()
		close(errCh)
		for err := range errCh {
			Expect(err).NotTo(HaveOccurred())
		}

		sort.Slice(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
		Expect(replay.ValidateSequence(events)).To(Succeed())
		Expect(events).To(Equal([]replay.Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Line: "ok\n"},
			{Sequence: 1, Stream: contracts.StreamStderr, Line: "e\n"},
		}))
	})

	It("returns non-EOF read failures instead of treating them as clean EOF", func() {
		stdout := newScriptedCaptureReader()

		var (
			recorded []replay.Event
			sequence atomic.Int64
		)
		record := func(seq int, stream contracts.Stream, line string) {
			recorded = append(recorded, replay.Event{Sequence: seq, Stream: stream, Line: line})
		}

		done := make(chan error, 1)
		go func() {
			done <- readSequencedCapture(stdout, contracts.StreamStdout, &sequence, record)
		}()

		stdout.sendByte('o')
		stdout.fail(io.ErrUnexpectedEOF)

		err := <-done
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, io.ErrUnexpectedEOF)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("read stdout stream"))
		Expect(recorded).To(Equal([]replay.Event{{Sequence: 0, Stream: contracts.StreamStdout, Line: "o"}}))
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

type scriptedCaptureReader struct {
	steps chan scriptedCaptureStep
}

type scriptedCaptureStep struct {
	data []byte
	err  error
	ack  chan struct{}
}

func newScriptedCaptureReader() *scriptedCaptureReader {
	return &scriptedCaptureReader{steps: make(chan scriptedCaptureStep)}
}

func (r *scriptedCaptureReader) Read(p []byte) (int, error) {
	step, ok := <-r.steps
	if !ok {
		return 0, os.ErrClosed
	}
	defer close(step.ack)
	if len(step.data) > 0 {
		p[0] = step.data[0]
		return 1, nil
	}
	return 0, step.err
}

func (r *scriptedCaptureReader) Close() error {
	return nil
}

func (r *scriptedCaptureReader) sendByte(b byte) {
	ack := make(chan struct{})
	r.steps <- scriptedCaptureStep{data: []byte{b}, ack: ack}
	<-ack
}

func (r *scriptedCaptureReader) finish() {
	ack := make(chan struct{})
	r.steps <- scriptedCaptureStep{err: io.EOF, ack: ack}
	<-ack
	close(r.steps)
}

func (r *scriptedCaptureReader) fail(err error) {
	ack := make(chan struct{})
	r.steps <- scriptedCaptureStep{err: err, ack: ack}
	<-ack
	close(r.steps)
}
