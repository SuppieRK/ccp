package lifecycle

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	core "go-command-compression-proxy/internal"
	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/replay"
	"go-command-compression-proxy/internal/version"
)

type stubVerifyRunner struct {
	gotArgs     []string
	gotEvents   []replay.Event
	gotExitCode int
	output      string
	decisions   string
	err         error
}

func (s *stubVerifyRunner) ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (core.ReplayResult, error) {
	s.gotArgs = append([]string(nil), args...)
	s.gotEvents = append([]replay.Event(nil), events...)
	s.gotExitCode = exitCode
	return core.ReplayResult{Output: s.output, Decisions: s.decisions}, s.err
}

var _ = Describe("verify", func() {
	captureStderr := func(fn func() error) string {
		orig := os.Stderr
		r, w, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		os.Stderr = w
		DeferCleanup(func() { os.Stderr = orig })

		Expect(fn()).To(Succeed())
		Expect(w.Close()).To(Succeed())

		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Close()).To(Succeed())
		return buf.String()
	}

	It("renders help output", func() {
		out := captureStderr(func() error { return RunVerify([]string{"--help"}) })
		for _, part := range []string{
			"ccp verify - replay captured fixtures through the current filter",
			"Usage:",
			"Flags:",
			"Notes:",
			"--dir",
			"verify-output.txt",
			"verify-decisions.txt",
		} {
			Expect(out).To(ContainSubstring(part))
		}
	})

	It("rejects non-dev builds", func() {
		restoreVersion := setVersionForTest("v1.2.3")
		DeferCleanup(restoreVersion)

		err := RunVerify(nil)
		Expect(err).To(MatchError(ContainSubstring("only available in dev builds")))
	})

	It("replays fixtures and writes verify artifacts", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		stub := &stubVerifyRunner{
			output:    "filtered output\n",
			decisions: "<keep>    | native stdout\n",
		}
		DeferCleanup(stubVerifyRunnerForTest(stub))

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"status\"]\n")
		writeFileForTest(filepath.Join(tmp, replay.StdoutFileName), "00000|native stdout\n")
		writeFileForTest(filepath.Join(tmp, replay.StderrFileName), "00001|native stderr\n")

		err := RunVerify([]string{"--dir", tmp})

		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Join(stub.gotArgs, " ")).To(Equal("git status"))
		Expect(stub.gotExitCode).To(BeZero())
		Expect(stub.gotEvents).To(Equal([]replay.Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Line: "native stdout\n"},
			{Sequence: 1, Stream: contracts.StreamStderr, Line: "native stderr\n"},
		}))
		outputData, err := os.ReadFile(filepath.Join(tmp, replay.VerifyOutputFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outputData)).To(Equal("filtered output\n"))
		decisionData, err := os.ReadFile(filepath.Join(tmp, replay.VerifyDecisionsFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(decisionData)).To(Equal("<keep>    | native stdout\n"))
	})

	It("records verify invocation outcomes in the audit log", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		auditHome := GinkgoT().TempDir()
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		stub := &stubVerifyRunner{output: "filtered output\n"}
		DeferCleanup(stubVerifyRunnerForTest(stub))

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"status\"]\n")
		writeFileForTest(filepath.Join(tmp, replay.StdoutFileName), "00000|native stdout\n")

		err := RunVerify([]string{"--dir", tmp})

		Expect(err).NotTo(HaveOccurred())
		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"verify_invocation_start"`))
		Expect(string(auditData)).To(ContainSubstring(`"msg":"verify_invocation_finish"`))
		Expect(string(auditData)).To(ContainSubstring(`"success":true`))
		Expect(string(auditData)).To(ContainSubstring(`"verify_output":` + strconv.Quote(filepath.Join(tmp, replay.VerifyOutputFileName))))
	})

	It("does not fail verify when audit logging cannot initialize", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		auditHome := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(auditHome, ".config"), []byte("block"), 0o644)).To(Succeed())
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		stub := &stubVerifyRunner{output: "filtered output\n"}
		DeferCleanup(stubVerifyRunnerForTest(stub))

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"status\"]\n")
		writeFileForTest(filepath.Join(tmp, replay.StdoutFileName), "00000|native stdout\n")

		err := RunVerify([]string{"--dir", tmp})

		Expect(err).NotTo(HaveOccurred())
		output, readErr := os.ReadFile(filepath.Join(tmp, replay.VerifyOutputFileName))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(output)).To(Equal("filtered output\n"))
	})

	It("records verify invocation failures in the audit log", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		auditHome := GinkgoT().TempDir()
		restoreAudit := audit.WithTestConfig(auditHome, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		stub := &stubVerifyRunner{output: "actual line\n"}
		DeferCleanup(stubVerifyRunnerForTest(stub))

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"status\"]\n")
		writeFileForTest(filepath.Join(tmp, replay.StdoutFileName), "oops\n")

		err := RunVerify([]string{"--dir", tmp})

		Expect(err).To(HaveOccurred())
		auditData, err := os.ReadFile(filepath.Join(auditHome, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"verify_invocation_finish"`))
		Expect(string(auditData)).To(ContainSubstring(`"success":false`))
		Expect(string(auditData)).To(ContainSubstring(`"stage":"read_events"`))
	})

	It("refuses to overwrite symlinked verify artifacts", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		stub := &stubVerifyRunner{output: "filtered output\n", decisions: "<keep>    | native stdout\n"}
		DeferCleanup(stubVerifyRunnerForTest(stub))

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"status\"]\n")
		writeFileForTest(filepath.Join(tmp, replay.StdoutFileName), "00000|native stdout\n")

		target := filepath.Join(GinkgoT().TempDir(), "outside-verify-output.txt")
		Expect(os.WriteFile(target, []byte("keep me"), 0o644)).To(Succeed())
		if err := os.Symlink(target, filepath.Join(tmp, replay.VerifyOutputFileName)); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := RunVerify([]string{"--dir", tmp})

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(target)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me"))
	})

	It("rejects command arguments", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		err := RunVerify([]string{"--dir", GinkgoT().TempDir(), "--", "git", "status"})
		Expect(err).To(MatchError(ContainSubstring("does not accept command arguments")))
	})

	It("fails when replay sequence prefixes break ordering", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"status\"]\n")
		writeFileForTest(filepath.Join(tmp, replay.StdoutFileName), "00000|alpha\n")
		writeFileForTest(filepath.Join(tmp, replay.StderrFileName), "00002|beta\n")

		err := RunVerify([]string{"--dir", tmp})
		Expect(err).To(MatchError(ContainSubstring("sequence")))
	})

	It("passes fixture exit codes to the runner", func() {
		restoreVersion := setVersionForTest("dev")
		DeferCleanup(restoreVersion)

		stub := &stubVerifyRunner{output: "filtered output\n"}
		DeferCleanup(stubVerifyRunnerForTest(stub))

		tmp := GinkgoT().TempDir()
		writeFileForTest(filepath.Join(tmp, replay.CommandFileName), "argv: [\"git\", \"show\"]\nexit_code: 128\n")
		writeFileForTest(filepath.Join(tmp, replay.StderrFileName), "00000|fatal: not a git repository: '.git'\n")

		err := RunVerify([]string{"--dir", tmp})

		Expect(err).NotTo(HaveOccurred())
		Expect(stub.gotExitCode).To(Equal(128))
	})
})

func stubVerifyRunnerForTest(runner verifyRunner) func() {
	prev := newVerifyRunner
	newVerifyRunner = func() verifyRunner { return runner }
	return func() { newVerifyRunner = prev }
}

func setVersionForTest(value string) func() {
	prev := version.Version
	version.Version = value
	return func() { version.Version = prev }
}

func writeFileForTest(path, content string) {
	Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
}
