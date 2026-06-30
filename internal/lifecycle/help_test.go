package lifecycle

import (
	"bytes"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("lifecycle help", func() {
	type helpCase struct {
		command string
		parts   []string
	}

	captureOutput := func(fn func() error) (string, string) {
		stdoutOrig := os.Stdout
		orig := os.Stderr
		stdoutR, stdoutW, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		stderrR, stderrW, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		os.Stdout = stdoutW
		os.Stderr = stderrW
		DeferCleanup(func() {
			os.Stdout = stdoutOrig
			os.Stderr = orig
		})

		Expect(fn()).To(Succeed())
		Expect(stdoutW.Close()).To(Succeed())
		Expect(stderrW.Close()).To(Succeed())

		var stdoutBuf, stderrBuf bytes.Buffer
		_, err = io.Copy(&stdoutBuf, stdoutR)
		Expect(err).NotTo(HaveOccurred())
		_, err = io.Copy(&stderrBuf, stderrR)
		Expect(err).NotTo(HaveOccurred())
		Expect(stdoutR.Close()).To(Succeed())
		Expect(stderrR.Close()).To(Succeed())
		return stdoutBuf.String(), stderrBuf.String()
	}

	captureStderr := func(fn func() error) string {
		_, stderr := captureOutput(fn)
		return stderr
	}

	DescribeTable("rendering help output",
		func(tc helpCase) {
			out := captureStderr(func() error {
				switch tc.command {
				case "capture":
					return RunCapture([]string{"--help"})
				case "init":
					return RunInit([]string{"--help"})
				case "gain":
					return RunGain([]string{"--help"}, "")
				case "history":
					return RunHistory([]string{"--help"}, "")
				case "filter-root":
					return RunFilter([]string{"--help"})
				case "filter":
					return RunFilter([]string{"new", "--help"})
				case "filter-prompt":
					return RunFilter([]string{"prompt", "--help"})
				case "filter-status":
					return RunFilter([]string{"status", "--help"})
				case "repair":
					return RunRepair([]string{"--help"})
				case "upgrade":
					return RunUpgrade([]string{"--help"})
				case "uninstall":
					return RunUninstall([]string{"--help"})
				default:
					Fail("unknown lifecycle help command: " + tc.command)
					return nil
				}
			})
			for _, part := range tc.parts {
				Expect(out).To(ContainSubstring(part))
			}
		},
		Entry("capture", helpCase{
			command: "capture",
			parts: []string{
				"ccp capture - capture native stdout/stderr and replay CCP output for local filter iteration",
				"Usage:",
				"Flags:",
				"Notes:",
				"--dir",
				"stdout.txt",
				"stderr.txt",
				"output.txt",
			},
		}),
		Entry("init", helpCase{
			command: "init",
			parts: []string{
				"ccp init - install or update supported agent integrations",
				"Usage:",
				"Flags:",
				"Notes:",
				"--tools",
				"~/.config/ccp/filters",
			},
		}),
		Entry("gain", helpCase{
			command: "gain",
			parts: []string{
				"ccp gain - show token savings history",
				"Usage:",
				"Flags:",
				"Notes:",
				"--period",
				"--format",
				"--table",
				"Run ccp gain after install or init to verify savings on real work.",
			},
		}),
		Entry("history", helpCase{
			command: "history",
			parts: []string{
				"ccp history - show recorded command history",
				"Usage:",
				"Flags:",
				"Notes:",
				"--since",
				"--tool",
			},
		}),
		Entry("filter root", helpCase{
			command: "filter-root",
			parts: []string{
				"ccp filter - YAML filter authoring and inspection helpers",
				"Usage:",
				"Flags:",
				"Notes:",
				"ccp filter <subcommand> [args...]",
				"subcommands: new, performance, prompt, status",
				"agents creating or improving filters should start with 'ccp filter prompt [name]'",
			},
		}),
		Entry("filter new", helpCase{
			command: "filter",
			parts: []string{
				"ccp filter new - generate a commented YAML scaffold for a new filter",
				"Usage:",
				"Flags:",
				"Notes:",
				"agents should prefer 'ccp filter prompt <name>' first",
				"./.ccp/filters/<name>.yaml",
				".mappings.yaml",
			},
		}),
		Entry("filter prompt", helpCase{
			command: "filter-prompt",
			parts: []string{
				"ccp filter prompt - print an embedded agent prompt for creating or improving filters",
				"Usage:",
				"Flags:",
				"Notes:",
				"ccp filter prompt [name]",
				"embedded in the ccp binary",
			},
		}),
		Entry("filter status", helpCase{
			command: "filter-status",
			parts: []string{
				"ccp filter status - show active, overridden, and broken filter registrations",
				"Usage:",
				"Flags:",
				"Notes:",
				"status shows all discovered rows from the current filter sources.",
			},
		}),
		Entry("upgrade", helpCase{
			command: "upgrade",
			parts: []string{
				"ccp upgrade - upgrade ccp from GitHub Releases",
				"Usage:",
				"Flags:",
				"Notes:",
				"--version",
				"rewrite repair",
				"downgrades are rejected",
			},
		}),
		Entry("repair", helpCase{
			command: "repair",
			parts: []string{
				"ccp repair - rewrite managed CCP home state to canonical shipped content",
				"Usage:",
				"Flags:",
				"Notes:",
				"--yes",
				"--no",
				"~/.config/ccp",
				"current-repository CCP migrations",
				"without mutating repository files",
			},
		}),
		Entry("uninstall", helpCase{
			command: "uninstall",
			parts: []string{
				"ccp uninstall - remove ccp integrations or fully uninstall ccp",
				"Usage:",
				"Flags:",
				"Notes:",
				"--tools",
				"complete uninstall",
				"removing the executable",
			},
		}),
	)

	DescribeTable("help requests do not emit report output",
		func(command string) {
			stdout, stderr := captureOutput(func() error {
				switch command {
				case "gain":
					return RunGain([]string{"--help"}, "")
				case "history":
					return RunHistory([]string{"--help"}, "")
				default:
					Fail("unknown lifecycle help command: " + command)
					return nil
				}
			})

			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(ContainSubstring("Usage:"))
		},
		Entry("gain", "gain"),
		Entry("history", "history"),
	)
})
