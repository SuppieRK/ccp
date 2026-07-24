package replay

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("replay fixtures", func() {
	Describe("command yaml", func() {
		It("writes and reads flow-style argv", func() {
			path := filepath.Join(GinkgoT().TempDir(), CommandFileName)

			Expect(WriteCommandWithExitCode(path, []string{"grep", "-r", "-n", "needle", "./internal"}, 0)).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`argv: ["grep", "-r", "-n", "needle", "./internal"]`))

			spec, err := ReadCommand(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Argv).To(Equal([]string{"grep", "-r", "-n", "needle", "./internal"}))
			Expect(spec.ExitCode).To(BeZero())
			Expect(spec.ExitCodeAsserted).To(BeTrue())
		})

		It("writes and reads non-zero exit codes when present", func() {
			path := filepath.Join(GinkgoT().TempDir(), CommandFileName)

			Expect(WriteCommandWithExitCode(path, []string{"git", "show"}, 128)).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`exit_code: 128`))

			spec, err := ReadCommand(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Argv).To(Equal([]string{"git", "show"}))
			Expect(spec.ExitCode).To(Equal(128))
			Expect(spec.ExitCodeAsserted).To(BeTrue())
		})

		It("distinguishes an omitted exit assertion from an asserted zero", func() {
			path := filepath.Join(GinkgoT().TempDir(), CommandFileName)
			Expect(os.WriteFile(path, []byte("argv: [\"git\", \"status\"]\n"), 0o644)).To(Succeed())

			spec, err := ReadCommand(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec.ExitCode).To(BeZero())
			Expect(spec.ExitCodeAsserted).To(BeFalse())
		})

		DescribeTable("rejecting malformed command fixtures",
			func(body string, expected string) {
				path := filepath.Join(GinkgoT().TempDir(), CommandFileName)
				Expect(os.WriteFile(path, []byte(body), 0o644)).To(Succeed())

				_, err := ReadCommand(path)

				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("missing argv", "exit_code: 1\n", "argv is required"),
			Entry("blank argv entry", "argv: [\"\"]\n", "argv[0] must be non-empty"),
		)
	})

	Describe("sequenced streams", func() {
		DescribeTable("reading replay lines without normalizing carriage returns",
			func(input string, expectedLine string, expectedErr error) {
				line, err := readReplayLine(bufio.NewReader(strings.NewReader(input)))

				Expect(line).To(Equal(expectedLine))
				if expectedErr == nil {
					Expect(err).NotTo(HaveOccurred())
					return
				}
				Expect(err).To(MatchError(expectedErr))
			},
			Entry("preserves a trailing bare carriage return", "spinner\r", "spinner\r", nil),
			Entry("preserves all carriage-return redraw bytes", "spinner\rdone", "spinner\rdone", nil),
			Entry("returns EOF for an empty reader", "", "", io.EOF),
		)

		DescribeTable("preserving carriage-return redraws in sequenced records",
			func(raw string, expected []Event) {
				path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
				Expect(os.WriteFile(path, []byte(raw), 0o644)).To(Succeed())

				loaded, err := ReadSequenced(path, contracts.StreamStdout)
				Expect(err).NotTo(HaveOccurred())
				Expect(loaded).To(Equal(expected))
			},
			Entry("keeps the sequence prefix for git rebase redraw output",
				"00000|Rebasing (1/1)\r\r\x1b[KSuccessfully rebased and updated refs/heads/feature.\n",
				[]Event{{Sequence: 0, Stream: contracts.StreamStdout, Line: "Rebasing (1/1)\r\r\x1b[KSuccessfully rebased and updated refs/heads/feature.\n"}},
			),
			Entry("keeps the sequence prefix for spinner redraw output",
				"00004|\r⠋ [1/2] compiling calculator.js...\r⠋ [2/2] compiling legacy.js...\n",
				[]Event{{Sequence: 4, Stream: contracts.StreamStdout, Line: "\r⠋ [1/2] compiling calculator.js...\r⠋ [2/2] compiling legacy.js...\n"}},
			),
		)

		It("writes and reloads sequenced stream files", func() {
			path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
			events := []Event{
				{Sequence: 2, Stream: contracts.StreamStdout, Line: "three\n"},
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "one\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "two"},
			}

			Expect(WriteSequenced(path, events)).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("00000|one\n"))
			Expect(string(body)).To(ContainSubstring("00001|" + encodedPayloadPrefix))
			Expect(string(body)).To(ContainSubstring("00002|three\n"))

			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "one\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "two"},
				{Sequence: 2, Stream: contracts.StreamStdout, Line: "three\n"},
			}))
		})

		It("writes only the selected stream events", func() {
			path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
			events := []Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"},
				{Sequence: 1, Stream: contracts.StreamStderr, Line: "err-1\n"},
				{Sequence: 2, Stream: contracts.StreamStdout, Line: "out-2\n"},
			}

			Expect(WriteSequencedEvents(path, events, contracts.StreamStdout)).To(Succeed())

			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"},
				{Sequence: 2, Stream: contracts.StreamStdout, Line: "out-2\n"},
			}))
		})

		It("round-trips sequence numbers beyond five digits", func() {
			path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
			events := []Event{
				{Sequence: 99999, Stream: contracts.StreamStdout, Line: "boundary\n"},
				{Sequence: 100000, Stream: contracts.StreamStdout, Line: "overflow\n"},
			}

			Expect(WriteSequenced(path, events)).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("99999|boundary\n100000|overflow\n"))

			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal(events))
		})

		It("preserves CRLF sequenced fixtures exactly", func() {
			path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
			Expect(os.WriteFile(path, []byte("00000|one\r\n00001|two\r\n"), 0o644)).To(Succeed())

			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "one\r\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "two\r\n"},
			}))
		})

		It("round-trips invalid UTF-8 and reserved-prefix payloads", func() {
			path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
			events := []Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: string([]byte{0xff, 0xfe, 'x'})},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: encodedPayloadPrefix + "literal\n"},
			}

			Expect(WriteSequenced(path, events)).To(Succeed())
			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal(events))
		})

		It("treats missing sequenced files as absent events", func() {
			loaded, err := ReadSequenced(filepath.Join(GinkgoT().TempDir(), "missing.txt"), contracts.StreamStdout)

			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(BeNil())
		})

		It("reads sequenced events from readers", func() {
			loaded, err := ReadSequencedReader(strings.NewReader("00000|one\n00001|two\n"), contracts.StreamStdout)

			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "one\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "two\n"},
			}))
		})

		It("treats nil readers as absent events", func() {
			loaded, err := ReadSequencedReader(nil, contracts.StreamStdout)

			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(BeNil())
		})

		It("rejects sequenced lines with an empty prefix", func() {
			_, err := ReadSequencedReader(strings.NewReader("|line\n"), contracts.StreamStdout)

			Expect(err).To(MatchError(ContainSubstring(`invalid prefix "|line`)))
		})

		DescribeTable("validating merged stream sequences",
			func(stdout, stderr []Event, expected []Event, wantErr string) {
				merged, err := MergeAndValidate(stdout, stderr)
				if wantErr != "" {
					Expect(err).To(MatchError(ContainSubstring(wantErr)))
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(merged).To(Equal(expected))
			},
			Entry("interleaved stdout and stderr",
				[]Event{{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"}, {Sequence: 2, Stream: contracts.StreamStdout, Line: "out-2\n"}},
				[]Event{{Sequence: 1, Stream: contracts.StreamStderr, Line: "err-1\n"}},
				[]Event{
					{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"},
					{Sequence: 1, Stream: contracts.StreamStderr, Line: "err-1\n"},
					{Sequence: 2, Stream: contracts.StreamStdout, Line: "out-2\n"},
				},
				"",
			),
			Entry("missing sequence number",
				[]Event{{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"}},
				[]Event{{Sequence: 2, Stream: contracts.StreamStderr, Line: "err-2\n"}},
				nil,
				"expected 00001, got 00002",
			),
		)

		It("reads and merges sequenced event files", func() {
			dir := GinkgoT().TempDir()
			stdoutPath := filepath.Join(dir, StdoutFileName)
			stderrPath := filepath.Join(dir, StderrFileName)
			Expect(os.WriteFile(stdoutPath, []byte("00000|out-0\n"), 0o644)).To(Succeed())
			Expect(os.WriteFile(stderrPath, []byte("00001|err-1\n"), 0o644)).To(Succeed())

			events, err := ReadEvents(stdoutPath, stderrPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(events).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"},
				{Sequence: 1, Stream: contracts.StreamStderr, Line: "err-1\n"},
			}))
		})

		It("reads and merges sequenced event readers", func() {
			events, err := ReadEventReaders(
				strings.NewReader("00000|out-0\n"),
				strings.NewReader("00001|err-1\n"),
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(events).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-0\n"},
				{Sequence: 1, Stream: contracts.StreamStderr, Line: "err-1\n"},
			}))
		})
	})

	It("treats any of stdout, stderr, or output as a valid fixture footprint", func() {
		dir := GinkgoT().TempDir()
		Expect(HasRequiredFixtureFiles(dir)).To(BeFalse())

		Expect(os.WriteFile(filepath.Join(dir, OutputFileName), []byte(""), 0o644)).To(Succeed())
		Expect(HasRequiredFixtureFiles(dir)).To(BeTrue())

		Expect(os.Remove(filepath.Join(dir, OutputFileName))).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, StdoutFileName), []byte("00000|line\n"), 0o644)).To(Succeed())
		Expect(HasRequiredFixtureFiles(dir)).To(BeTrue())
	})

	It("preserves raw concatenation order for token counting", func() {
		events := []Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Line: "alpha\n"},
			{Sequence: 1, Stream: contracts.StreamStderr, Line: "beta\n"},
			{Sequence: 2, Stream: contracts.StreamStdout, Line: "gamma"},
		}

		Expect(strings.ReplaceAll(CombinedInput(events), "\r\n", "\n")).To(Equal("alpha\nbeta\ngamma"))
	})

	It("refuses to overwrite a symlinked command fixture", func() {
		tmp := GinkgoT().TempDir()
		target := filepath.Join(tmp, "outside-command.yaml")
		link := filepath.Join(tmp, CommandFileName)
		Expect(os.WriteFile(target, []byte("keep me"), 0o644)).To(Succeed())
		if err := os.Symlink(target, link); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := WriteCommandWithExitCode(link, []string{"git", "status"}, 0)

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(target)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me"))
	})

	It("refuses to write through a symlinked parent directory", func() {
		tmp := GinkgoT().TempDir()
		fixtureDir := filepath.Join(tmp, "fixture")
		outsideDir := filepath.Join(tmp, "outside")
		linkDir := filepath.Join(fixtureDir, "stdout")
		outsidePath := filepath.Join(outsideDir, StdoutFileName)
		artifactPath := filepath.Join(linkDir, StdoutFileName)

		Expect(os.MkdirAll(fixtureDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(outsideDir, 0o755)).To(Succeed())
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		err := WriteArtifact(artifactPath, []byte("should-not-escape"), 0o644)

		Expect(err).To(HaveOccurred())
		_, statErr := os.Stat(outsidePath)
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})

	Describe("LoadFixture", func() {
		It("loads fixture metadata and resolves absolute paths", func() {
			dir := GinkgoT().TempDir()
			Expect(WriteCommandWithExitCode(filepath.Join(dir, CommandFileName), []string{"git", "status"}, 7)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, OutputFileName), []byte("output"), 0o644)).To(Succeed())

			fixture, err := LoadFixture(dir)

			Expect(err).NotTo(HaveOccurred())
			Expect(fixture.Dir).To(Equal(dir))
			Expect(fixture.Command.Argv).To(Equal([]string{"git", "status"}))
			Expect(fixture.Command.ExitCode).To(Equal(7))
			Expect(fixture.CommandPath).To(Equal(filepath.Join(dir, CommandFileName)))
			Expect(fixture.OutputPath).To(Equal(filepath.Join(dir, OutputFileName)))
		})

		It("rejects fixture directories without replay footprint files", func() {
			dir := GinkgoT().TempDir()
			Expect(WriteCommandWithExitCode(filepath.Join(dir, CommandFileName), []string{"git", "status"}, 0)).To(Succeed())

			_, err := LoadFixture(dir)

			Expect(err).To(MatchError(ContainSubstring("must contain at least one of")))
		})
	})
})
