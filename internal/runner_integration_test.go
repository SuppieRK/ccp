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

	It("handles find -exec ccp grep fanout without hanging", func() {
		workdir := newWorkspace()

		stdout := expectSuccessfulRun(
			workdir,
			"find", ".", "-type", "f", "-not", "-path", "*/.git/*",
			"-exec", "ccp", "grep", "-nH", "--", "v2", "{}", "+",
		)

		Expect(stdout).To(ContainSubstring("./src/alpha.txt:\n  1: alpha v2"))
		Expect(stdout).To(ContainSubstring("./src/beta.txt:\n  1: beta v2\n  2: beta v2 again"))
		Expect(stdout).NotTo(ContainSubstring("ignored.txt"))
	})

	It("handles find | ccp xargs | ccp grep pipelines", func() {
		workdir := newWorkspace()

		stdout := expectSuccessfulRun(
			workdir,
			"bash", "-lc",
			`find . -type f -not -path '*/.git/*' -print0 | ccp xargs -0 -r ccp grep -nH -- 'v2'`,
		)

		Expect(stdout).To(ContainSubstring("./src/alpha.txt:\n  1: alpha v2"))
		Expect(stdout).To(ContainSubstring("./src/beta.txt:\n  1: beta v2\n  2: beta v2 again"))
		Expect(stdout).NotTo(ContainSubstring("ignored.txt"))
	})

	It("handles chained shell pipelines around nested ccp invocations", func() {
		workdir := newWorkspace()

		stdout := expectSuccessfulRun(
			workdir,
			"bash", "-lc",
			`find . -type f -not -path '*/.git/*' -exec ccp grep -nH -- 'v2' {} + | tail -20`,
		)

		Expect(stdout).To(ContainSubstring("./src/alpha.txt:"))
		Expect(stdout).To(ContainSubstring("./src/beta.txt:"))
		Expect(stdout).NotTo(ContainSubstring("ignored.txt"))
	})

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
