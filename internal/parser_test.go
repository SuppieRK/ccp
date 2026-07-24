package core

import (
	"github.com/SuppieRK/cmdshape/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseCommandArgs", func() {
	DescribeTable("converts argv into command metadata",
		func(args []string, expected contracts.Command) {
			command, err := ParseCommandArgs(args)

			Expect(err).NotTo(HaveOccurred())
			Expect(command).To(Equal(expected))
		},
		Entry("plain executable with local short flag",
			[]string{"python3", "-m", "pytest", "-q"},
			contracts.Command{
				RawInput: "python3 -m pytest -q",
				Args:     []string{"python3", "-m", "pytest", "-q"},
				Tool:     "python3",
			},
		),
		Entry("executable path is reduced to its basename",
			[]string{"/usr/local/bin/git", "-C", "repo", "status"},
			contracts.Command{
				RawInput: "/usr/local/bin/git -C repo status",
				Args:     []string{"/usr/local/bin/git", "-C", "repo", "status"},
				Tool:     "git",
			},
		),
		Entry("windows style executable path keeps its path-style basename",
			[]string{`C:\Tools\kubectl.exe`, "--context", "dev", "get", "pods"},
			contracts.Command{
				RawInput: `'C:\Tools\kubectl.exe' --context dev get pods`,
				Args:     []string{`C:\Tools\kubectl.exe`, "--context", "dev", "get", "pods"},
				Tool:     "kubectl",
			},
		),
		Entry("gradle wrapper argv resolves to canonical gradle tool",
			[]string{"./gradlew", "test", "--stacktrace"},
			contracts.Command{
				RawInput: "./gradlew test --stacktrace",
				Args:     []string{"./gradlew", "test", "--stacktrace"},
				Tool:     "gradle",
			},
		),
		Entry("maven wrapper windows script resolves to canonical maven tool",
			[]string{`C:\repo\mvnw.cmd`, "-B", "verify"},
			contracts.Command{
				RawInput: `'C:\repo\mvnw.cmd' -B verify`,
				Args:     []string{`C:\repo\mvnw.cmd`, "-B", "verify"},
				Tool:     "mvn",
			},
		),
		Entry("global and local flags are preserved verbatim",
			[]string{"go", "test", "-run", "TestParser", "-count=1", "./internal"},
			contracts.Command{
				RawInput: "go test -run TestParser -count=1 ./internal",
				Args:     []string{"go", "test", "-run", "TestParser", "-count=1", "./internal"},
				Tool:     "go",
			},
		),
		Entry("separator markers are preserved verbatim",
			[]string{"pytest", "-q", "--", "-k", "parser"},
			contracts.Command{
				RawInput: "pytest -q -- -k parser",
				Args:     []string{"pytest", "-q", "--", "-k", "parser"},
				Tool:     "pytest",
			},
		),
		Entry("shell utility wrapper keeps the wrapper executable as tool",
			[]string{"bash", "-lc", "npm test && npm run lint"},
			contracts.Command{
				RawInput: "bash -lc 'npm test && npm run lint'",
				Args:     []string{"bash", "-lc", "npm test && npm run lint"},
				Tool:     "bash",
			},
		),
	)

	It("renders RawInput so argv round-trips through command line parsing", func() {
		args := []string{"cmd", "", "two words", `contains'quote`, `C:\Program Files\demo\main.py`, `semi;colon`}

		command, err := ParseCommandArgs(args)
		Expect(err).NotTo(HaveOccurred())

		reparsed, err := ParseCommandLine(command.RawInput)
		Expect(err).NotTo(HaveOccurred())
		Expect(reparsed.Args).To(Equal(args))
	})

	It("rejects missing command args", func() {
		command, err := ParseCommandArgs(nil)

		Expect(err).To(MatchError("no command provided"))
		Expect(command).To(Equal(contracts.Command{}))
	})

	It("rejects argv with a blank executable token", func() {
		command, err := ParseCommandArgs([]string{"   ", "--flag"})

		Expect(err).To(MatchError("no command provided"))
		Expect(command).To(Equal(contracts.Command{}))
	})
})

var _ = Describe("ParseCommandLine", func() {
	DescribeTable("parses shell-like command lines",
		func(raw string, expected contracts.Command) {
			command, err := ParseCommandLine(raw)

			Expect(err).NotTo(HaveOccurred())
			Expect(command).To(Equal(expected))
		},
		Entry("plain argv",
			"python -m pytest -q",
			contracts.Command{
				RawInput: "python -m pytest -q",
				Args:     []string{"python", "-m", "pytest", "-q"},
				Tool:     "python",
			},
		),
		Entry("global flag before git subcommand",
			"git -C repo status --short",
			contracts.Command{
				RawInput: "git -C repo status --short",
				Args:     []string{"git", "-C", "repo", "status", "--short"},
				Tool:     "git",
			},
		),
		Entry("toolchain selector before cargo subcommand",
			"cargo +stable test --workspace -- --nocapture",
			contracts.Command{
				RawInput: "cargo +stable test --workspace -- --nocapture",
				Args:     []string{"cargo", "+stable", "test", "--workspace", "--", "--nocapture"},
				Tool:     "cargo",
			},
		),
		Entry("kubectl with global long and short flags",
			"kubectl --context dev -n kube-system get pods -l app=api",
			contracts.Command{
				RawInput: "kubectl --context dev -n kube-system get pods -l app=api",
				Args:     []string{"kubectl", "--context", "dev", "-n", "kube-system", "get", "pods", "-l", "app=api"},
				Tool:     "kubectl",
			},
		),
		Entry("docker compose with local long flag",
			"docker compose --project-name demo ps --all",
			contracts.Command{
				RawInput: "docker compose --project-name demo ps --all",
				Args:     []string{"docker", "compose", "--project-name", "demo", "ps", "--all"},
				Tool:     "docker",
			},
		),
		Entry("docker run with mixed short and long flags",
			`docker run --rm -e APP_ENV=dev -v "$PWD:/work" alpine:3.20 sh -lc "echo done"`,
			contracts.Command{
				RawInput: `docker run --rm -e APP_ENV=dev -v "$PWD:/work" alpine:3.20 sh -lc "echo done"`,
				Args:     []string{"docker", "run", "--rm", "-e", "APP_ENV=dev", "-v", "$PWD:/work", "alpine:3.20", "sh", "-lc", "echo done"},
				Tool:     "docker",
			},
		),
		Entry("npm with global prefix flag and separator",
			"npm --prefix web run build -- --watch",
			contracts.Command{
				RawInput: "npm --prefix web run build -- --watch",
				Args:     []string{"npm", "--prefix", "web", "run", "build", "--", "--watch"},
				Tool:     "npm",
			},
		),
		Entry("yarn with top level long flag and script args",
			`yarn --cwd web test --watch --runInBand`,
			contracts.Command{
				RawInput: `yarn --cwd web test --watch --runInBand`,
				Args:     []string{"yarn", "--cwd", "web", "test", "--watch", "--runInBand"},
				Tool:     "yarn",
			},
		),
		Entry("pnpm with recursive short flag and separator",
			`pnpm -r test -- --runInBand`,
			contracts.Command{
				RawInput: `pnpm -r test -- --runInBand`,
				Args:     []string{"pnpm", "-r", "test", "--", "--runInBand"},
				Tool:     "pnpm",
			},
		),
		Entry("gradle wrapper with project property and stacktrace",
			`./gradlew -p app test --stacktrace`,
			contracts.Command{
				RawInput: `./gradlew -p app test --stacktrace`,
				Args:     []string{"./gradlew", "-p", "app", "test", "--stacktrace"},
				Tool:     "gradle",
			},
		),
		Entry("maven wrapper with global batch mode and local goal flags",
			`./mvnw -B test -DskipITs -pl service`,
			contracts.Command{
				RawInput: `./mvnw -B test -DskipITs -pl service`,
				Args:     []string{"./mvnw", "-B", "test", "-DskipITs", "-pl", "service"},
				Tool:     "mvn",
			},
		),
		Entry("maven debug wrapper resolves to canonical maven tool",
			`./mvnwDebug test -DskipITs`,
			contracts.Command{
				RawInput: `./mvnwDebug test -DskipITs`,
				Args:     []string{"./mvnwDebug", "test", "-DskipITs"},
				Tool:     "mvn",
			},
		),
		Entry("pytest with long flag assignment and marker expression",
			`pytest -q --maxfail=1 -m "not slow and not e2e"`,
			contracts.Command{
				RawInput: `pytest -q --maxfail=1 -m "not slow and not e2e"`,
				Args:     []string{"pytest", "-q", "--maxfail=1", "-m", "not slow and not e2e"},
				Tool:     "pytest",
			},
		),
		Entry("go test with run filter and count flag",
			`go test ./... -run TestParser -count=1`,
			contracts.Command{
				RawInput: `go test ./... -run TestParser -count=1`,
				Args:     []string{"go", "test", "./...", "-run", "TestParser", "-count=1"},
				Tool:     "go",
			},
		),
		Entry("git commit with short flag and quoted message",
			`git commit -m "parser: preserve quoted args" --no-verify`,
			contracts.Command{
				RawInput: `git commit -m "parser: preserve quoted args" --no-verify`,
				Args:     []string{"git", "commit", "-m", "parser: preserve quoted args", "--no-verify"},
				Tool:     "git",
			},
		),
		Entry("ssh command with nested remote shell snippet",
			`ssh prod "journalctl -u app -n 100 --no-pager"`,
			contracts.Command{
				RawInput: `ssh prod "journalctl -u app -n 100 --no-pager"`,
				Args:     []string{"ssh", "prod", "journalctl -u app -n 100 --no-pager"},
				Tool:     "ssh",
			},
		),
		Entry("bash wrapper with chained commands inside one shell snippet",
			`bash -lc "npm test && npm run lint"`,
			contracts.Command{
				RawInput: `bash -lc "npm test && npm run lint"`,
				Args:     []string{"bash", "-lc", "npm test && npm run lint"},
				Tool:     "bash",
			},
		),
		Entry("sh wrapper with pipeline inside one shell snippet",
			`sh -c "grep foo app.log | sort -u"`,
			contracts.Command{
				RawInput: `sh -c "grep foo app.log | sort -u"`,
				Args:     []string{"sh", "-c", "grep foo app.log | sort -u"},
				Tool:     "sh",
			},
		),
		Entry("direct chained command tokens keep the first executable as tool",
			`git status && make test`,
			contracts.Command{
				RawInput: `git status && make test`,
				Args:     []string{"git", "status", "&&", "make", "test"},
				Tool:     "git",
			},
		),
		Entry("direct pipeline tokens keep the first executable as tool",
			`grep foo app.log | sort -u`,
			contracts.Command{
				RawInput: `grep foo app.log | sort -u`,
				Args:     []string{"grep", "foo", "app.log", "|", "sort", "-u"},
				Tool:     "grep",
			},
		),
		Entry("find with exec placeholder preserves shell metacharacters as argv tokens",
			`find . -name "*.go" -exec grep -n parser {} \;`,
			contracts.Command{
				RawInput: `find . -name "*.go" -exec grep -n parser {} \;`,
				Args:     []string{"find", ".", "-name", "*.go", "-exec", "grep", "-n", "parser", "{}", ";"},
				Tool:     "find",
			},
		),
		Entry("env style prefixes are preserved as plain argv",
			`env FOO=bar BAR=baz python -m pytest`,
			contracts.Command{
				RawInput: `env FOO=bar BAR=baz python -m pytest`,
				Args:     []string{"env", "FOO=bar", "BAR=baz", "python", "-m", "pytest"},
				Tool:     "env",
			},
		),
		Entry("ripgrep with quoted glob and long option",
			`rg -n --glob "*.go" main`,
			contracts.Command{
				RawInput: `rg -n --glob "*.go" main`,
				Args:     []string{"rg", "-n", "--glob", "*.go", "main"},
				Tool:     "rg",
			},
		),
		Entry("double quoted arguments",
			`npx "my tool" --flag="two words"`,
			contracts.Command{
				RawInput: `npx "my tool" --flag="two words"`,
				Args:     []string{"npx", "my tool", "--flag=two words"},
				Tool:     "npx",
			},
		),
		Entry("single quoted arguments",
			`python -c 'print("hi")'`,
			contracts.Command{
				RawInput: `python -c 'print("hi")'`,
				Args:     []string{"python", "-c", `print("hi")`},
				Tool:     "python",
			},
		),
		Entry("mixed quoting inside double quotes",
			`sh -lc "printf '%s\n' 'hello world'"`,
			contracts.Command{
				RawInput: `sh -lc "printf '%s\n' 'hello world'"`,
				Args:     []string{"sh", "-lc", `printf '%s\n' 'hello world'`},
				Tool:     "sh",
			},
		),
		Entry("double quotes preserve backslashes before non-special characters",
			`cmd "path\zvalue"`,
			contracts.Command{
				RawInput: `cmd "path\zvalue"`,
				Args:     []string{"cmd", `path\zvalue`},
				Tool:     "cmd",
			},
		),
		Entry("escaped spaces",
			`python path\ with\ spaces.py`,
			contracts.Command{
				RawInput: `python path\ with\ spaces.py`,
				Args:     []string{"python", "path with spaces.py"},
				Tool:     "python",
			},
		),
		Entry("escaped tabs outside quotes are treated as literal characters",
			`python path\	to\	file.py`,
			contracts.Command{
				RawInput: `python path\	to\	file.py`,
				Args:     []string{"python", "path\tto\tfile.py"},
				Tool:     "python",
			},
		),
		Entry("escaped quote inside double quotes",
			`python -c "print(\"hello\")"`,
			contracts.Command{
				RawInput: `python -c "print(\"hello\")"`,
				Args:     []string{"python", "-c", `print("hello")`},
				Tool:     "python",
			},
		),
		Entry("single quoted empty argument is preserved",
			`cmd '' --flag`,
			contracts.Command{
				RawInput: `cmd '' --flag`,
				Args:     []string{"cmd", "", "--flag"},
				Tool:     "cmd",
			},
		),
		Entry("double quoted argument can contain leading and trailing spaces",
			`cmd "  padded value  " --flag`,
			contracts.Command{
				RawInput: `cmd "  padded value  " --flag`,
				Args:     []string{"cmd", "  padded value  ", "--flag"},
				Tool:     "cmd",
			},
		),
		Entry("empty quoted argument is preserved",
			`cmd "" --flag`,
			contracts.Command{
				RawInput: `cmd "" --flag`,
				Args:     []string{"cmd", "", "--flag"},
				Tool:     "cmd",
			},
		),
		Entry("leading and repeated whitespace is ignored between args",
			"   go   test   ./...   ",
			contracts.Command{
				RawInput: "go   test   ./...",
				Args:     []string{"go", "test", "./..."},
				Tool:     "go",
			},
		),
		Entry("windows style path with escaped backslashes in quotes",
			`python "C:\\Program Files\\demo\\main.py" --verbose`,
			contracts.Command{
				RawInput: `python "C:\\Program Files\\demo\\main.py" --verbose`,
				Args:     []string{"python", `C:\Program Files\demo\main.py`, "--verbose"},
				Tool:     "python",
			},
		),
		Entry("unquoted windows paths preserve backslashes",
			`cmd --path C:\temp\file.txt`,
			contracts.Command{
				RawInput: `cmd --path C:\temp\file.txt`,
				Args:     []string{"cmd", "--path", `C:\temp\file.txt`},
				Tool:     "cmd",
			},
		),
	)

	DescribeTable("rejects malformed command lines",
		func(raw, message string) {
			command, err := ParseCommandLine(raw)

			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(command).To(Equal(contracts.Command{}))
		},
		Entry("blank input", "   ", "no command provided"),
		Entry("only quoted whitespace", `""   ""`, "no command provided"),
		Entry("unterminated single quote", `python -c 'print("hi")`, "unterminated single quote"),
		Entry("unterminated double quote", `python -c "print('hi')`, "unterminated double quote"),
		Entry("unterminated escape", `python trailing\`, "unterminated escape sequence"),
	)
})

var _ = Describe("splitCommandLine", func() {
	It("rejects input that tokenizes to no args", func() {
		args, err := splitCommandLine("")

		Expect(err).To(MatchError("no command provided"))
		Expect(args).To(BeNil())
	})

	DescribeTable("starts the expected unquoted parser mode",
		func(input rune, expectSingle bool, expectDouble bool, expectEscaping bool) {
			state := newCommandLineSplitState()

			state.consumeUnquoted(input)

			Expect(state.inSingle).To(Equal(expectSingle))
			Expect(state.inDouble).To(Equal(expectDouble))
			Expect(state.escaping).To(Equal(expectEscaping))
			Expect(state.tokenStarted).To(BeTrue())
			Expect(state.current.String()).To(BeEmpty())
		},
		Entry("single quote starts single-quoted mode", '\'', true, false, false),
		Entry("double quote starts double-quoted mode", '"', false, true, false),
		Entry("backslash starts escaping mode", '\\', false, false, true),
	)
})

var _ = Describe("renderCommandArgs", func() {
	It("returns an empty string for empty argv", func() {
		Expect(renderCommandArgs(nil)).To(BeEmpty())
	})
})
