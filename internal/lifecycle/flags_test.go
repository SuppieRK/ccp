package lifecycle

import (
	"bytes"
	"flag"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("lifecycle flag helpers", func() {
	Describe("newLifecycleFlagSet", func() {
		It("writes flag defaults to stderr", func() {
			originalStderr := os.Stderr
			reader, writer, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			os.Stderr = writer
			DeferCleanup(func() {
				os.Stderr = originalStderr
			})
			DeferCleanup(func() { _ = reader.Close() })
			DeferCleanup(func() { _ = writer.Close() })

			fs := newLifecycleFlagSet("demo")
			fs.Bool("verbose", false, "emit verbose output")

			fs.PrintDefaults()
			Expect(writer.Close()).To(Succeed())

			body, readErr := io.ReadAll(reader)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("-verbose"))
			Expect(string(body)).To(ContainSubstring("emit verbose output"))
		})
	})

	Describe("parseLifecycleFlags", func() {
		It("treats help requests as handled without returning an error", func() {
			fs := newLifecycleFlagSet("demo")

			helpRequested, err := parseLifecycleFlags(fs, []string{"--help"})

			Expect(err).NotTo(HaveOccurred())
			Expect(helpRequested).To(BeTrue())
		})

		It("preserves non-help parse errors", func() {
			fs := newLifecycleFlagSet("demo")

			helpRequested, err := parseLifecycleFlags(fs, []string{"--unknown"})

			Expect(err).To(HaveOccurred())
			Expect(helpRequested).To(BeFalse())
			Expect(err).NotTo(MatchError(flag.ErrHelp))
		})

		It("returns false when parsing succeeds without a help flag", func() {
			fs := newLifecycleFlagSet("demo")
			fs.Bool("json", false, "render json")

			helpRequested, err := parseLifecycleFlags(fs, []string{"--json"})

			Expect(err).NotTo(HaveOccurred())
			Expect(helpRequested).To(BeFalse())
			Expect(fs.Lookup("json").Value.String()).To(Equal("true"))
		})

		It("does not treat args after -- as lifecycle help", func() {
			fs := newLifecycleFlagSet("capture")

			helpRequested, err := parseLifecycleFlags(fs, []string{"--", "--help"})

			Expect(err).NotTo(HaveOccurred())
			Expect(helpRequested).To(BeFalse())
			Expect(fs.Args()).To(Equal([]string{"--help"}))
		})
	})

	Describe("setLifecycleUsage", func() {
		It("renders usage, flags, and notes sections", func() {
			fs := newLifecycleFlagSet("gain")
			var output bytes.Buffer
			fs.SetOutput(&output)
			fs.Bool("json", false, "render json output")

			setLifecycleUsage(fs, "show token savings history", []string{"ccp gain [flags]"}, "note one", "note two")

			fs.Usage()

			text := output.String()
			Expect(text).To(ContainSubstring("ccp gain - show token savings history"))
			Expect(text).To(ContainSubstring("Usage:"))
			Expect(text).To(ContainSubstring("  ccp gain [flags]"))
			Expect(text).To(ContainSubstring("Flags:"))
			Expect(text).To(ContainSubstring("-json"))
			Expect(text).To(ContainSubstring("render json output"))
			Expect(text).To(ContainSubstring("Notes:"))
			Expect(text).To(ContainSubstring("  - note one"))
			Expect(text).To(ContainSubstring("  - note two"))
		})

		It("omits the notes section when no notes are provided", func() {
			fs := newLifecycleFlagSet("history")
			var output bytes.Buffer
			fs.SetOutput(&output)
			fs.String("format", "text", "output format")

			setLifecycleUsage(fs, "show recorded command history", []string{"ccp history [flags]"})

			fs.Usage()

			text := output.String()
			Expect(text).To(ContainSubstring("ccp history - show recorded command history"))
			Expect(text).To(ContainSubstring("Usage:"))
			Expect(text).To(ContainSubstring("Flags:"))
			Expect(text).NotTo(ContainSubstring("Notes:"))
		})
	})

	DescribeTable("detecting lifecycle help flags",
		func(args []string, expected bool) {
			Expect(lifecycleHelpRequested(args)).To(Equal(expected))
		},
		Entry("detects short help", []string{"capture", "-h"}, true),
		Entry("detects long help", []string{"capture", "--help"}, true),
		Entry("stops at command separator", []string{"capture", "--", "--help"}, false),
		Entry("ignores non-help args", []string{"capture", "--json"}, false),
	)
})
