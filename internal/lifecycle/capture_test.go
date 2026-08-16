package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	core "github.com/SuppieRK/cmdshape/internal"
	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/replay"
)

type stubCaptureRunner struct {
	gotArgs      []string
	gotEvents    []replay.Event
	gotExitCode  int
	output       string
	stdout       string
	stderr       string
	decisions    string
	dispatch     string
	confidential []string
	err          error
}

type streamingPassthroughCaptureRunner struct {
	events   int
	maxEvent int
}

func (s *streamingPassthroughCaptureRunner) ReplaySequenceToWritersWithExitCode(_ []string, events iter.Seq2[replay.Event, error], _ int, writers core.ReplayWriters) (string, error) {
	for event, err := range events {
		if err != nil {
			return "", err
		}
		s.events++
		s.maxEvent = max(s.maxEvent, len(event.Line))
		if _, err := io.WriteString(writers.Output, event.Line); err != nil {
			return "", err
		}
		streamWriter := writers.Stdout
		if event.Stream == contracts.StreamStderr {
			streamWriter = writers.Stderr
		}
		if _, err := io.WriteString(streamWriter, event.Line); err != nil {
			return "", err
		}
	}
	return "passthrough", nil
}

func (s *stubCaptureRunner) ReplaySequenceToWritersWithExitCode(args []string, events iter.Seq2[replay.Event, error], exitCode int, writers core.ReplayWriters) (string, error) {
	s.gotArgs = append([]string(nil), args...)
	for event, err := range events {
		if err != nil {
			return "", err
		}
		s.gotEvents = append(s.gotEvents, event)
	}
	s.gotExitCode = exitCode
	for _, artifact := range []struct {
		writer   io.Writer
		contents string
	}{
		{writers.Output, s.output},
		{writers.Stdout, s.stdout},
		{writers.Stderr, s.stderr},
		{writers.Decisions, s.decisions},
	} {
		if _, err := io.WriteString(artifact.writer, redactCaptureText(artifact.contents, s.confidential)); err != nil {
			return "", err
		}
	}
	return s.dispatch, s.err
}

var _ = Describe("capture", func() {
	DescribeTable("resolving capture directories",
		func(dir string, wantCurrentDir bool) {
			resolved, err := resolveCaptureDir(dir, "demo")
			Expect(err).NotTo(HaveOccurred())

			if wantCurrentDir {
				cwd, cwdErr := os.Getwd()
				Expect(cwdErr).NotTo(HaveOccurred())
				Expect(resolved).To(Equal(cwd))
				return
			}

			expected, absErr := filepath.Abs(dir)
			Expect(absErr).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(expected))
		},
		Entry("uses the current working directory when no dir is provided", "", true),
		Entry("uses the provided directory when set", filepath.Join("testdata", "capture"), false),
	)

	It("writes command, sequenced streams, and cmdshape output artifacts", func() {
		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{
			output: "proxy output\n", stdout: "proxy stdout\n", stderr: "proxy stderr\n",
			decisions: "kept line\n", dispatch: "demo|default",
		}
		prev := newCaptureRunner
		newCaptureRunner = func([]string) captureVerifier { return stub }
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
		Expect(string(commandData)).To(ContainSubstring("exit_code: 0"))

		outputData, err := os.ReadFile(filepath.Join(tmp, captureOutputFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputData)).To(Equal("proxy output\n"))
		outputStdoutData, err := os.ReadFile(filepath.Join(tmp, captureOutputStdoutFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputStdoutData)).To(Equal("proxy stdout\n"))
		outputStderrData, err := os.ReadFile(filepath.Join(tmp, captureOutputStderrFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputStderrData)).To(Equal("proxy stderr\n"))
		decisionsData, err := os.ReadFile(filepath.Join(tmp, replay.DecisionsFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(decisionsData)).To(Equal("kept line\n"))
		dispatchData, err := os.ReadFile(filepath.Join(tmp, replay.VerifyDispatchFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(dispatchData)).To(Equal("demo|default\n"))
		Expect(filepath.Join(tmp, replay.DispatchFileName)).NotTo(BeAnExistingFile())
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
		dirInfo, err := os.Stat(tmp)
		Expect(err).NotTo(HaveOccurred())
		if runtime.GOOS != "windows" {
			Expect(dirInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))
		}
		for _, name := range []string{
			replay.CommandFileName,
			captureStdoutFileName,
			captureStderrFileName,
			captureOutputFileName,
			captureOutputStdoutFileName,
			captureOutputStderrFileName,
		} {
			info, statErr := os.Stat(filepath.Join(tmp, name))
			Expect(statErr).NotTo(HaveOccurred())
			if runtime.GOOS != "windows" {
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)), name)
			}
		}
	})

	It("redacts confidential argv and native streams and marks the fixture", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}
		auditHome := GinkgoT().TempDir()
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		tmp := GinkgoT().TempDir()
		secret := "capture-super-secret"
		stub := &stubCaptureRunner{
			output: "merged " + secret + "\n",
			stdout: "stdout " + secret + "\n",
			stderr: "stderr " + secret + "\n",
		}
		var gotConfidential []string
		prev := newCaptureRunner
		newCaptureRunner = func(confidential []string) captureVerifier {
			gotConfidential = slices.Clone(confidential)
			stub.confidential = slices.Clone(confidential)
			return stub
		}
		DeferCleanup(func() { newCaptureRunner = prev })

		Expect(RunCapture([]string{
			"--dir", tmp,
			"--confidential", secret,
			"--", "sh", "-c", "printf " + secret,
		})).To(Succeed())

		Expect(gotConfidential).To(Equal([]string{secret}))
		for _, name := range []string{
			replay.CommandFileName,
			captureStdoutFileName,
			captureOutputFileName,
			captureOutputStdoutFileName,
			captureOutputStderrFileName,
		} {
			body, err := os.ReadFile(filepath.Join(tmp, name))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).NotTo(ContainSubstring(secret))
		}
		for _, name := range []string{
			captureOutputFileName,
			captureOutputStdoutFileName,
			captureOutputStderrFileName,
		} {
			body, err := os.ReadFile(filepath.Join(tmp, name))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("***"))
		}
		command, err := replay.ReadCommand(filepath.Join(tmp, replay.CommandFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Redacted).To(BeTrue())
		Expect(strings.Join(command.Argv, " ")).To(ContainSubstring("***"))
		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "cmdshape", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).NotTo(ContainSubstring(secret))
		Expect(string(auditData)).To(ContainSubstring(`"command":"sh -c printf ***"`))
	})

	It("redacts confidential values split across capture records", func() {
		tmp := GinkgoT().TempDir()
		stdoutSrcPath := filepath.Join(tmp, "staged-stdout.txt")
		stderrSrcPath := filepath.Join(tmp, "staged-stderr.txt")
		stdoutDstPath := filepath.Join(tmp, replay.StdoutFileName)
		stderrDstPath := filepath.Join(tmp, replay.StderrFileName)
		prefix := strings.Repeat("x", replay.StreamRecordLimit-len("SE"))
		stdoutSrc, err := os.Create(stdoutSrcPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay.WriteSequencedEvent(stdoutSrc, replay.Event{Sequence: 0, Stream: contracts.StreamStdout, Line: prefix + "SE"})).To(Succeed())
		Expect(replay.WriteSequencedEvent(stdoutSrc, replay.Event{Sequence: 1, Stream: contracts.StreamStdout, Line: "CRET suffix"})).To(Succeed())
		Expect(stdoutSrc.Close()).To(Succeed())
		Expect(os.WriteFile(stderrSrcPath, nil, 0o600)).To(Succeed())

		Expect(writeCapturedStreams(stdoutSrcPath, stderrSrcPath, stdoutDstPath, stderrDstPath, []string{"SECRET"})).To(Succeed())
		events, err := replay.ReadEvents(stdoutDstPath, stderrDstPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(events).To(HaveLen(2))
		Expect(events[0].Sequence).To(Equal(0))
		Expect(events[1].Sequence).To(Equal(1))
		Expect(replay.CombinedInput(events)).To(Equal(prefix + "*** suffix"))
	})

	DescribeTable("redacts globally ordered confidential values across streams",
		func(events []replay.Event, confidential []string, expected string, replacementStream contracts.Stream) {
			tmp := GinkgoT().TempDir()
			stdoutSrcPath := filepath.Join(tmp, "staged-stdout.txt")
			stderrSrcPath := filepath.Join(tmp, "staged-stderr.txt")
			stdoutDstPath := filepath.Join(tmp, replay.StdoutFileName)
			stderrDstPath := filepath.Join(tmp, replay.StderrFileName)
			Expect(replay.WriteSequencedEventsMode(stdoutSrcPath, events, contracts.StreamStdout, 0o600)).To(Succeed())
			Expect(replay.WriteSequencedEventsMode(stderrSrcPath, events, contracts.StreamStderr, 0o600)).To(Succeed())

			Expect(writeCapturedStreams(stdoutSrcPath, stderrSrcPath, stdoutDstPath, stderrDstPath, confidential)).To(Succeed())

			redacted, err := replay.ReadEvents(stdoutDstPath, stderrDstPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(redacted).To(HaveLen(len(events)))
			for index := range events {
				Expect(redacted[index].Sequence).To(Equal(events[index].Sequence))
				Expect(redacted[index].Stream).To(Equal(events[index].Stream))
			}
			Expect(replay.CombinedInput(redacted)).To(Equal(expected))
			var replacementOutput strings.Builder
			for _, event := range redacted {
				if event.Stream == replacementStream {
					replacementOutput.WriteString(event.Line)
				}
			}
			Expect(replacementOutput.String()).To(ContainSubstring("***"))
		},
		Entry("assigns a cross-stream replacement to its first contributing stream",
			[]replay.Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "prefix SE"},
				{Sequence: 1, Stream: contracts.StreamStderr, Line: "CRET suffix"},
			},
			[]string{"SECRET"},
			"prefix *** suffix",
			contracts.StreamStdout,
		),
		Entry("preserves ordered nested ReplaceAll semantics across streams",
			[]replay.Event{
				{Sequence: 0, Stream: contracts.StreamStderr, Line: "ab"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "cab"},
			},
			[]string{"abc", "***ab"},
			"***",
			contracts.StreamStderr,
		),
		Entry("preserves non-overlapping ReplaceAll semantics for overlapping matches",
			[]replay.Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "ab"},
				{Sequence: 1, Stream: contracts.StreamStderr, Line: "aba"},
			},
			[]string{"aba", "bab"},
			"***ba",
			contracts.StreamStdout,
		),
	)

	It("keeps confidential replacement state bounded by the longest secret", func() {
		sink := &countingAttributedCaptureWriter{}
		root, stages := newAttributedCaptureWriter(sink, []string{"SECRET", "***-TOKEN"})

		for index := range 10_000 {
			stream := contracts.StreamStdout
			if index%2 == 1 {
				stream = contracts.StreamStderr
			}
			attribution := captureAttribution{sequence: index, stream: stream}
			Expect(root.WriteAttributed(attribution, []byte("noise"))).To(Succeed())
			for _, stage := range stages {
				Expect(len(stage.pending) - stage.head).To(BeNumerically("<=", len(stage.old)-1))
			}
		}
		for _, stage := range stages {
			Expect(stage.Flush()).To(Succeed())
		}
		Expect(sink.bytes).To(Equal(10_000 * len("noise")))
	})

	It("does not change permissions on an existing capture directory", func() {
		if runtime.GOOS == "windows" {
			Skip("Windows does not expose Unix directory permission bits")
		}
		tmp := GinkgoT().TempDir()
		Expect(os.Chmod(tmp, 0o755)).To(Succeed())
		stub := &stubCaptureRunner{}
		prev := newCaptureRunner
		newCaptureRunner = func([]string) captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		commandArgs, _, _ := captureStdoutOnlyCommand()
		Expect(RunCapture(append([]string{"--dir", tmp, "--"}, commandArgs...))).To(Succeed())

		info, err := os.Stat(tmp)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o755)))
	})

	It("keeps artifacts when the native command exits non-zero", func() {
		tmp := GinkgoT().TempDir()
		stub := &stubCaptureRunner{output: "captured failure\n"}
		prev := newCaptureRunner
		newCaptureRunner = func([]string) captureVerifier { return stub }
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
		newCaptureRunner = func([]string) captureVerifier { return stub }
		DeferCleanup(func() { newCaptureRunner = prev })

		commandArgs, _, _ := captureStdoutOnlyCommand()
		Expect(RunCapture(append([]string{"--dir", tmp, "--"}, commandArgs...))).To(Succeed())

		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "cmdshape", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"capture_invocation_start"`))
		Expect(string(auditData)).To(ContainSubstring(`"msg":"capture_invocation_finish"`))
		Expect(string(auditData)).To(ContainSubstring(`"success":true`))
		Expect(string(auditData)).To(ContainSubstring(`"command_path":` + strconv.Quote(filepath.Join(tmp, replay.CommandFileName))))
		Expect(string(auditData)).To(ContainSubstring(`"output_path":` + strconv.Quote(filepath.Join(tmp, captureOutputFileName))))
	})

	It("redacts confidential values from failed capture audit fields", func() {
		auditHome := GinkgoT().TempDir()
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)
		secret := "audit-super-secret"

		err := recordCaptureFailure(
			[]string{"demo", "--token=" + secret},
			filepath.Join("captures", secret),
			[]string{secret},
			"native_exec",
			errors.New("could not execute "+secret),
		)

		Expect(err).To(MatchError("could not execute " + secret))
		auditData, readErr := os.ReadFile(filepath.Join(auditHome, ".config", "cmdshape", "audit", "audit.log"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(auditData)).NotTo(ContainSubstring(secret))
		Expect(string(auditData)).To(ContainSubstring("***"))
	})

	It("preserves carriage-return progress output in captured events", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		events, exitCode, err := runNativeCapture([]string{"sh", "-c", "printf '\rstep 1\rstep 2\rdone\n'"})

		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(BeZero())
		Expect(replay.CombinedInput(events)).To(Equal("\rstep 1\rstep 2\rdone\n"))
		Expect(events).To(Equal([]replay.Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Line: "\r"},
			{Sequence: 1, Stream: contracts.StreamStdout, Line: "step 1\r"},
			{Sequence: 2, Stream: contracts.StreamStdout, Line: "step 2\r"},
			{Sequence: 3, Stream: contracts.StreamStdout, Line: "done\n"},
		}))
	})

	It("stages large newline-free native output on disk", func() {
		if runtime.GOOS == "windows" {
			Skip("uses POSIX stream utilities")
		}
		recording, exitCode, err := runNativeCaptureStaged([]string{
			"sh", "-c", fmt.Sprintf("head -c %d /dev/zero | tr '\\0' x", replay.StreamRecordLimit+17),
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(recording.cleanup)
		Expect(exitCode).To(BeZero())
		Expect(recording.stdoutBytes).To(Equal(replay.StreamRecordLimit + 17))
		Expect(recording.stderrBytes).To(BeZero())
		Expect(recording.stdoutPath).To(BeAnExistingFile())
	})

	It("streams large capture artifacts to disk and preserves a nonzero exit", func() {
		if runtime.GOOS == "windows" {
			Skip("uses POSIX stream utilities")
		}
		tmp := GinkgoT().TempDir()
		streaming := &streamingPassthroughCaptureRunner{}
		previous := newCaptureRunner
		newCaptureRunner = func([]string) captureVerifier { return streaming }
		DeferCleanup(func() { newCaptureRunner = previous })
		size := replay.StreamRecordLimit + 17

		err := executeCapture([]string{
			"sh", "-c", fmt.Sprintf("head -c %d /dev/zero | tr '\\0' x; exit 7", size),
		}, tmp, nil)

		exitErr, ok := errors.AsType[captureExitError](err)
		Expect(ok).To(BeTrue())
		Expect(exitErr.ExitCode()).To(Equal(7))
		for _, name := range []string{replay.OutputFileName, replay.OutputStdoutFileName} {
			info, statErr := os.Stat(filepath.Join(tmp, name))
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Size()).To(Equal(int64(size)))
		}
		stderrInfo, statErr := os.Stat(filepath.Join(tmp, replay.OutputStderrFileName))
		Expect(statErr).NotTo(HaveOccurred())
		Expect(stderrInfo.Size()).To(BeZero())
		Expect(streaming.events).To(Equal(2))
		Expect(streaming.maxEvent).To(Equal(replay.StreamRecordLimit))
		entries, readErr := os.ReadDir(tmp)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(slices.ContainsFunc(entries, func(entry os.DirEntry) bool {
			return strings.HasPrefix(entry.Name(), ".cmdshape-capture-output-")
		})).To(BeFalse())
	})

	It("returns startup errors from native capture immediately", func() {
		_, _, err := runNativeCaptureContext(context.Background(), []string{"cmdshape-command-that-does-not-exist"})
		Expect(err).To(HaveOccurred())
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

	It("flushes the final buffered line when EOF arrives", func() {
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
		stdout.sendByte('k')
		stdout.finish()

		Expect(<-done).NotTo(HaveOccurred())
		Expect(recorded).To(Equal([]replay.Event{{Sequence: 0, Stream: contracts.StreamStdout, Line: "ok"}}))
	})

	It("counts bytes only from the requested stream", func() {
		total := streamBytes([]replay.Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Line: "ok\n"},
			{Sequence: 1, Stream: contracts.StreamStderr, Line: "warn\n"},
			{Sequence: 2, Stream: contracts.StreamStdout, Line: "done"},
		}, contracts.StreamStdout)

		Expect(total).To(Equal(len("ok\n") + len("done")))
	})

	It("cancels captured subprocess trees when the execution context ends", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix process groups")
		}

		startedPath := filepath.Join(GinkgoT().TempDir(), "started.txt")
		childStartedPath := filepath.Join(GinkgoT().TempDir(), "child-started.txt")
		markerPath := filepath.Join(GinkgoT().TempDir(), "orphan.txt")
		originalHelper := os.Getenv("CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER")
		Expect(os.Setenv("CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER", "1")).To(Succeed())
		DeferCleanup(func() {
			if originalHelper == "" {
				Expect(os.Unsetenv("CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER")).To(Succeed())
				return
			}
			Expect(os.Setenv("CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER", originalHelper)).To(Succeed())
		})
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() {
			_, _, err := runNativeCaptureContext(ctx, []string{os.Args[0], "-test.run=TestCaptureManagedDescendantHelper", "--", startedPath, childStartedPath, markerPath})
			done <- err
		}()

		Eventually(func() error {
			_, err := os.Stat(startedPath)
			return err
		}, time.Second).Should(Succeed())
		Eventually(func() error {
			_, err := os.Stat(childStartedPath)
			return err
		}, time.Second).Should(Succeed())

		cancel()
		var err error
		Eventually(done, 5*time.Second).Should(Receive(&err))
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		Expect(ok).To(BeTrue())
		Expect(exitErr.ExitCode()).NotTo(BeZero())
		Consistently(func() bool {
			_, err := os.Stat(markerPath)
			return err == nil
		}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())
	})

	It("returns native capture events sorted by sequence after interleaved streams", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		events, exitCode, err := runNativeCapture([]string{"sh", "-c", "printf 'o'; sleep 0.1; printf 'k\\n'; printf 'e\\n' >&2"})

		Expect(err).NotTo(HaveOccurred())
		Expect(exitCode).To(BeZero())
		Expect(events).To(Equal([]replay.Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Line: "ok\n"},
			{Sequence: 1, Stream: contracts.StreamStderr, Line: "e\n"},
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
		newCaptureRunner = func([]string) captureVerifier { return stub }
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

func TestCaptureManagedDescendantHelper(t *testing.T) {
	if os.Getenv("CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER") != "1" {
		return
	}

	sep := slices.Index(os.Args, "--")
	if sep < 0 || len(os.Args) < sep+4 {
		os.Exit(2)
	}
	startedPath := os.Args[sep+1]
	childStartedPath := os.Args[sep+2]
	markerPath := os.Args[sep+3]
	if os.Getenv("CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER_MODE") == "child" {
		if err := os.WriteFile(childStartedPath, []byte("started"), 0o644); err != nil {
			os.Exit(5)
		}
		time.Sleep(3 * time.Second)
		if err := os.WriteFile(markerPath, []byte("orphan"), 0o644); err != nil {
			os.Exit(6)
		}
		os.Exit(0)
	}
	if err := os.WriteFile(startedPath, []byte("started"), 0o644); err != nil {
		os.Exit(3)
	}
	child := exec.Command(os.Args[0], "-test.run=TestCaptureManagedDescendantHelper", "--", startedPath, childStartedPath, markerPath)
	child.Env = append(os.Environ(), "CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER=1", "CMDSHAPE_CAPTURE_MANAGED_DESCENDANT_HELPER_MODE=child")
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Stdin = os.Stdin
	if err := child.Start(); err != nil {
		os.Exit(4)
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

type scriptedCaptureReader struct {
	steps chan scriptedCaptureStep
}

type countingAttributedCaptureWriter struct {
	bytes int
}

func (w *countingAttributedCaptureWriter) WriteAttributed(_ captureAttribution, payload []byte) error {
	w.bytes += len(payload)
	return nil
}

func (w *countingAttributedCaptureWriter) Flush() error { return nil }

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
