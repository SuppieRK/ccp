package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/cli"
	"go-command-compression-proxy/internal/version"
)

var _ = Describe("ccp main", func() {
	Describe("buildRuntime", func() {
		It("builds the YAML-backed runtime by default", func() {
			r, err := buildRuntime(cli.Options{CommandArgs: []string{"ls"}})

			Expect(err).NotTo(HaveOccurred())
			Expect(r).NotTo(BeNil())
		})
	})

	Describe("usageText", func() {
		It("includes the expected usage sections", func() {
			got := usageText()

			Expect(got).NotTo(BeEmpty())
			Expect(got).To(ContainSubstring("ccp - command compression proxy for coding-agent workflows"))
			Expect(got).To(ContainSubstring("Usage:"))
			Expect(got).To(ContainSubstring("Execution flags:"))
			Expect(got).To(ContainSubstring("Lifecycle commands:"))
			Expect(got).To(ContainSubstring("Notes:"))
			Expect(got).To(ContainSubstring("--confidential"))
			Expect(got).To(ContainSubstring("capture               Write command.yaml, sequenced streams, and replay output artifacts"))
			Expect(got).To(ContainSubstring("init"))
			Expect(got).To(ContainSubstring("filter                YAML filter authoring helpers"))
			Expect(got).To(ContainSubstring("verify                Replay one fixture directory through the current filter"))
			Expect(got).To(ContainSubstring("gain                  Show token savings summary and recent proof output (--global supported)"))
			Expect(got).To(ContainSubstring("history               Show recorded command history (--global supported)"))
			Expect(got).To(ContainSubstring("uninstall             Remove selected integrations or fully uninstall ccp"))
			Expect(got).To(ContainSubstring("Run ccp gain after install or init to verify savings on real work."))
			Expect(got).To(ContainSubstring("--raw preserves native output unless --confidential is also used."))
		})
	})

	Describe("runInvocation", func() {
		Context("when lifecycle dispatch handles the command", func() {
			It("returns a handled success without building the runtime", func() {
				previous := lifecycleDispatch
				lifecycleDispatch = func(args []string) (bool, error) {
					Expect(args).To(Equal([]string{"history", "--help"}))
					return true, nil
				}
				DeferCleanup(func() { lifecycleDispatch = previous })

				handled, exitCode, err := runInvocation(cli.Options{CommandArgs: []string{"history", "--help"}})

				Expect(err).NotTo(HaveOccurred())
				Expect(handled).To(BeTrue())
				Expect(exitCode).To(BeZero())
			})

			It("converts lifecycle dispatch errors into exit code 1", func() {
				previous := lifecycleDispatch
				lifecycleDispatch = func(args []string) (bool, error) {
					Expect(args).To(Equal([]string{"verify", "fixture"}))
					return true, errors.New("lifecycle boom")
				}
				DeferCleanup(func() { lifecycleDispatch = previous })

				handled, exitCode, err := runInvocation(cli.Options{CommandArgs: []string{"verify", "fixture"}})

				Expect(err).To(MatchError("lifecycle boom"))
				Expect(handled).To(BeTrue())
				Expect(exitCode).To(Equal(1))
			})
		})

		Context("when wrapped execution fails before process exit", func() {
			It("returns the runner exit code and error", func() {
				handled, exitCode, err := runInvocation(cli.Options{CommandArgs: []string{"__ccp_missing_binary__"}})

				Expect(err).To(HaveOccurred())
				Expect(handled).To(BeTrue())
				Expect(exitCode).To(Equal(127))
			})
		})
	})

	Describe("run", func() {
		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		BeforeEach(func() {
			stdout.Reset()
			stderr.Reset()
		})

		Context("when argument parsing fails", func() {
			It("returns exit code 2 and writes the parse error to stderr", func() {
				code := run([]string{"--unknown"}, &stdout, &stderr)

				Expect(code).To(Equal(2))
				Expect(stdout.String()).To(BeEmpty())
				Expect(stderr.String()).To(ContainSubstring("unknown flag: --unknown"))
			})
		})

		Context("when help is requested", func() {
			It("writes usage to stdout", func() {
				code := run([]string{"--help"}, &stdout, &stderr)

				Expect(code).To(BeZero())
				Expect(stdout.String()).To(ContainSubstring("Usage:"))
				Expect(stderr.String()).To(BeEmpty())
			})
		})

		Context("when version is requested", func() {
			It("writes the current version to stdout", func() {
				prevVersion := version.Version
				version.Version = "9.8.7"
				DeferCleanup(func() { version.Version = prevVersion })

				code := run([]string{"--version"}, &stdout, &stderr)

				Expect(code).To(BeZero())
				Expect(stdout.String()).To(Equal("9.8.7\n"))
				Expect(stderr.String()).To(BeEmpty())
			})
		})

		Context("when no command is provided", func() {
			It("returns exit code 2 and writes usage to stderr", func() {
				code := run(nil, &stdout, &stderr)

				Expect(code).To(Equal(2))
				Expect(stdout.String()).To(BeEmpty())
				Expect(stderr.String()).To(ContainSubstring("Usage:"))
			})
		})

		Context("when lifecycle dispatch handles the command", func() {
			It("returns the lifecycle exit code without writing diagnostics", func() {
				previous := lifecycleDispatch
				lifecycleDispatch = func(args []string) (bool, error) {
					Expect(args).To(Equal([]string{"history", "--help"}))
					return true, nil
				}
				DeferCleanup(func() { lifecycleDispatch = previous })

				code := run([]string{"history", "--help"}, &stdout, &stderr)

				Expect(code).To(BeZero())
				Expect(stdout.String()).To(BeEmpty())
				Expect(stderr.String()).To(BeEmpty())
			})
		})

		Context("when wrapped execution fails", func() {
			It("returns the runner exit code and writes the error to stderr", func() {
				code := run([]string{"__ccp_missing_binary__"}, &stdout, &stderr)

				Expect(code).To(Equal(127))
				Expect(stdout.String()).To(BeEmpty())
				Expect(stderr.String()).NotTo(BeEmpty())
			})
		})
	})

	Describe("main", func() {
		var (
			cmd    *exec.Cmd
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		BeforeEach(func() {
			cmd = nil
			stdout.Reset()
			stderr.Reset()
		})

		Context("when invoked without an execution command", func() {
			It("prints usage and exits non-zero", func() {
				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper")
				cmd.Env = append(os.Environ(), "CCP_MAIN_TEST_HELPER=1")
				cmd.Stderr = &stderr

				err := cmd.Run()
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
				Expect(exitErr.ExitCode()).NotTo(Equal(0))
				Expect(stderr.String()).To(ContainSubstring("Usage:"))
				Expect(stderr.String()).To(ContainSubstring("Execution flags:"))
				Expect(stderr.String()).To(ContainSubstring("Lifecycle commands:"))
			})
		})

		Context("when invoked with --help", func() {
			It("prints usage to stdout and exits successfully", func() {
				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper-help")
				cmd.Env = append(os.Environ(), "CCP_MAIN_TEST_HELPER_HELP=1")
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				Expect(cmd.Run()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("Usage:"))
				Expect(stdout.String()).To(ContainSubstring("Lifecycle commands:"))
				Expect(stderr.String()).To(BeEmpty())
			})
		})

		Context("when invoked with --version", func() {
			It("prints the current version to stdout and exits successfully", func() {
				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper-version")
				cmd.Env = append(os.Environ(),
					"CCP_MAIN_TEST_HELPER_VERSION=1",
					"CCP_MAIN_TEST_HELPER_VERSION_VALUE=9.8.7",
				)
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				Expect(cmd.Run()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("9.8.7\n"))
				Expect(stderr.String()).To(BeEmpty())
			})
		})

		Context("when invoked with lifecycle help", func() {
			It("routes history help through lifecycle dispatch", func() {
				handled, err := runLifecycleCommand([]string{"history", "--help"})

				Expect(err).NotTo(HaveOccurred())
				Expect(handled).To(BeTrue())
			})
		})

		Context("when audit setup is blocked", func() {
			It("still reaches the installed entrypoint smoke command", func() {
				home := GinkgoT().TempDir()
				Expect(os.WriteFile(home+string(os.PathSeparator)+".config", []byte("block"), 0o644)).To(Succeed())

				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper-smoke")
				cmd.Env = append(os.Environ(),
					"CCP_MAIN_TEST_HELPER_SMOKE=1",
					"HOME="+home,
					"USERPROFILE="+home,
				)
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				Expect(cmd.Run()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("smoke-ok"))
			})
		})

		Context("when exit helpers terminate the process", func() {
			It("prints error text for exitWithErr", func() {
				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper-exit-err")
				cmd.Env = append(os.Environ(), "CCP_MAIN_TEST_HELPER_EXIT_WITH_ERR=1")
				cmd.Stderr = &stderr

				err := cmd.Run()
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
				Expect(exitErr.ExitCode()).To(Equal(7))
				Expect(stderr.String()).To(ContainSubstring("boom"))
			})

			It("exits quietly for exitWithErr with nil", func() {
				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper-exit-nil")
				cmd.Env = append(os.Environ(), "CCP_MAIN_TEST_HELPER_EXIT_WITH_NIL_ERR=1")
				cmd.Stderr = &stderr

				err := cmd.Run()
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
				Expect(exitErr.ExitCode()).To(Equal(9))
				Expect(stderr.String()).To(BeEmpty())
			})
		})
	})

	DescribeTable("classifying lifecycle commands separately from wrapped execution",
		func(args []string, expected bool) {
			Expect(cli.IsManagedArgs(args)).To(Equal(expected))
		},
		Entry("capture command", []string{"capture", "--", "echo", "hi"}, true),
		Entry("init command", []string{"init"}, true),
		Entry("repair command", []string{"repair"}, true),
		Entry("filter command", []string{"filter", "new", "demo"}, true),
		Entry("uninstall command", []string{"uninstall"}, true),
		Entry("pwd execution", []string{"pwd"}, false),
		Entry("echo execution", []string{"echo", "hi"}, false),
		Entry("nil args", nil, false),
	)

	Describe("runLifecycleCommand", func() {
		It("ignores non-managed commands", func() {
			handled, err := runLifecycleCommand([]string{"echo", "hi"})

			Expect(err).NotTo(HaveOccurred())
			Expect(handled).To(BeFalse())
		})

		It("routes filter help through lifecycle dispatch", func() {
			handled, err := runLifecycleCommand([]string{"filter", "--help"})

			Expect(err).NotTo(HaveOccurred())
			Expect(handled).To(BeTrue())
		})
	})
})

func init() {
	runMainUsageHelperIfRequested()
	runMainHelpHelperIfRequested()
	runMainVersionHelperIfRequested()
	runMainSmokeHelperIfRequested()
	runExitWithErrHelperIfRequested()
	runExitWithNilErrHelperIfRequested()
}

func runMainUsageHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER") != "1" {
		return
	}

	os.Args = []string{"ccp"}
	main()
}

func runMainSmokeHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER_SMOKE") != "1" {
		return
	}

	if runtime.GOOS == "windows" {
		os.Args = []string{"ccp", "cmd", "/c", "echo", "smoke-ok"}
	} else {
		os.Args = []string{"ccp", "sh", "-c", "printf 'smoke-ok\n'"}
	}
	main()
	os.Exit(0)
}

func runMainHelpHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER_HELP") != "1" {
		return
	}

	os.Args = []string{"ccp", "--help"}
	main()
	os.Exit(0)
}

func runMainVersionHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER_VERSION") != "1" {
		return
	}

	version.Version = fmt.Sprint(os.Getenv("CCP_MAIN_TEST_HELPER_VERSION_VALUE"))
	os.Args = []string{"ccp", "--version"}
	main()
	os.Exit(0)
}

func runExitWithErrHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER_EXIT_WITH_ERR") != "1" {
		return
	}

	exitWithErr(7, errors.New("boom"))
}

func runExitWithNilErrHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER_EXIT_WITH_NIL_ERR") != "1" {
		return
	}

	exitWithErr(9, nil)
}
