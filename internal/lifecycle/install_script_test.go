package lifecycle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("install script", func() {
	var scriptPath string

	BeforeEach(func() {
		var err error
		scriptPath, err = filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("rejecting explicit versions that are not exact semantic versions",
		func(version string) {
			workspace := GinkgoT().TempDir()
			binDir := filepath.Join(workspace, "bin")
			Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())
			writeExecutable(filepath.Join(binDir, "uname"), `#!/bin/sh
case "$1" in
  -m) printf 'x86_64\n' ;;
  *) printf 'Linux\n' ;;
esac
`)
			writeExecutable(filepath.Join(binDir, "curl"), "#!/bin/sh\nexit 99\n")
			writeExecutable(filepath.Join(binDir, "unzip"), "#!/bin/sh\nexit 99\n")

			result := runInstallScript(scriptPath, workspace, map[string]string{
				"VERSION": version,
				"HOME":    filepath.Join(workspace, "home"),
				"PATH":    testPATH(binDir, os.Getenv("PATH")),
			})

			Expect(result.exitCode).NotTo(BeZero())
			Expect(result.stderr).To(ContainSubstring("release version must be exact semantic version (X.Y.Z): " + version))
			Expect(result.stderr).NotTo(ContainSubstring("missing required command"))
		},
		Entry("leading v prefix", "v1.2.3"),
		Entry("missing patch component", "1.2"),
		Entry("prerelease suffix", "1.2.3-beta.1"),
	)

	It("downloads a release archive and invokes install on the resolved asset", func() {
		workspace := GinkgoT().TempDir()
		binDir := filepath.Join(workspace, "bin")
		assetPath, checksumPath := makeInstallFixtures(workspace, "1.2.3")
		installLog := filepath.Join(workspace, "install.log")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())

		writeExecutable(filepath.Join(binDir, "uname"), `#!/bin/sh
case "$1" in
  -m) printf 'x86_64\n' ;;
  *) printf 'Linux\n' ;;
esac
`)
		writeExecutable(filepath.Join(binDir, "curl"), fmt.Sprintf(`#!/bin/sh
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) url="$1"; shift ;;
  esac
done
case "$url" in
  *ccp_checksums.txt) cp %s "$out" ;;
  *) cp %s "$out" ;;
esac
`, shellQuoteArg(checksumPath), shellQuoteArg(assetPath)))
		writeExecutable(filepath.Join(binDir, "install"), fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" > %s
exit 0
`, shellQuoteArg(installLog)))

		result := runInstallScript(scriptPath, workspace, map[string]string{
			"VERSION": "1.2.3",
			"HOME":    filepath.Join(workspace, "home"),
			"PATH":    testPATH(binDir, os.Getenv("PATH")),
		})

		Expect(result.exitCode).To(BeZero(), result.stderr)
		Expect(result.stdout).To(ContainSubstring("Downloading https://github.com/SuppieRK/ccp/releases/download/1.2.3/ccp_1.2.3_linux_amd64.zip"))
		Expect(result.stdout).To(ContainSubstring("Installed ccp 1.2.3 to "))

		body, err := os.ReadFile(installLog)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("-m 0755"))
		Expect(string(body)).To(ContainSubstring("/ccp"))
	})

	Describe("sourced helper behavior", func() {
		var functionPrefix string

		BeforeEach(func() {
			body, err := os.ReadFile(scriptPath)
			Expect(err).NotTo(HaveOccurred())
			functionPrefix = installScriptFunctionPrefix(string(body))
		})

		DescribeTable("validating release versions",
			func(input, expected string, wantSuccess bool) {
				result := runInstallScriptSnippet(functionPrefix, fmt.Sprintf("result=$(validate_release_version %s)\nprintf 'status=%%s value=%%s\\n' \"$?\" \"$result\"\n", shellQuoteArg(input)), nil)

				if wantSuccess {
					Expect(result.exitCode).To(BeZero(), result.stderr)
					Expect(result.stdout).To(ContainSubstring("status=0 value=" + expected))
					return
				}
				Expect(result.exitCode).NotTo(BeZero())
			},
			Entry("accepts exact semantic versions", "1.2.3", "1.2.3", true),
			Entry("rejects versions with v prefix", "v1.2.3", "", false),
			Entry("rejects prerelease versions", "1.2.3-rc1", "", false),
		)

		DescribeTable("comparing versions against the repair cutoff",
			func(input string, expectedExit int) {
				result := runInstallScriptSnippet(functionPrefix, fmt.Sprintf("version_lt_cutoff %s\n", shellQuoteArg(input)), nil)
				Expect(result.exitCode).To(Equal(expectedExit), result.stderr)
			},
			Entry("treats older versions as below cutoff", "0.5.0", 0),
			Entry("treats equal versions as not below cutoff", "0.5.1", 1),
			Entry("treats newer versions as not below cutoff", "0.5.2", 1),
		)

		It("appends a PATH export only once", func() {
			workspace := GinkgoT().TempDir()
			profilePath := filepath.Join(workspace, ".profile")

			result := runInstallScriptSnippet(functionPrefix, fmt.Sprintf("append_path_export_once %s %s\nappend_path_export_once %s %s\ncat %s\n", shellQuoteArg(profilePath), shellQuoteArg("/tmp/ccp-bin"), shellQuoteArg(profilePath), shellQuoteArg("/tmp/ccp-bin"), shellQuoteArg(profilePath)), nil)

			Expect(result.exitCode).To(BeZero(), result.stderr)
			Expect(strings.Count(result.stdout, "# added by ccp installer")).To(Equal(1))
			Expect(strings.Count(result.stdout, `export PATH="$PATH:/tmp/ccp-bin"`)).To(Equal(1))
		})
	})
})

type shellRunResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runInstallScript(scriptPath, workdir string, env map[string]string) shellRunResult {
	cmd := exec.Command("sh", scriptPath)
	cmd.Dir = workdir
	cmd.Env = withEnv(env)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = exitErr.ExitCode()
		} else {
			Fail(fmt.Sprintf("run install script: %v stderr=%s", err, stderr.String()))
		}
	}
	return shellRunResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func runInstallScriptSnippet(prefix, snippet string, env map[string]string) shellRunResult {
	workspace := GinkgoT().TempDir()
	script := prefix + "\n" + snippet
	scriptPath := filepath.Join(workspace, "snippet.sh")
	Expect(os.WriteFile(scriptPath, []byte(script), 0o755)).To(Succeed())
	return runInstallScript(scriptPath, workspace, env)
}

func installScriptFunctionPrefix(body string) string {
	const marker = "OS=\"$(uname -s | tr '[:upper:]' '[:lower:]')\""
	before, _, found := strings.Cut(body, marker)
	Expect(found).To(BeTrue())
	return before
}

func makeInstallFixtures(root, version string) (string, string) {
	assetName := fmt.Sprintf("ccp_%s_linux_amd64.zip", version)
	assetPath := filepath.Join(root, assetName)
	file, err := os.Create(assetPath)
	Expect(err).NotTo(HaveOccurred())
	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create("ccp")
	Expect(err).NotTo(HaveOccurred())
	_, err = entry.Write([]byte("#!/bin/sh\necho ccp\n"))
	Expect(err).NotTo(HaveOccurred())
	Expect(zipWriter.Close()).To(Succeed())
	Expect(file.Close()).To(Succeed())

	checksumPath := filepath.Join(root, "ccp_checksums.txt")
	sum := sha256.Sum256(mustReadFile(assetPath))
	Expect(os.WriteFile(checksumPath, fmt.Appendf(nil, "%x  ./%s\n", sum, assetName), 0o644)).To(Succeed())
	return assetPath, checksumPath
}

func mustReadFile(path string) []byte {
	body, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	return body
}

func withEnv(overrides map[string]string) []string {
	base := os.Environ()
	for key, value := range overrides {
		prefix := key + "="
		replaced := false
		for i, entry := range base {
			if strings.HasPrefix(entry, prefix) {
				base[i] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			base = append(base, prefix+value)
		}
	}
	return base
}

func writeExecutable(path, body string) {
	Expect(os.WriteFile(path, []byte(body), 0o755)).To(Succeed())
}

func testPATH(prefix, current string) string {
	if current == "" {
		return prefix
	}
	if runtime.GOOS == "windows" {
		return filepath.ToSlash(prefix) + ":" + current
	}
	return prefix + string(os.PathListSeparator) + current
}
