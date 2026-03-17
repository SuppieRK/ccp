package core

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go-command-compression-proxy/internal/audit"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/engine"
	corefilters "go-command-compression-proxy/internal/filters"
	filteryaml "go-command-compression-proxy/internal/filters/yaml"
	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/version"
)

var _ = Describe("Runner", func() {
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

	It("uses default filter sources when none are provided", func() {
		runner := NewRunner()
		Expect(runner).NotTo(BeNil())
		Expect(runner.sources).To(Equal(defaultFilterSources()))
	})

	It("uses repository filters by default in dev builds", func() {
		oldVersion := version.Version
		version.Version = "dev"
		DeferCleanup(func() {
			version.Version = oldVersion
		})

		sources := defaultFilterSources()

		Expect(sources).To(HaveLen(1))
		Expect(sources[0]).To(Equal(corefilters.RepositorySource(filteryaml.ProjectRootFromSource())))
	})

	It("uses project and home filters by default in non-dev builds", func() {
		oldVersion := version.Version
		version.Version = "v1.2.3"
		DeferCleanup(func() {
			version.Version = oldVersion
		})

		sources := defaultFilterSources()

		Expect(sources).To(HaveLen(2))
		Expect(sources[0].Kind).To(Equal(corefilters.SourceProject))
		Expect(sources[0].Directory).To(HaveSuffix(filepath.Join(".ccp", "filters")))
		Expect(sources[1].Kind).To(Equal(corefilters.SourceHome))
		Expect(sources[1].Directory).To(HaveSuffix(filepath.Join(".config", "ccp", "filters")))
	})

	It("fails when no command is provided", func() {
		runner := &Runner{sources: []corefilters.FilterSource{}}

		code, err := runner.Run(nil)

		Expect(code).To(Equal(2))
		Expect(err).To(MatchError("no command provided"))
	})

	It("returns command start errors with shell-not-found exit semantics", func() {
		runner := &Runner{sources: []corefilters.FilterSource{}}

		code, err := runner.Run([]string{"__ccp_missing_binary__"})

		Expect(code).To(Equal(127))
		Expect(err).To(HaveOccurred())
	})

	It("routes direct emitted entries to the correct streams", func() {
		runner := &Runner{sources: []corefilters.FilterSource{}}

		runner.writeEntries([]engine.BufferEntry{
			{Stream: contracts.StreamStdout, Line: "out-1\n"},
			{Stream: contracts.StreamStderr, Line: "err-1\n"},
			{Stream: contracts.StreamStdout, Line: "out-2\n"},
		})

		Expect(closeAndRead(stdoutReader, stdoutWriter)).To(Equal("out-1\nout-2\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(Equal("err-1\n"))
	})

	It("copies a trailing line before stopping on a non-EOF read error", func() {
		runner := &Runner{sources: []corefilters.FilterSource{}}
		src := &errorAfterLineReader{
			line: "tail-without-newline",
			err:  errors.New("boom"),
		}
		stats := &streamStats{}

		runner.copyStream(src, func(line string) []engine.BufferEntry {
			return []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: line}}
		}, stats)

		Expect(closeAndRead(stdoutReader, stdoutWriter)).To(Equal("tail-without-newline"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
		Expect(stats.rawBytes).To(Equal(len("tail-without-newline")))
		Expect(stats.keptBytes).To(Equal(len("tail-without-newline")))
	})

	It("treats carriage returns as in-place line overwrites before emit", func() {
		runner := &Runner{sources: []corefilters.FilterSource{}}
		src := strings.NewReader("\r⠋ first\r⠙ second\rDone\n")
		stats := &streamStats{}

		runner.copyStream(src, func(line string) []engine.BufferEntry {
			return []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: line}}
		}, stats)

		Expect(closeAndRead(stdoutReader, stdoutWriter)).To(Equal("Done\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
		Expect(stats.rawBytes).To(Equal(len("\r⠋ first\r⠙ second\rDone\n")))
		Expect(stats.keptBytes).To(Equal(len("Done\n")))
	})

	It("preserves ordinary CRLF line endings", func() {
		runner := &Runner{sources: []corefilters.FilterSource{}}
		src := strings.NewReader("runner-win\r\n")
		stats := &streamStats{}

		runner.copyStream(src, func(line string) []engine.BufferEntry {
			return []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: line}}
		}, stats)

		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("runner-win\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
		Expect(stats.rawBytes).To(Equal(len("runner-win\r\n")))
		Expect(stats.keptBytes).To(Equal(len("runner-win\n")))
	})

	It("executes a real command and preserves piped stdin on unix", func() {
		if runtime.GOOS == "windows" {
			Skip("unix stdin integration uses cat")
		}

		runner := &Runner{sources: []corefilters.FilterSource{}}
		oldStdin := os.Stdin
		stdinReader, stdinWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		_, err = io.WriteString(stdinWriter, "alpha\nbeta")
		Expect(err).NotTo(HaveOccurred())
		Expect(stdinWriter.Close()).To(Succeed())
		os.Stdin = stdinReader
		defer func() {
			os.Stdin = oldStdin
			_ = stdinReader.Close()
		}()

		code, err := runner.Run([]string{"cat"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("alpha\nbeta"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
	})

	It("executes a real command on windows", func() {
		if runtime.GOOS != "windows" {
			Skip("windows-specific command")
		}

		runner := &Runner{sources: []corefilters.FilterSource{}}

		code, err := runner.Run([]string{"cmd", "/c", "echo runner-win"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(ContainSubstring("runner-win\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
	})

	It("preserves stderr output and non-zero exit codes from a real command", func() {
		if runtime.GOOS == "windows" {
			Skip("unix shell integration uses sh")
		}

		runner := &Runner{sources: []corefilters.FilterSource{}}

		code, err := runner.Run(
			[]string{"sh", "-c", "printf 'out\\n'; printf 'err\\n' >&2; exit 7"},
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(7))
		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("out\n"))
		Expect(normalizeNL(closeAndRead(stderrReader, stderrWriter))).To(Equal("err\n"))
	})

	It("writes gain metrics using the resolved filter dispatch key", func() {
		tmpDir, err := os.MkdirTemp("", "core-runner-metrics-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		runner := &Runner{
			sources:     []corefilters.FilterSource{},
			metricsPath: filepath.Join(tmpDir, ".ccp", "gain.db"),
		}

		args, expectedTool := metricsCommand()
		code, err := runner.Run(args)

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		_ = closeAndRead(stdoutReader, stdoutWriter)
		_ = closeAndRead(stderrReader, stderrWriter)

		history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(history[0].Tool).To(Equal(expectedTool))
		Expect(history[0].DispatchKey).To(Equal(expectedTool))
		Expect(history[0].RawBytes).To(BeNumerically(">", 0))
		Expect(history[0].KeptBytes).To(BeNumerically(">", 0))
		Expect(history[0].Passthrough).To(BeTrue())
	})

	It("writes execution and fallback audit events", func() {
		tmpDir, err := os.MkdirTemp("", "core-runner-audit-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		restoreAudit := audit.WithTestConfig(tmpDir, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		runner := &Runner{sources: []corefilters.FilterSource{}}

		code, err := runner.Run(auditCommand())

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(closeAndRead(stdoutReader, stdoutWriter)).To(ContainSubstring("audit-ok"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())

		auditData, err := os.ReadFile(filepath.Join(tmpDir, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"execution_start"`))
		Expect(string(auditData)).To(ContainSubstring(`"msg":"filter_fallback"`))
		Expect(string(auditData)).To(ContainSubstring(`"msg":"execution_finish"`))
	})

	It("records nested and chained execution shape diagnostics", func() {
		tmpDir, err := os.MkdirTemp("", "core-runner-audit-shape-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		restoreAudit := audit.WithTestConfig(tmpDir, 8, 7)
		DeferCleanup(restoreAudit)
		DeferCleanup(audit.Reset)

		runner := &Runner{sources: []corefilters.FilterSource{}}

		code, err := runner.Run([]string{"bash", "-lc", "printf audit-shape | ccp xargs -0 -r ccp echo hi || true"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		_ = closeAndRead(stdoutReader, stdoutWriter)
		_ = closeAndRead(stderrReader, stderrWriter)

		auditData, err := os.ReadFile(filepath.Join(tmpDir, ".config", "ccp", "audit", "audit.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(auditData)).To(ContainSubstring(`"msg":"execution_start"`))
		Expect(string(auditData)).To(ContainSubstring(`"uses_shell":true`))
		Expect(string(auditData)).To(ContainSubstring(`"has_pipeline":true`))
		Expect(string(auditData)).To(ContainSubstring(`"has_chain":true`))
		Expect(string(auditData)).To(ContainSubstring(`"has_xargs":true`))
		Expect(string(auditData)).To(ContainSubstring(`"nested_ccp":true`))
	})

	It("does not record wrapped ccp lifecycle invocations in gain metrics", func() {
		tmpDir, err := os.MkdirTemp("", "core-runner-self-metrics-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		runner := &Runner{
			sources:     []corefilters.FilterSource{},
			metricsPath: filepath.Join(tmpDir, ".ccp", "gain.db"),
		}

		command := contracts.Command{
			RawInput: "ccp history",
			Args:     []string{"ccp", "history"},
			Tool:     "ccp",
			Dispatch: "ccp",
		}

		runner.appendMetrics(command, true, 0, 1, 32, 32)

		history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(BeEmpty())
	})

	It("does not record wrapped ccp capture invocations in gain metrics", func() {
		tmpDir, err := os.MkdirTemp("", "core-runner-ccp-metrics-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		runner := &Runner{
			sources:     []corefilters.FilterSource{},
			metricsPath: filepath.Join(tmpDir, ".ccp", "gain.db"),
		}

		command := contracts.Command{
			RawInput: "ccp capture -- echo hi",
			Args:     []string{"ccp", "capture", "--", "echo", "hi"},
			Tool:     "ccp",
			Dispatch: "ccp",
		}

		runner.appendMetrics(command, true, 0, 1, 32, 32)

		history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(BeEmpty())
	})

	It("creates subprocess pipes with stdin attached to os.Stdin", func() {
		name, args := successCommand()

		cmd, stdout, stderr, err := commandWithPipes(name, args)

		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Stdin).To(Equal(os.Stdin))
		Expect(stdout).NotTo(BeNil())
		Expect(stderr).NotTo(BeNil())
		closePipes(stdout, stderr)
	})

	It("returns code 1 for wait errors that are not process exit errors", func() {
		name, args := successCommand()
		cmd := exec.Command(name, args...)

		code, err := waitExitCode(cmd)

		Expect(code).To(Equal(1))
		Expect(err).To(HaveOccurred())
	})

	It("loads filters from configured sources before execution", func() {
		repoRoot, err := os.MkdirTemp("", "core-runner-sources-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})
		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: pytest
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		runner := &Runner{sources: []corefilters.FilterSource{
			corefilters.RepositorySource(repoRoot),
		}}
		registry, err := runner.loadRegistry()
		Expect(err).NotTo(HaveOccurred())

		resolved := registry.Resolve(contracts.Command{
			Tool: "python",
			Args: []string{"python", "-m", "pytest"},
		})
		Expect(resolved).NotTo(Equal(corefilters.Passthrough{}))
	})

	It("replays captured streams through the current YAML discovery path", func() {
		repoRoot, err := os.MkdirTemp("", "core-runner-verify-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})
		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "pytest.yaml"), []byte(`
version: 1
filter: pytest
about: test
cases:
  - id: summary
    compress_output:
      combined:
        lines:
          replace:
            - regex: '^=+ ([0-9]+) passed in .+$'
              to: 'pytest: $1 passed'
`), 0o644)).To(Succeed())

		runner := &Runner{sources: []corefilters.FilterSource{
			corefilters.RepositorySource(repoRoot),
		}}

		actual, err := runner.Verify(
			[]string{"pytest"},
			strings.NewReader("===== 2 passed in 0.12s =====\n"),
			nil,
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal("pytest: 2 passed\n"))
	})

	It("loads source-local mappings before registry resolution", func() {
		repoRoot, err := os.MkdirTemp("", "core-runner-mappings-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})
		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte(`
version: 1
map:
  py: python
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: pytest
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		runner := &Runner{sources: []corefilters.FilterSource{
			corefilters.RepositorySource(repoRoot),
		}}
		registry, err := runner.loadRegistry()
		Expect(err).NotTo(HaveOccurred())

		resolved := registry.Resolve(contracts.Command{
			Tool: "py",
			Args: []string{"py", "-m", "pytest"},
		})
		Expect(resolved).NotTo(Equal(corefilters.Passthrough{}))
	})

	It("falls back to direct lookup when source-local mappings are invalid", func() {
		repoRoot, err := os.MkdirTemp("", "core-runner-invalid-mappings-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})
		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte("version: oops\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: pytest
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		runner := &Runner{sources: []corefilters.FilterSource{
			corefilters.RepositorySource(repoRoot),
		}}
		registry, err := runner.loadRegistry()
		Expect(err).NotTo(HaveOccurred())

		resolved := registry.Resolve(contracts.Command{
			Tool: "python",
			Args: []string{"python", "-m", "pytest"},
		})
		Expect(resolved).NotTo(Equal(corefilters.Passthrough{}))
	})

	It("prefers project filters over home filters for the same identity", func() {
		root, err := os.MkdirTemp("", "core-runner-project-home-direct-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(root)).To(Succeed())
		})

		projectRoot := filepath.Join(root, "project")
		homeRoot := filepath.Join(root, "home")
		projectDir := filepath.Join(projectRoot, ".ccp", "filters")
		homeDir := filepath.Join(homeRoot, ".ccp", "filters")
		Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(homeDir, 0o755)).To(Succeed())

		Expect(os.WriteFile(filepath.Join(projectDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: project
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: home
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		runner := &Runner{sources: []corefilters.FilterSource{
			corefilters.ProjectSource(projectRoot),
			corefilters.HomeSource(homeRoot),
		}}
		registry, err := runner.loadRegistry()
		Expect(err).NotTo(HaveOccurred())

		resolved := registry.Resolve(contracts.Command{
			Tool: "python",
			Args: []string{"python", "-m", "pytest"},
		})
		Expect(resolved.Dispatch(contracts.Command{
			Tool: "python",
			Args: []string{"python", "-m", "pytest"},
		})).To(Equal("python|project"))
	})

	It("prefers project mappings over home mappings for the same alias", func() {
		root, err := os.MkdirTemp("", "core-runner-project-home-mapped-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(root)).To(Succeed())
		})

		projectRoot := filepath.Join(root, "project")
		homeRoot := filepath.Join(root, "home")
		projectDir := filepath.Join(projectRoot, ".ccp", "filters")
		homeDir := filepath.Join(homeRoot, ".ccp", "filters")
		Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(homeDir, 0o755)).To(Succeed())

		Expect(os.WriteFile(filepath.Join(projectDir, ".mappings.yaml"), []byte(`
version: 1
map:
  py: python
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, ".mappings.yaml"), []byte(`
version: 1
map:
  py: python
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(projectDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: project
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(homeDir, "python.yaml"), []byte(`
version: 1
filter: python
cases:
  - id: home
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		runner := &Runner{sources: []corefilters.FilterSource{
			corefilters.ProjectSource(projectRoot),
			corefilters.HomeSource(homeRoot),
		}}
		registry, err := runner.loadRegistry()
		Expect(err).NotTo(HaveOccurred())

		resolved := registry.Resolve(contracts.Command{
			Tool: "py",
			Args: []string{"py", "-m", "pytest"},
		})
		Expect(resolved.Dispatch(contracts.Command{
			Tool: "py",
			Args: []string{"py", "-m", "pytest"},
		})).To(Equal("python|project"))
	})

	It("applies YAML-authored command mutations before execution", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix printf")
		}

		repoRoot, err := os.MkdirTemp("", "core-runner-command-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})

		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "printf.yaml"), []byte(`
version: 1
filter: printf
cases:
  - id: default
    normalize_command:
      append_if_missing: ["mutated\n"]
    compress_output:
      combined:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

		runner := &Runner{
			sources: []corefilters.FilterSource{
				corefilters.RepositorySource(repoRoot),
			},
			metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
		}

		code, err := runner.Run([]string{"printf"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(closeAndRead(stdoutReader, stdoutWriter)).To(Equal("mutated\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
		history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(history[0].DispatchKey).To(Equal("printf|default"))
	})

	It("falls back to passthrough execution when a mapped YAML scaffold is invalid", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix cat")
		}

		repoRoot, err := os.MkdirTemp("", "core-runner-invalid-scaffold-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})

		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte(`
version: 1
map:
  cat: cat
`), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "cat.yaml"), []byte(`
version: 1
filter: cat
about: Placeholder cat filter scaffold for YAML migration.
`), 0o644)).To(Succeed())

		runner := &Runner{
			sources: []corefilters.FilterSource{
				corefilters.RepositorySource(repoRoot),
			},
			metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
		}

		oldStdin := os.Stdin
		stdinReader, stdinWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		_, err = io.WriteString(stdinWriter, "fallback\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(stdinWriter.Close()).To(Succeed())
		os.Stdin = stdinReader
		DeferCleanup(func() {
			os.Stdin = oldStdin
			_ = stdinReader.Close()
		})

		code, err := runner.Run([]string{"cat"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("fallback\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())

		history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(HaveLen(1))
		Expect(history[0].Tool).To(Equal("cat"))
		Expect(history[0].DispatchKey).To(Equal("cat"))
		Expect(history[0].Passthrough).To(BeTrue())
	})

	It("flushes retained stderr-only YAML output on exit", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		repoRoot, err := os.MkdirTemp("", "core-runner-stderr-keep-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(repoRoot)).To(Succeed())
		})

		filterDir := filepath.Join(repoRoot, "filters")
		Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filterDir, "sh.yaml"), []byte(`
version: 1
filter: sh
cases:
  - id: stderr_keep
    compress_output:
      stderr:
        lines:
          keep:
            - contains: "TS2367"
            - contains: "error:"
`), 0o644)).To(Succeed())

		runner := &Runner{
			sources: []corefilters.FilterSource{
				corefilters.RepositorySource(repoRoot),
			},
			metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
		}

		code, err := runner.Run([]string{"sh", "-c", "printf 'TS2367 [ERROR]: boom\\nerror: fail\\n' >&2; exit 1"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(1))
		Expect(closeAndRead(stdoutReader, stdoutWriter)).To(BeEmpty())
		Expect(normalizeNL(closeAndRead(stderrReader, stderrWriter))).To(Equal("TS2367 [ERROR]: boom\nerror: fail\n"))
	})

	It("bypasses filtering in raw mode", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		runner := NewRunnerWithOptions(Options{Raw: true})

		code, err := runner.Run([]string{"sh", "-c", "printf 'same\\nsame\\n'"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("same\nsame\n"))
		Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
	})

	It("redacts confidential tokens from filtered output", func() {
		if runtime.GOOS == "windows" {
			Skip("uses unix sh")
		}

		runner := NewRunnerWithOptions(Options{Confidential: []string{"hello"}})

		code, err := runner.Run([]string{"sh", "-c", "printf 'hello-stdout\\n'; printf 'hello-stderr\\n' >&2"})

		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(0))
		Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("***-stdout\n"))
		Expect(normalizeNL(closeAndRead(stderrReader, stderrWriter))).To(Equal("***-stderr\n"))
	})

})

func closeAndRead(reader, writer *os.File) string {
	_ = writer.Close()
	data, err := io.ReadAll(reader)
	Expect(err).NotTo(HaveOccurred())
	return string(data)
}

func normalizeNL(v string) string {
	return strings.ReplaceAll(v, "\r\n", "\n")
}

type errorAfterLineReader struct {
	line string
	err  error
	done bool
}

func (r *errorAfterLineReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	n := copy(p, r.line)
	return n, r.err
}

func successCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "exit", "0"}
	}
	return "sh", []string{"-c", "true"}
}

func metricsCommand() ([]string, string) {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "echo metrics-win"}, "cmd"
	}
	return []string{"sh", "-c", "printf 'metrics\\n'"}, "sh"
}

func auditCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "echo audit-ok"}
	}
	return []string{"sh", "-c", "printf 'audit-ok\\n'"}
}
