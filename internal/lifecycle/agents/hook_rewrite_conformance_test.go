package agents

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type hookRewriteConformanceCase struct {
	name     string
	command  string
	expected string
}

var hookRewriteConformanceCases = []hookRewriteConformanceCase{
	{name: "simple external", command: "git status", expected: "ccp git status"},
	{name: "safe and chain", command: "git status && ls", expected: "ccp git status && ccp ls"},
	{name: "safe or and semicolon chain", command: "git status || ls; date", expected: "ccp git status || ccp ls ; ccp date"},
	{name: "quoted and operator", command: `git commit -m "a && b"`, expected: `ccp git commit -m "a && b"`},
	{name: "quoted pipe", command: `git log --format='a|b'`, expected: `ccp git log --format='a|b'`},
	{name: "escaped quote", command: `git commit -m "say \"hi\""`, expected: `ccp git commit -m "say \"hi\""`},
	{name: "leading assignments", command: `FOO=1 BAR='two words' git status`, expected: `FOO=1 BAR='two words' ccp git status`},
	{name: "env and sudo prefixes", command: `env -i FOO=1 sudo -u root git status`, expected: `env -i FOO=1 sudo -u root ccp git status`},
	{name: "sudo and env prefixes", command: `sudo --user=root env FOO=1 git status`, expected: `sudo --user=root env FOO=1 ccp git status`},
	{name: "already prefixed", command: "ccp git status", expected: "ccp git status"},
	{name: "mixed prefixed chain", command: "ccp git status && ls", expected: "ccp git status && ccp ls"},
	{name: "builtin cd", command: "cd /tmp && git status", expected: "cd /tmp && git status"},
	{name: "builtin export", command: "export FOO=1", expected: "export FOO=1"},
	{name: "builtin source", command: "source ./env.sh", expected: "source ./env.sh"},
	{name: "builtin set", command: "set -e", expected: "set -e"},
	{name: "builtin echo", command: "echo hello", expected: "echo hello"},
	{name: "if control syntax", command: "if git status; then ls; fi", expected: "if git status; then ls; fi"},
	{name: "loop control syntax", command: "for f in *; do ls \"$f\"; done", expected: "for f in *; do ls \"$f\"; done"},
	{name: "shell function", command: "f() { git status; }; f", expected: "f() { git status; }; f"},
	{name: "heredoc", command: "cat <<EOF", expected: "cat <<EOF"},
	{name: "here string", command: "cat <<<value", expected: "cat <<<value"},
	{name: "subshell", command: "(git status)", expected: "(git status)"},
	{name: "command substitution", command: "git add $(pwd)", expected: "git add $(pwd)"},
	{name: "backtick substitution", command: "git add " + string(rune(96)) + "pwd" + string(rune(96)), expected: "git add " + string(rune(96)) + "pwd" + string(rune(96))},
	{name: "arithmetic", command: "git show $((1+1))", expected: "git show $((1+1))"},
	{name: "parameter expansion", command: "git add ${HOME}", expected: "git add ${HOME}"},
	{name: "output redirect", command: "git status >out", expected: "git status >out"},
	{name: "input redirect", command: "git apply <patch", expected: "git apply <patch"},
	{name: "redirection only", command: ">out", expected: ">out"},
	{name: "grep pipeline", command: "git status | grep M", expected: "git status | grep M"},
	{name: "head pipeline", command: "git log | head", expected: "git log | head"},
	{name: "tail pipe stderr", command: "git log |& tail", expected: "git log |& tail"},
	{name: "xargs pipeline", command: "find . -type f | xargs wc", expected: "find . -type f | xargs wc"},
	{name: "find exec", command: "find . -exec ls {} ;", expected: "find . -exec ls {} ;"},
	{name: "xargs nested execution", command: "xargs git status", expected: "xargs git status"},
	{name: "nested shell", command: `sh -c "git status"`, expected: `sh -c "git status"`},
	{name: "single ampersand", command: "git status &", expected: "git status &"},
	{name: "trailing operator", command: "git status &&", expected: "git status &&"},
	{name: "incomplete single quote", command: "git commit -m 'oops", expected: "git commit -m 'oops"},
	{name: "incomplete double quote", command: `git commit -m "oops`, expected: `git commit -m "oops`},
	{name: "unsupported sudo option", command: "sudo -Z git status", expected: "sudo -Z git status"},
	{name: "newline control", command: "git status\nls", expected: "git status\nls"},
}

var _ = ginkgo.Describe("hook rewrite conformance", ginkgo.Label("live-smoke"), func() {
	ginkgo.It("classifies every Bash hook command with the shared corpus", func() {
		for _, testCase := range hookRewriteConformanceCases {
			var input bytes.Buffer
			encoder := json.NewEncoder(&input)
			encoder.SetEscapeHTML(false)
			err := encoder.Encode(map[string]any{
				"tool_input": map[string]any{"command": testCase.command},
			})
			Expect(err).NotTo(HaveOccurred(), testCase.name)
			result := runHookScript(
				ginkgo.GinkgoT(),
				"ccp-rewrite.sh",
				"ccp-rewrite.log",
				bashRewriteHookScriptContent("conformance", "ccp-rewrite.log"),
				input.String(),
				true,
			)
			Expect(result.exitCode).To(Equal(0), testCase.name, result.stderr)
			if testCase.expected == testCase.command {
				Expect(strings.TrimSpace(result.stdout)).To(BeEmpty(), testCase.name)
				continue
			}
			Expect(decodeClaudeHookOutput(ginkgo.GinkgoT(), result.stdout)).To(Equal(testCase.expected), testCase.name)
		}
	})

	ginkgo.It("classifies the JavaScript plugin identically with the shared corpus", func() {
		if runtime.GOOS == "windows" {
			ginkgo.Skip("JavaScript plugin execution conformance is covered on Unix")
		}
		node, err := exec.LookPath("node")
		if err != nil {
			ginkgo.Skip("node is not available")
		}
		dir := ginkgo.GinkgoT().TempDir()
		pluginPath := filepath.Join(dir, "plugin.mjs")
		runnerPath := filepath.Join(dir, "runner.mjs")
		Expect(os.WriteFile(pluginPath, []byte(managedBashRewritePluginContent()), 0o600)).To(Succeed())
		runner := `import factory from "./plugin.mjs";
const plugin = await factory();
const output = {args: {command: process.argv[2]}};
await plugin["tool.execute.before"]({tool: "bash"}, output);
process.stdout.write(JSON.stringify(output.args.command));
`
		Expect(os.WriteFile(runnerPath, []byte(runner), 0o600)).To(Succeed())

		for _, testCase := range hookRewriteConformanceCases {
			cmd := exec.Command(node, runnerPath, testCase.command)
			cmd.Dir = dir
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			Expect(cmd.Run()).To(Succeed(), testCase.name, stderr.String())
			var got string
			Expect(json.Unmarshal(stdout.Bytes(), &got)).To(Succeed(), testCase.name)
			Expect(got).To(Equal(testCase.expected), testCase.name)
		}
	})
})
