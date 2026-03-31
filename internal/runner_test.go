package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"go-command-compression-proxy/internal/audit"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/engine"
	corefilters "go-command-compression-proxy/internal/filters"
	filteryaml "go-command-compression-proxy/internal/filters/yaml"
	"go-command-compression-proxy/internal/metrics"
	"go-command-compression-proxy/internal/replay"
	"go-command-compression-proxy/internal/version"
	"go-command-compression-proxy/internal/workspaces"
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

	Context("when constructing default runner state", func() {
		It("uses default filter sources when none are provided", func() {
			runner := NewRunnerWithOptions(Options{})
			Expect(runner).NotTo(BeNil())
			Expect(runner.sources).To(Equal(filteryaml.DefaultSources()))
		})

		It("uses repository filters by default in dev builds", func() {
			oldVersion := version.Version
			version.Version = "dev"
			DeferCleanup(func() {
				version.Version = oldVersion
			})

			sources := filteryaml.DefaultSources()

			Expect(sources).To(HaveLen(1))
			Expect(sources[0]).To(Equal(corefilters.RepositorySource(filteryaml.ProjectRootFromSource())))
		})

		It("uses project and home filters by default in non-dev builds", func() {
			oldVersion := version.Version
			version.Version = "1.2.3"
			DeferCleanup(func() {
				version.Version = oldVersion
			})

			sources := filteryaml.DefaultSources()

			Expect(sources).To(HaveLen(2))
			Expect(sources[0].Kind).To(Equal(corefilters.SourceProject))
			Expect(sources[0].Directory).To(HaveSuffix(filepath.Join(".ccp", "filters")))
			Expect(sources[1].Kind).To(Equal(corefilters.SourceHome))
			Expect(sources[1].Directory).To(HaveSuffix(filepath.Join(".config", "ccp", "filters")))
		})

		It("leaves default filter sources and metrics path unset when os.Getwd fails", func() {
			oldVersion := version.Version
			version.Version = "1.2.3"
			DeferCleanup(func() { version.Version = oldVersion })

			withUnavailableWorkingDirectory(func() {
				cwd, err := os.Getwd()
				if err == nil || cwd != "" {
					Skip("platform keeps reporting a working directory after removal")
				}

				Expect(filteryaml.DefaultSources()).To(BeNil())
				Expect(defaultMetricsPath()).To(BeEmpty())
				Expect(currentWorkingDir()).To(BeEmpty())

				runner := NewRunnerWithOptions(Options{})
				Expect(runner.metricsPath).To(BeEmpty())
				Expect(runner.workingDir).To(BeEmpty())
			})
		})
	})

	Context("when starting execution", func() {
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

		It("lets RunContext fall back to Run when the parent is nil", func() {
			runner := &Runner{sources: []corefilters.FilterSource{}}
			nilParent := func() context.Context { return nil }

			code, err := runner.RunContext(nilParent(), nil)

			Expect(code).To(Equal(2))
			Expect(err).To(MatchError("no command provided"))
		})

		It("runs with an explicit parent context", func() {
			runner := &Runner{sources: []corefilters.FilterSource{}}

			code, err := runner.RunContext(context.Background(), nil)

			Expect(code).To(Equal(2))
			Expect(err).To(MatchError("no command provided"))
		})

		It("routes direct emitted entries to the correct streams", func() {
			runner := &Runner{sources: []corefilters.FilterSource{}}

			_, err := runner.writeEntries([]engine.BufferEntry{
				{Stream: contracts.StreamStdout, Line: "out-1\n"},
				{Stream: contracts.StreamStderr, Line: "err-1\n"},
				{Stream: contracts.StreamStdout, Line: "out-2\n"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(closeAndRead(stdoutReader, stdoutWriter)).To(Equal("out-1\nout-2\n"))
			Expect(closeAndRead(stderrReader, stderrWriter)).To(Equal("err-1\n"))
		})

		Context("when downstream output cannot be written", func() {
			DescribeTable("returns an error when downstream writes fail",
				func(makeRunner func(string) *Runner, command []string) {
					if runtime.GOOS == "windows" {
						Skip("uses unix sh")
					}

					brokenStdout, err := os.CreateTemp("", "core-runner-broken-stdout-*")
					Expect(err).NotTo(HaveOccurred())
					brokenStdoutPath := brokenStdout.Name()
					DeferCleanup(func() {
						Expect(os.Remove(brokenStdoutPath)).To(Succeed())
					})
					Expect(brokenStdout.Close()).To(Succeed())
					os.Stdout = brokenStdout

					repoRoot, err := os.MkdirTemp("", "core-runner-write-failure-*")
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(func() {
						Expect(os.RemoveAll(repoRoot)).To(Succeed())
					})

					runner := makeRunner(repoRoot)
					code, err := runner.Run(command)

					Expect(err).To(HaveOccurred())
					Expect(code).NotTo(Equal(0))
				},
				Entry("for filtered stdout writes",
					func(repoRoot string) *Runner {
						return &Runner{
							sources:     []corefilters.FilterSource{},
							metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
						}
					},
					[]string{"sh", "-c", "printf 'hello from ccp\\n'"},
				),
				Entry("for raw-mode flushes",
					func(string) *Runner {
						return NewRunnerWithOptions(Options{
							Raw:          true,
							Confidential: []string{"secret"},
						})
					},
					[]string{"sh", "-c", "printf 'secret-without-newline'"},
				),
			)

		})

		Context("when configured filter sources cannot be loaded", func() {
			DescribeTable("fails open and preserves native command execution",
				func(command []string, expectedExit int) {
					tmpDir, err := os.MkdirTemp("", "core-runner-registry-fallback-*")
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(func() {
						Expect(os.RemoveAll(tmpDir)).To(Succeed())
					})

					restoreAudit := audit.WithTestConfig(tmpDir, 8, 7)
					DeferCleanup(restoreAudit)
					DeferCleanup(audit.Reset)

					sourceFile := filepath.Join(tmpDir, "not-a-dir")
					Expect(os.WriteFile(sourceFile, []byte("x"), 0o644)).To(Succeed())

					runner := &Runner{sources: []corefilters.FilterSource{{Directory: sourceFile}}}

					code, err := runner.Run(command)

					Expect(err).NotTo(HaveOccurred())
					Expect(code).To(Equal(expectedExit))
					Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("registry-fallback\n"))
					Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())

					auditData, err := os.ReadFile(filepath.Join(tmpDir, ".config", "ccp", "audit", "audit.log"))
					Expect(err).NotTo(HaveOccurred())
					Expect(string(auditData)).To(ContainSubstring(`"msg":"execution_registry_error"`))
					Expect(string(auditData)).To(ContainSubstring(`"msg":"filter_fallback"`))
					Expect(string(auditData)).To(ContainSubstring(`"msg":"execution_finish"`))
				},
				Entry("for successful commands", registryFailureCommand(0), 0),
				Entry("for non-zero exit codes", registryFailureCommand(7), 7),
			)
		})
	})

	Context("when draining streams", func() {
		DescribeTable("copies trailing lines before surfacing non-EOF read errors",
			func(line string, invoke func(*Runner, io.Reader, *streamStats) error, expectStats bool) {
				runner := &Runner{sources: []corefilters.FilterSource{}}
				src := &errorAfterLineReader{
					line: line,
					err:  errors.New("boom"),
				}
				stats := &streamStats{}

				err := invoke(runner, src, stats)
				Expect(err).To(MatchError(ContainSubstring("read stream: boom")))

				Expect(closeAndRead(stdoutReader, stdoutWriter)).To(Equal(line))
				Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
				if expectStats {
					Expect(stats.rawBytes).To(Equal(len(line)))
					Expect(stats.keptBytes).To(Equal(len(line)))
				}
			},
			Entry("for filtered output", "tail-without-newline", func(runner *Runner, src io.Reader, stats *streamStats) error {
				return runner.drainStream(src, func(line string) []engine.BufferEntry {
					return []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: line}}
				}, stats, runner.writeEntries)
			}, true),
			Entry("for raw output", "raw-tail-without-newline", func(runner *Runner, src io.Reader, _ *streamStats) error {
				return runner.copyRawStream(src, stdoutWriter)
			}, false),
		)

		DescribeTable("normalizes carriage-return stream behavior before emit",
			func(input string, expectedStdout string, rawBytes int, keptBytes int) {
				runner := &Runner{sources: []corefilters.FilterSource{}}
				stats := &streamStats{}

				Expect(runner.drainStream(strings.NewReader(input), func(line string) []engine.BufferEntry {
					return []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: line}}
				}, stats, runner.writeEntries)).NotTo(HaveOccurred())

				Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal(expectedStdout))
				Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
				Expect(stats.rawBytes).To(Equal(rawBytes))
				Expect(stats.keptBytes).To(Equal(keptBytes))
			},
			Entry("for in-place carriage return overwrites", "\r⠋ first\r⠙ second\rDone\n", "Done\n", len("\r⠋ first\r⠙ second\rDone\n"), len("Done\n")),
			Entry("for ordinary CRLF endings", "runner-win\r\n", "runner-win\n", len("runner-win\r\n"), len("runner-win\n")),
		)

		It("treats nil stream readers as empty input", func() {
			runner := &Runner{sources: []corefilters.FilterSource{}}

			Expect(runner.drainStream(nil, func(line string) []engine.BufferEntry {
				return []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: line}}
			}, &streamStats{}, runner.writeEntries)).To(Succeed())
			Expect(closeAndRead(stdoutReader, stdoutWriter)).To(BeEmpty())
			Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())
		})
	})

	Context("when executing real commands", func() {
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

		It("executes a real command on Windows", func() {
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
	})

	Context("when recording metrics", func() {
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

		It("marks yaml-authored grep invert-match cases as passthrough", func() {
			if runtime.GOOS == "windows" {
				Skip("uses unix grep")
			}

			tmpDir, err := os.MkdirTemp("", "core-runner-grep-passthrough-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(os.RemoveAll(tmpDir)).To(Succeed())
			})

			oldStdin := os.Stdin
			stdinReader, stdinWriter, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			_, err = io.WriteString(stdinWriter, "keep\nfiltered\nalso keep\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdinWriter.Close()).To(Succeed())
			os.Stdin = stdinReader
			DeferCleanup(func() {
				os.Stdin = oldStdin
				_ = stdinReader.Close()
			})

			runner := &Runner{
				sources: []corefilters.FilterSource{
					corefilters.RepositorySource(filteryaml.ProjectRootFromSource()),
				},
				metricsPath: filepath.Join(tmpDir, ".ccp", "gain.db"),
			}

			code, err := runner.Run([]string{"grep", "-v", "filtered"})

			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
			Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("keep\nalso keep\n"))
			Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())

			history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			Expect(history[0].Tool).To(Equal("grep"))
			Expect(history[0].DispatchKey).To(Equal("grep|precision_short_passthrough"))
			Expect(history[0].Passthrough).To(BeTrue())
			Expect(history[0].DroppedBytes).To(BeZero())
			Expect(history[0].EstimatedSavedTokens).To(BeZero())
		})

		DescribeTable("does not record wrapped ccp lifecycle metrics",
			func(rawInput string, args []string) {
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
					RawInput: rawInput,
					Args:     args,
					Tool:     "ccp",
					Dispatch: "ccp",
				}

				runner.appendMetrics(command, true, 0, 1, 32, 32)

				history, err := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(history).To(BeEmpty())
			},
			Entry("for history", "ccp history", []string{"ccp", "history"}),
			Entry("for capture", "ccp capture -- echo hi", []string{"ccp", "capture", "--", "echo", "hi"}),
		)

		It("registers the current working directory after writing normal gain metrics", func() {
			tmpDir, err := os.MkdirTemp("", "core-runner-workspaces-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(os.RemoveAll(tmpDir)).To(Succeed())
			})
			restore := workspaces.WithTestConfig(tmpDir, nil)
			DeferCleanup(restore)

			runner := &Runner{
				sources:     []corefilters.FilterSource{},
				metricsPath: filepath.Join(tmpDir, "repo", ".ccp", "gain.db"),
				workingDir:  filepath.Join(tmpDir, "repo"),
			}

			command := contracts.Command{
				RawInput: "go test ./...",
				Args:     []string{"go", "test", "./..."},
				Tool:     "go",
				Dispatch: "go",
			}

			runner.appendMetrics(command, false, 0, 1, 32, 16)

			registryPath, err := workspaces.DefaultPath()
			Expect(err).NotTo(HaveOccurred())
			entries, err := workspaces.ListPath(registryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].CWD).To(Equal(filepath.Join(tmpDir, "repo")))
			Expect(entries[0].MetricsPath).To(Equal(filepath.Join(tmpDir, "repo", ".ccp", "gain.db")))
		})

		It("records append metrics no-ops for raw mode, failed writes, and blank working directories", func() {
			tmpDir := GinkgoT().TempDir()
			restore := workspaces.WithTestConfig(tmpDir, nil)
			DeferCleanup(restore)

			command := contracts.Command{
				RawInput: "go test ./...",
				Args:     []string{"go", "test", "./..."},
				Tool:     "go",
				Dispatch: "go",
			}

			rawRunner := NewRunnerWithOptions(Options{Raw: true, MetricsPath: filepath.Join(tmpDir, "raw.db")})
			rawRunner.appendMetrics(command, false, 0, 1, 10, 5)
			history, err := metrics.QueryHistory(rawRunner.metricsPath, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(BeEmpty())

			parentFile := filepath.Join(tmpDir, "not-a-dir")
			Expect(os.WriteFile(parentFile, []byte("x"), 0o644)).To(Succeed())
			brokenRunner := &Runner{metricsPath: filepath.Join(parentFile, "gain.db"), workingDir: filepath.Join(tmpDir, "repo")}
			brokenRunner.appendMetrics(command, false, 0, 1, 10, 5)
			registryPath, err := workspaces.DefaultPath()
			Expect(err).NotTo(HaveOccurred())
			entries, err := workspaces.ListPath(registryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())

			blankRunner := &Runner{metricsPath: filepath.Join(tmpDir, "blank.db"), workingDir: "   "}
			blankRunner.appendMetrics(command, false, 0, 1, 10, 5)
			history, err = metrics.QueryHistory(blankRunner.metricsPath, metrics.QueryOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(history).To(HaveLen(1))
			entries, err = workspaces.ListPath(registryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})
	})

	Context("when recording audit events", func() {
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

		It("redacts confidential command arguments in audit logs", func() {
			tmpDir, err := os.MkdirTemp("", "core-runner-audit-secret-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(os.RemoveAll(tmpDir)).To(Succeed())
			})

			restoreAudit := audit.WithTestConfig(tmpDir, 8, 7)
			DeferCleanup(restoreAudit)
			DeferCleanup(audit.Reset)

			secret := "super-secret-token"
			runner := NewRunnerWithOptions(Options{Confidential: []string{secret}})

			code, err := runner.Run(confidentialAuditCommand(secret))

			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
			auditData, err := os.ReadFile(filepath.Join(tmpDir, ".config", "ccp", "audit", "audit.log"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(auditData)).To(ContainSubstring(`***`))
			Expect(string(auditData)).NotTo(ContainSubstring(secret))
		})

		It("does not fail execution when audit logging cannot initialize", func() {
			tmpDir, err := os.MkdirTemp("", "core-runner-audit-fail-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(os.RemoveAll(tmpDir)).To(Succeed())
			})
			Expect(os.WriteFile(filepath.Join(tmpDir, ".config"), []byte("block"), 0o644)).To(Succeed())

			restoreAudit := audit.WithTestConfig(tmpDir, 8, 7)
			DeferCleanup(restoreAudit)
			DeferCleanup(audit.Reset)

			runner := &Runner{sources: []corefilters.FilterSource{}}

			code, err := runner.Run(auditCommand())

			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
			Expect(closeAndRead(stdoutReader, stdoutWriter)).To(ContainSubstring("audit-ok"))
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
	})

	Context("when managing subprocess plumbing", func() {
		It("creates subprocess pipes with stdin attached to os.Stdin", func() {
			name, args := successCommand()

			cmd, stdout, stderr, err := CommandWithPipesContext(context.Background(), name, args)

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

		It("cancels managed subprocesses when the execution context ends", func() {
			startedPath := filepath.Join(GinkgoT().TempDir(), "started.txt")
			markerPath := filepath.Join(GinkgoT().TempDir(), "late.txt")
			name, args := cancellableCommand(startedPath, markerPath)

			ctx, cancel := context.WithCancel(context.Background())
			cmd, stdout, stderr, err := CommandWithPipesContext(ctx, name, args)

			Expect(err).NotTo(HaveOccurred())
			cmd.Env = append(os.Environ(), "CCP_MANAGED_SUBPROCESS_HELPER=1")
			Expect(cmd.Start()).To(Succeed())
			DeferCleanup(func() { closePipes(stdout, stderr) })

			Eventually(func() error {
				_, err := os.Stat(startedPath)
				return err
			}, time.Second).Should(Succeed())

			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			cancel()
			Eventually(done, 5*time.Second).Should(Receive(HaveOccurred()))
			Consistently(func() bool {
				_, err := os.Stat(markerPath)
				return err == nil
			}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())
		})

		It("kills descendant processes when the execution context ends", func() {
			if runtime.GOOS == "windows" {
				Skip("uses unix process groups")
			}

			startedPath := filepath.Join(GinkgoT().TempDir(), "started.txt")
			childStartedPath := filepath.Join(GinkgoT().TempDir(), "child-started.txt")
			markerPath := filepath.Join(GinkgoT().TempDir(), "orphan.txt")
			name, args := descendantCommand(startedPath, childStartedPath, markerPath)
			ctx, cancel := context.WithCancel(context.Background())
			cmd, stdout, stderr, err := CommandWithPipesContext(ctx, name, args)

			Expect(err).NotTo(HaveOccurred())
			cmd.Env = append(os.Environ(), "CCP_MANAGED_DESCENDANT_HELPER=1")
			Expect(cmd.Start()).To(Succeed())
			DeferCleanup(func() { closePipes(stdout, stderr) })

			Eventually(func() error {
				_, err := os.Stat(startedPath)
				return err
			}, time.Second).Should(Succeed())
			Eventually(func() error {
				_, err := os.Stat(childStartedPath)
				return err
			}, time.Second).Should(Succeed())

			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			cancel()
			Eventually(done, 5*time.Second).Should(Receive(HaveOccurred()))
			Consistently(func() bool {
				_, err := os.Stat(markerPath)
				return err == nil
			}, 1500*time.Millisecond, 100*time.Millisecond).Should(BeFalse())
		})
	})

	Context("when exercising runner helpers", func() {
		DescribeTable("classifies filtered run outcomes",
			func(
				setupCtx func() (context.Context, context.CancelFunc),
				waitErr error,
				outputErr error,
				exitCode int,
				expectedCode int,
				expectedErr string,
			) {
				ctx, cancel := setupCtx()
				DeferCleanup(cancel)

				code, err := filteredRunResult(ctx, waitErr, outputErr, exitCode)

				Expect(code).To(Equal(expectedCode))
				if expectedErr == "" {
					Expect(err).NotTo(HaveOccurred())
					return
				}
				Expect(err).To(MatchError(ContainSubstring(expectedErr)))
			},
			Entry("returns cancellation errors first",
				func() (context.Context, context.CancelFunc) {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx, func() {}
				},
				nil, errors.New("write failed"), 0, 1, "context canceled",
			),
			Entry("returns wait errors before exit codes",
				func() (context.Context, context.CancelFunc) {
					return context.WithCancel(context.Background())
				},
				errors.New("wait failed"), errors.New("write failed"), 7, 1, "wait failed",
			),
			Entry("returns the native exit code on success",
				func() (context.Context, context.CancelFunc) {
					return context.WithCancel(context.Background())
				},
				nil, nil, 7, 7, "",
			),
			Entry("maps output failures on zero exits to code one",
				func() (context.Context, context.CancelFunc) {
					return context.WithCancel(context.Background())
				},
				nil, errors.New("write failed"), 0, 1, "write failed",
			),
			Entry("preserves non-zero exits when output writing fails",
				func() (context.Context, context.CancelFunc) {
					return context.WithCancel(context.Background())
				},
				nil, errors.New("write failed"), 7, 7, "write failed",
			),
		)

		DescribeTable("maps audit failures onto exit semantics",
			func(exitCode int, expectedCode int) {
				code, err := auditFailureResult(exitCode, errors.New("audit failed"))

				Expect(code).To(Equal(expectedCode))
				Expect(err).To(MatchError("audit failed"))
			},
			Entry("for successful commands", 0, 1),
			Entry("for non-zero exits", 7, 7),
		)

		It("covers replay collector helpers and action labels", func() {
			collector := &replayCollector{}

			collector.recordInput(
				replay.Event{Stream: contracts.StreamStdout, Line: "replace me\n"},
				contracts.Action{Kind: contracts.ActionReplace},
				[]engine.BufferEntry{{Stream: contracts.StreamStdout, Line: "rewritten\n"}},
			)
			collector.recordInput(
				replay.Event{Stream: contracts.StreamStderr, Line: "skip me\n"},
				contracts.Action{Kind: contracts.ActionIgnore},
				nil,
			)
			collector.recordExit(contracts.Action{}, []engine.BufferEntry{{Stream: contracts.StreamStdout, Line: "exit\n"}})

			Expect(collector.output.String()).To(Equal("rewritten\nexit\n"))
			Expect(collector.decisions.String()).To(ContainSubstring("<replace> | replace me"))
			Expect(collector.decisions.String()).To(ContainSubstring("<emit>    | rewritten"))
			Expect(collector.decisions.String()).To(ContainSubstring("<skip>    | skip me"))

			Expect(labelForInputAction(contracts.Action{Kind: contracts.ActionEmit})).To(Equal("<keep>"))
			Expect(labelForInputAction(contracts.Action{Kind: contracts.ActionKeep})).To(Equal("<keep>"))
			Expect(labelForInputAction(contracts.Action{Kind: "unknown"})).To(Equal("<keep>"))
			Expect(splitDecisionLines("")).To(Equal([]string{""}))
			Expect(splitDecisionLines("a\r\nb\nc")).To(Equal([]string{"a\n", "b\n", "c"}))
		})

		It("covers writer helpers and stderr naming", func() {
			buffer := &strings.Builder{}
			writer := &errorRecordingWriter{writer: buffer, name: outputName(os.Stderr)}

			n, err := writer.Write([]byte("stderr-line"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len("stderr-line")))
			Expect(buffer.String()).To(Equal("stderr-line"))
			Expect(outputName(os.Stderr)).To(Equal("stderr"))
			Expect(outputName(os.Stdout)).To(Equal("stdout"))

			writer.err = errors.New("already-failed")
			n, err = writer.Write([]byte("ignored"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len("ignored")))

			writer = &errorRecordingWriter{name: "stdout"}
			n, err = writer.Write([]byte("nil-writer"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len("nil-writer")))

			writer = &errorRecordingWriter{writer: failingWriter{err: errors.New("boom")}, name: "stdout"}
			n, err = writer.Write([]byte("wrapped"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len("wrapped")))
			Expect(writer.err).To(MatchError("write stdout: boom"))
		})

		It("covers redaction helpers for buffered writes and flush behavior", func() {
			Expect(redactConfidential("", []string{"secret"})).To(BeEmpty())
			Expect(redactConfidential("secret value", []string{"", "secret"})).To(Equal("*** value"))

			buffer := &strings.Builder{}
			writer := &redactingWriter{writer: buffer, confidential: []string{"secret"}}
			n, err := writer.Write([]byte("secret\ntrail-secret"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len("secret\ntrail-secret")))
			Expect(buffer.String()).To(Equal("***\n"))
			Expect(writer.Flush()).To(Succeed())
			Expect(buffer.String()).To(Equal("***\ntrail-***"))

			writer = &redactingWriter{}
			n, err = writer.Write([]byte("ignored"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len("ignored")))
			Expect(writer.Flush()).To(Succeed())

			writer = &redactingWriter{writer: failingWriter{err: errors.New("flush boom")}, confidential: []string{"secret"}}
			_, err = writer.Write([]byte("secret\n"))
			Expect(err).To(MatchError("flush boom"))

			writer = &redactingWriter{writer: failingWriter{err: errors.New("flush tail boom")}, confidential: []string{"secret"}, buf: []byte("secret")}
			Expect(writer.Flush()).To(MatchError("flush tail boom"))
		})

		It("surfaces direct writeRedacted failures with stderr labels", func() {
			brokenStderr, err := os.CreateTemp("", "core-runner-broken-stderr-*")
			Expect(err).NotTo(HaveOccurred())
			brokenStderrPath := brokenStderr.Name()
			DeferCleanup(func() {
				Expect(os.Remove(brokenStderrPath)).To(Succeed())
			})
			Expect(brokenStderr.Close()).To(Succeed())
			os.Stderr = brokenStderr

			runner := NewRunnerWithOptions(Options{Confidential: []string{"secret"}})
			written, err := runner.writeRedacted(brokenStderr, "secret")

			Expect(written).To(Equal(0))
			Expect(err).To(MatchError(ContainSubstring("write stderr:")))
			Expect(runner.auditCommand("secret command")).To(Equal("*** command"))
		})
	})

	Context("when loading filter registries", func() {
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

		Context("when verifying and replaying captured output", func() {
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
					strings.NewReader("00000|===== 2 passed in 0.12s =====\n"),
					nil,
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(Equal("pytest: 2 passed\n"))
			})

			It("preserves stdout stderr interleaving during verify when readers carry sequence prefixes", func() {
				repoRoot, err := os.MkdirTemp("", "core-runner-verify-order-*")
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
  - id: order
    compress_output:
      combined:
        lines:
          keep:
            - regex: '^'
`), 0o644)).To(Succeed())

				runner := &Runner{sources: []corefilters.FilterSource{
					corefilters.RepositorySource(repoRoot),
				}}

				expected, err := runner.ReplayWithExitCode([]string{"pytest"}, []replay.Event{
					{Sequence: 0, Stream: contracts.StreamStdout, Line: "out-1\n"},
					{Sequence: 1, Stream: contracts.StreamStderr, Line: "err-1\n"},
					{Sequence: 2, Stream: contracts.StreamStdout, Line: "out-2\n"},
				}, 0)
				Expect(err).NotTo(HaveOccurred())

				actual, err := runner.Verify(
					[]string{"pytest"},
					strings.NewReader("00000|out-1\n00002|out-2\n"),
					strings.NewReader("00001|err-1\n"),
				)

				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(Equal(expected.Output))
			})

			It("returns verify input reader errors before replay", func() {
				runner := &Runner{sources: []corefilters.FilterSource{}}

				_, err := runner.Verify(
					[]string{"git", "status"},
					&readErrorReader{err: errors.New("boom")},
					nil,
				)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("read sequenced stream stdout: boom"))
			})

			DescribeTable("reports missing command arguments before replay",
				func(invoke func(*Runner) error) {
					runner := &Runner{sources: []corefilters.FilterSource{}}

					err := invoke(runner)

					Expect(err).To(MatchError("no command provided"))
				},
				Entry("for verify", func(runner *Runner) error {
					_, err := runner.Verify(nil, nil, nil)
					return err
				}),
				Entry("for replay", func(runner *Runner) error {
					_, err := runner.ReplayWithExitCode(nil, nil, 0)
					return err
				}),
			)

			DescribeTable("reports registry loading failures before replay",
				func(invoke func(*Runner) error) {
					sourceFile := filepath.Join(GinkgoT().TempDir(), "not-a-dir")
					Expect(os.WriteFile(sourceFile, []byte("x"), 0o644)).To(Succeed())

					runner := &Runner{sources: []corefilters.FilterSource{{Directory: sourceFile}}}

					err := invoke(runner)

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not a directory"))
				},
				Entry("for verify", func(runner *Runner) error {
					_, err := runner.Verify([]string{"git", "status"}, strings.NewReader("00000|ok\n"), nil)
					return err
				}),
				Entry("for replay", func(runner *Runner) error {
					_, err := runner.ReplayWithExitCode([]string{"git", "status"}, nil, 0)
					return err
				}),
			)

			It("replays stderr events and explicit exit codes without errors", func() {
				runner := &Runner{sources: []corefilters.FilterSource{}}

				result, err := runner.ReplayWithExitCode([]string{"git", "status"}, []replay.Event{{
					Sequence: 0,
					Stream:   contracts.StreamStderr,
					Line:     "stderr-only\n",
				}}, 7)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Output).To(Equal("stderr-only\n"))
				Expect(result.Decisions).To(ContainSubstring("<keep>    | stderr-only"))
			})
		})

		Context("when resolving mappings and source precedence", func() {
			DescribeTable("resolves filters with source-local mapping behavior",
				func(tool string, mappingFile string, args []string) {
					repoRoot, err := os.MkdirTemp("", "core-runner-mappings-*")
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(func() {
						Expect(os.RemoveAll(repoRoot)).To(Succeed())
					})
					filterDir := filepath.Join(repoRoot, "filters")
					Expect(os.MkdirAll(filterDir, 0o755)).To(Succeed())
					Expect(os.WriteFile(filepath.Join(filterDir, ".mappings.yaml"), []byte(mappingFile), 0o644)).To(Succeed())
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
						Tool: tool,
						Args: args,
					})
					Expect(resolved).NotTo(Equal(corefilters.Passthrough{}))
				},
				Entry("with valid mappings", "py", "version: 1\nmap:\n  py: python\n", []string{"py", "-m", "pytest"}),
				Entry("with invalid mappings that fall back to direct lookup", "python", "version: oops\n", []string{"python", "-m", "pytest"}),
			)

			DescribeTable("prefers project sources over home sources",
				func(tool string, writeMappings bool) {
					root, err := os.MkdirTemp("", "core-runner-project-home-*")
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

					if writeMappings {
						Expect(os.WriteFile(filepath.Join(projectDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  py: python\n"), 0o644)).To(Succeed())
						Expect(os.WriteFile(filepath.Join(homeDir, ".mappings.yaml"), []byte("version: 1\nmap:\n  py: python\n"), 0o644)).To(Succeed())
					}

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
						Tool: tool,
						Args: []string{tool, "-m", "pytest"},
					})
					Expect(resolved.Dispatch(contracts.Command{
						Tool: tool,
						Args: []string{tool, "-m", "pytest"},
					})).To(Equal("python|project"))
				},
				Entry("for direct filter identities", "python", false),
				Entry("for mapped aliases", "py", true),
			)
		})

		Context("when applying YAML-authored execution behavior", func() {
			DescribeTable("applies YAML-authored execution behavior before command output is emitted",
				func(skipReason string, setup func(string) *Runner, command []string, expectedStdout string, expectedStderr string, checkHistory func(string)) {
					if runtime.GOOS == "windows" {
						Skip(skipReason)
					}

					repoRoot, err := os.MkdirTemp("", "core-runner-yaml-execution-*")
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(func() {
						Expect(os.RemoveAll(repoRoot)).To(Succeed())
					})

					runner := setup(repoRoot)
					code, err := runner.Run(command)

					Expect(err).NotTo(HaveOccurred())
					Expect(code).To(Equal(0))
					Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal(expectedStdout))
					Expect(normalizeNL(closeAndRead(stderrReader, stderrWriter))).To(Equal(expectedStderr))
					checkHistory(runner.metricsPath)
				},
				Entry("with YAML command mutation",
					"uses unix printf",
					func(repoRoot string) *Runner {
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

						return &Runner{
							sources: []corefilters.FilterSource{
								corefilters.RepositorySource(repoRoot),
							},
							metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
						}
					},
					[]string{"printf"},
					"mutated\n",
					"",
					func(metricsPath string) {
						history, err := metrics.QueryHistory(metricsPath, metrics.QueryOptions{})
						Expect(err).NotTo(HaveOccurred())
						Expect(history).To(HaveLen(1))
						Expect(history[0].DispatchKey).To(Equal("printf|default"))
					},
				),
				Entry("with invalid mapped scaffold fallback",
					"uses unix cat",
					func(repoRoot string) *Runner {
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

						return &Runner{
							sources: []corefilters.FilterSource{
								corefilters.RepositorySource(repoRoot),
							},
							metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
						}
					},
					[]string{"cat"},
					"fallback\n",
					"",
					func(metricsPath string) {
						history, err := metrics.QueryHistory(metricsPath, metrics.QueryOptions{})
						Expect(err).NotTo(HaveOccurred())
						Expect(history).To(HaveLen(1))
						Expect(history[0].Tool).To(Equal("cat"))
						Expect(history[0].DispatchKey).To(Equal("cat"))
						Expect(history[0].Passthrough).To(BeTrue())
					},
				),
			)

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

			It("does not report negative savings when exit handling emits synthetic output", func() {
				if runtime.GOOS == "windows" {
					Skip("uses unix sh")
				}

				repoRoot, err := os.MkdirTemp("", "core-runner-exit-metrics-*")
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
  - id: final_summary
    compress_output:
      stdout:
        lines:
          keep:
            - regex: '^'
    finally:
      print: 'summary: synthetic exit expansion'
`), 0o644)).To(Succeed())

				runner := &Runner{
					sources: []corefilters.FilterSource{
						corefilters.RepositorySource(repoRoot),
					},
					metricsPath: filepath.Join(repoRoot, ".ccp", "gain.db"),
				}

				code, err := runner.Run([]string{"sh", "-c", "printf 'x\\n'"})

				Expect(err).NotTo(HaveOccurred())
				Expect(code).To(Equal(0))
				Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal("x\nsummary: synthetic exit expansion\n"))
				Expect(closeAndRead(stderrReader, stderrWriter)).To(BeEmpty())

				history, queryErr := metrics.QueryHistory(runner.metricsPath, metrics.QueryOptions{})
				Expect(queryErr).NotTo(HaveOccurred())
				Expect(history).To(HaveLen(1))
				Expect(history[0].DroppedBytes).To(BeNumerically(">=", 0))
				Expect(history[0].DropRatio).To(BeNumerically(">=", 0))
				Expect(history[0].EstimatedSavedTokens).To(BeNumerically(">=", 0))
			})

			DescribeTable("preserves output semantics for direct execution modes",
				func(runner *Runner, command []string, expectedStdout string, expectedStderr string) {
					if runtime.GOOS == "windows" {
						Skip("uses unix sh")
					}

					code, err := runner.Run(command)

					Expect(err).NotTo(HaveOccurred())
					Expect(code).To(Equal(0))
					Expect(normalizeNL(closeAndRead(stdoutReader, stdoutWriter))).To(Equal(expectedStdout))
					Expect(normalizeNL(closeAndRead(stderrReader, stderrWriter))).To(Equal(expectedStderr))
				},
				Entry("in raw mode", NewRunnerWithOptions(Options{Raw: true}), []string{"sh", "-c", "printf 'same\\nsame\\n'"}, "same\nsame\n", ""),
				Entry("with confidential redaction", NewRunnerWithOptions(Options{Confidential: []string{"hello"}}), []string{"sh", "-c", "printf 'hello-stdout\\n'; printf 'hello-stderr\\n' >&2"}, "***-stdout\n", "***-stderr\n"),
			)
		})
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

func registryFailureCommand(exitCode int) []string {
	if runtime.GOOS == "windows" {
		if exitCode == 0 {
			return []string{"cmd", "/c", "@echo registry-fallback"}
		}
		return []string{"cmd", "/c", fmt.Sprintf("@echo registry-fallback&&exit /b %d", exitCode)}
	}
	return []string{"sh", "-c", fmt.Sprintf("printf 'registry-fallback\\n'; exit %d", exitCode)}
}

func auditCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "echo audit-ok"}
	}
	return []string{"sh", "-c", "printf 'audit-ok\\n'"}
}

func confidentialAuditCommand(secret string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "echo", secret}
	}
	return []string{"echo", secret}
}

func cancellableCommand(startedPath, markerPath string) (string, []string) {
	return os.Args[0], []string{"-test.run=TestManagedSubprocessHelper", "--", startedPath, markerPath}
}

func descendantCommand(startedPath, childStartedPath, markerPath string) (string, []string) {
	return os.Args[0], []string{"-test.run=TestManagedDescendantHelper", "--", startedPath, childStartedPath, markerPath}
}

func TestManagedSubprocessHelper(t *testing.T) {
	if os.Getenv("CCP_MANAGED_SUBPROCESS_HELPER") != "1" {
		return
	}

	sep := slices.Index(os.Args, "--")
	if sep < 0 || len(os.Args) < sep+3 {
		os.Exit(2)
	}
	startedPath := os.Args[sep+1]
	markerPath := os.Args[sep+2]
	if err := os.WriteFile(startedPath, []byte("started"), 0o644); err != nil {
		os.Exit(3)
	}
	time.Sleep(30 * time.Second)
	if err := os.WriteFile(markerPath, []byte("late"), 0o644); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestManagedDescendantHelper(t *testing.T) {
	if os.Getenv("CCP_MANAGED_DESCENDANT_HELPER") != "1" {
		return
	}

	sep := slices.Index(os.Args, "--")
	if sep < 0 || len(os.Args) < sep+4 {
		os.Exit(2)
	}
	startedPath := os.Args[sep+1]
	childStartedPath := os.Args[sep+2]
	markerPath := os.Args[sep+3]
	if os.Getenv("CCP_MANAGED_DESCENDANT_HELPER_MODE") == "child" {
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
	child := exec.Command(os.Args[0], "-test.run=TestManagedDescendantHelper", "--", startedPath, childStartedPath, markerPath)
	child.Env = append(os.Environ(), "CCP_MANAGED_DESCENDANT_HELPER=1", "CCP_MANAGED_DESCENDANT_HELPER_MODE=child")
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Stdin = os.Stdin
	if err := child.Start(); err != nil {
		os.Exit(4)
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func withUnavailableWorkingDirectory(fn func()) {
	original, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	tmpDir := GinkgoT().TempDir()
	Expect(os.Chdir(tmpDir)).To(Succeed())
	defer func() {
		Expect(os.Chdir(original)).To(Succeed())
	}()

	err = os.RemoveAll(tmpDir)
	if err != nil {
		Skip("cannot remove current working directory on this platform: " + err.Error())
	}

	fn()
}

type readErrorReader struct {
	err error
}

func (r *readErrorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
