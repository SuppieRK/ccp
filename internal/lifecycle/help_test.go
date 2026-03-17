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
				case "filter":
					return RunFilter([]string{"new", "--help"})
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
		Entry("filter new", helpCase{
			command: "filter",
			parts: []string{
				"ccp filter new - generate a commented YAML scaffold for a new filter",
				"Usage:",
				"Flags:",
				"Notes:",
				"./.ccp/filters/<name>.yaml",
				".mappings.yaml",
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
				"~/.config/ccp",
			},
		}),
		Entry("uninstall", helpCase{
			command: "uninstall",
			parts: []string{
				"ccp uninstall - remove supported agent integrations",
				"Usage:",
				"Flags:",
				"Notes:",
				"--tools",
				"auto-detection from the current repository",
			},
		}),
	)
})
