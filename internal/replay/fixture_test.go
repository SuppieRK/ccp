package replay

import (
	"os"
	"path/filepath"
	"strings"

	"go-command-compression-proxy/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("replay fixtures", func() {
	Describe("command yaml", func() {
		It("writes and reads flow-style argv", func() {
			path := filepath.Join(GinkgoT().TempDir(), CommandFileName)

			Expect(WriteCommand(path, []string{"grep", "-r", "-n", "needle", "./internal"})).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`argv: ["grep", "-r", "-n", "needle", "./internal"]`))

			spec, err := ReadCommand(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Argv).To(Equal([]string{"grep", "-r", "-n", "needle", "./internal"}))
			Expect(spec.ExitCode).To(BeZero())
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
		})
	})

	Describe("sequenced streams", func() {
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
			Expect(string(body)).To(Equal("00000|one\n00001|two\n00002|three\n"))

			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "one\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "two\n"},
				{Sequence: 2, Stream: contracts.StreamStdout, Line: "three\n"},
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

		It("normalizes CRLF sequenced fixtures without leaking stray carriage returns", func() {
			path := filepath.Join(GinkgoT().TempDir(), StdoutFileName)
			Expect(os.WriteFile(path, []byte("00000|one\r\n00001|two\r\n"), 0o644)).To(Succeed())

			loaded, err := ReadSequenced(path, contracts.StreamStdout)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded).To(Equal([]Event{
				{Sequence: 0, Stream: contracts.StreamStdout, Line: "one\n"},
				{Sequence: 1, Stream: contracts.StreamStdout, Line: "two\n"},
			}))
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

		err := WriteCommand(link, []string{"git", "status"})

		Expect(err).To(HaveOccurred())
		body, readErr := os.ReadFile(target)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("keep me"))
	})
})
