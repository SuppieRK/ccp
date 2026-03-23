package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corefilters "go-command-compression-proxy/internal/filters"
	filteryaml "go-command-compression-proxy/internal/filters/yaml"
	"go-command-compression-proxy/internal/metrics"
)

var _ = Describe("nested and chained ccp execution", Ordered, func() {
	var (
		binDir  string
		binPath string
	)

	BeforeAll(func() {
		if runtime.GOOS == "windows" {
			Skip("nested shell execution coverage is unix-oriented")
		}

		tmp := GinkgoT().TempDir()
		binDir = filepath.Join(tmp, "bin")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())

		binPath = filepath.Join(binDir, "ccp")
		build := exec.Command("go", "build", "-o", binPath, "./cmd/ccp")
		build.Dir = filteryaml.ProjectRootFromSource()
		build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(tmp, ".gocache"))
		out, err := build.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
	})

	newWorkspace := func() string {
		root := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, ".git"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(root, "src"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "src", "alpha.txt"), []byte("alpha v2\nalpha done\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "src", "beta.txt"), []byte("beta v2\nbeta v2 again\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".git", "ignored.txt"), []byte("ignored v2\n"), 0o644)).To(Succeed())
		return root
	}

	runCCP := func(workdir string, args ...string) (string, string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = workdir
		cmd.Env = append(
			os.Environ(),
			"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"HOME="+workdir,
		)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	expectSuccessfulRun := func(workdir string, args ...string) string {
		stdout, stderr, err := runCCP(workdir, args...)
		debug := fmt.Sprintf("command: %s\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), stdout, stderr)
		Expect(err).NotTo(HaveOccurred(), debug)
		Expect(stderr).To(BeEmpty(), debug)
		return stdout
	}

	Context("when nested ccp invocations are composed through shell fanout", func() {
		DescribeTable("handles nested ccp grep fanout without hanging",
			func(args []string, expectedSubstrings []string) {
				workdir := newWorkspace()

				stdout := expectSuccessfulRun(workdir, args...)

				for _, expected := range expectedSubstrings {
					Expect(stdout).To(ContainSubstring(expected))
				}
				Expect(stdout).NotTo(ContainSubstring("ignored.txt"))
			},
			Entry("via find -exec fanout",
				[]string{"find", ".", "-type", "f", "-not", "-path", "*/.git/*", "-exec", "ccp", "grep", "-nH", "--", "v2", "{}", "+"},
				[]string{"./src/alpha.txt:\n  1: alpha v2", "./src/beta.txt:\n  1: beta v2\n  2: beta v2 again"},
			),
			Entry("via find and xargs pipelines",
				[]string{"bash", "-lc", `find . -type f -not -path '*/.git/*' -print0 | ccp xargs -0 -r ccp grep -nH -- 'v2'`},
				[]string{"./src/alpha.txt:\n  1: alpha v2", "./src/beta.txt:\n  1: beta v2\n  2: beta v2 again"},
			),
			Entry("via chained shell pipelines",
				[]string{"bash", "-lc", `find . -type f -not -path '*/.git/*' -exec ccp grep -nH -- 'v2' {} + | tail -20`},
				[]string{"./src/alpha.txt:", "./src/beta.txt:"},
			),
		)
	})

	Context("when bounded shell pipelines are recorded", func() {
		It("records go-build-like grep-v shell pipelines as one bounded top-level metric", func() {
			workdir := newWorkspace()
			script := `for i in $(seq 1 200); do
  printf '> Task :travels:app:compileJava noisy-%03d\n' "$i" >&2
done
printf 'internal/parser.go:12:2: undefined: missingSymbol\n' >&2
printf '. daemon detail that should be filtered\n' >&2
printf -- '- compiler detail that should be filtered\n' >&2
printf 'internal/runner.go:44:7: undefined: otherSymbol\n' >&2
printf 'internal/metrics/store.go:90:1: too many errors\n' >&2`

			stdout := expectSuccessfulRun(
				workdir,
				"bash", "-lc",
				"("+script+`) 2>&1 | grep -v "^>" | grep -v "^\\." | grep -v "^-" | tail -2`,
			)

			Expect(stdout).To(Equal("internal/runner.go:44:7: undefined: otherSymbol\ninternal/metrics/store.go:90:1: too many errors\n"))

			history, err := metrics.QueryHistory(filepath.Join(workdir, ".ccp", "gain.db"), metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("bash"))
			Expect(history[0].Command).To(ContainSubstring(`grep -v "^>"`))
			Expect(history[0].Command).To(ContainSubstring(`grep -v "^\\."`))
			Expect(history[0].Command).To(ContainSubstring(`grep -v "^-"`))
			Expect(history[0].Command).To(ContainSubstring("tail -2"))
			Expect(history[0].RawBytes).To(BeNumerically("<", 1024))
			Expect(history[0].KeptBytes).To(BeNumerically("<", 1024))
			Expect(history[0].EstimatedInputTokens).To(BeNumerically("<", 256))
			Expect(history[0].EstimatedOutputTokens).To(BeNumerically("<", 256))
			Expect(history[0].Passthrough).To(BeTrue())
		})
	})
})

var _ = Describe("runner execution edge cases", func() {
	var (
		stdoutReader *os.File
		stdoutWriter *os.File
		stderrReader *os.File
		stderrWriter *os.File
		oldStdout    *os.File
		oldStderr    *os.File
	)

	BeforeEach(func() {
		var err error
		oldStdout = os.Stdout
		oldStderr = os.Stderr

		stdoutReader, stdoutWriter, err = os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		stderrReader, stderrWriter, err = os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		os.Stdout = stdoutWriter
		os.Stderr = stderrWriter
	})

	AfterEach(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
	})

	Context("when execution fails before command output is processed", func() {
		It("returns registry errors from invalid filter sources before execution", func() {
			sourceFile := filepath.Join(GinkgoT().TempDir(), "not-a-dir")
			Expect(os.WriteFile(sourceFile, []byte("x"), 0o644)).To(Succeed())

			runner := &Runner{sources: []corefilters.FilterSource{{Directory: sourceFile}}}

			code, err := runner.Run([]string{"git", "status"})

			Expect(code).To(Equal(1))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not a directory"))
		})

		It("returns raw-mode start failures with shell-not-found exit semantics", func() {
			runner := NewRunnerWithOptions(Options{Raw: true})

			code, err := runner.Run([]string{"__ccp_missing_binary__"})

			Expect(code).To(Equal(127))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when downstream writes fail after execution starts", func() {
		DescribeTable("surfaces write failures while preserving non-zero exit codes",
			func(makeRunner func() *Runner, command []string) {
				if runtime.GOOS == "windows" {
					Skip("uses unix sh")
				}

				brokenStdout, err := os.CreateTemp("", "core-runner-broken-stdout-nonzero-*")
				Expect(err).NotTo(HaveOccurred())
				brokenStdoutPath := brokenStdout.Name()
				DeferCleanup(func() {
					Expect(os.Remove(brokenStdoutPath)).To(Succeed())
				})
				Expect(brokenStdout.Close()).To(Succeed())
				os.Stdout = brokenStdout

				runner := makeRunner()
				code, err := runner.Run(command)

				Expect(err).To(HaveOccurred())
				Expect(code).To(Equal(7))
			},
			Entry("for filtered stdout writes",
				func() *Runner { return &Runner{sources: []corefilters.FilterSource{}} },
				[]string{"sh", "-c", "printf 'hello from ccp\\n'; exit 7"},
			),
			Entry("for raw-mode writes",
				func() *Runner { return NewRunnerWithOptions(Options{Raw: true}) },
				[]string{"sh", "-c", "printf 'raw-fail'; exit 7"},
			),
		)
	})

	Context("when raw mode preserves native exit behavior", func() {
		It("returns non-zero exit codes from raw mode without wrapping them as errors", func() {
			if runtime.GOOS == "windows" {
				Skip("uses unix sh")
			}

			runner := NewRunnerWithOptions(Options{Raw: true})

			code, err := runner.Run([]string{"sh", "-c", "printf 'raw-nonzero\\n'; exit 7"})

			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(7))
			Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("raw-nonzero\n"))
			Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
		})
	})
})
