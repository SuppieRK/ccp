package lifecycle

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	newBinaryContent = "new-binary"
	testDownloadURL  = "https://example/a.zip"
	errWriteDestFmt  = "write dest: %v"
	flagVersion      = "--version"
)

func TestReplaceBinary(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.WriteFile(src, []byte(newBinaryContent), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	if err := replaceBinary(dst, src); err != nil {
		t.Fatalf("replace binary: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(b) != newBinaryContent {
		t.Fatalf("unexpected dst content: %q", string(b))
	}
}

func TestReplaceBinaryErrorsForMissingSource(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "dst")
	if err := os.WriteFile(dst, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	err := replaceBinary(dst, filepath.Join(tmp, "missing-src"))
	if err == nil {
		t.Fatal("expected replaceBinary error for missing source")
	}
}

func TestReplaceBinaryErrorsWhenDestinationDirMissing(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.WriteFile(src, []byte(newBinaryContent), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	err := replaceBinary(filepath.Join(tmp, "missing", "dst"), src)
	if err == nil {
		t.Fatal("expected replaceBinary error for missing destination directory")
	}
}

func TestReleaseAssetNameSupportedTargets(t *testing.T) {
	cases := []struct {
		name      string
		goos      string
		goarch    string
		wantAsset string
		wantBin   string
	}{
		{
			name:      "linux-amd64",
			goos:      "linux",
			goarch:    "amd64",
			wantAsset: "ccp_1.2.3_linux_amd64.zip",
			wantBin:   "ccp",
		},
		{
			name:      "windows-arm64",
			goos:      "windows",
			goarch:    "arm64",
			wantAsset: "ccp_1.2.3_windows_arm64.zip",
			wantBin:   "ccp.exe",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asset, bin, err := releaseAssetName("1.2.3", tc.goos, tc.goarch)
			if err != nil {
				t.Fatalf("releaseAssetName error: %v", err)
			}
			if asset != tc.wantAsset {
				t.Fatalf("asset=%q", asset)
			}
			if bin != tc.wantBin {
				t.Fatalf("bin=%q", bin)
			}
		})
	}
}

func TestReleaseAssetNameUnsupportedOS(t *testing.T) {
	_, _, err := releaseAssetName("1.2.3", "plan9", "amd64")
	if err == nil || !strings.Contains(err.Error(), "unsupported os") {
		t.Fatalf("expected unsupported os error, got %v", err)
	}
}

func TestReleaseAssetNameUnsupportedArch(t *testing.T) {
	_, _, err := releaseAssetName("1.2.3", "linux", "386")
	if err == nil || !strings.Contains(err.Error(), "unsupported arch") {
		t.Fatalf("expected unsupported arch error, got %v", err)
	}
}

func TestSelectAssetURL(t *testing.T) {
	rel := githubRelease{Assets: []githubAssetInfo{{Name: "a.zip", BrowserDownloadURL: testDownloadURL}}}
	got, err := selectAssetURL(rel, "a.zip")
	if err != nil {
		t.Fatalf("selectAssetURL error: %v", err)
	}
	if got != testDownloadURL {
		t.Fatalf("got=%q", got)
	}
}

func TestSelectAssetURLMissing(t *testing.T) {
	rel := githubRelease{Assets: []githubAssetInfo{{Name: "a.zip", BrowserDownloadURL: testDownloadURL}}}
	_, err := selectAssetURL(rel, "b.zip")
	if err == nil {
		t.Fatal("expected missing asset error")
	}
}

func TestRunUpgradeLatestSuccess(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "ccp")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf(errWriteDestFmt, err)
	}
	client := mockUpgradeClient(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), false)

	restore := stubUpgradeRuntimeDeps(
		func() (string, error) { return dest, nil },
		func() string { return "linux" },
		func() string { return "amd64" },
		client,
	)
	defer restore()

	muteUpgradePrint(t)

	if err := RunUpgrade([]string{}); err != nil {
		t.Fatalf("RunUpgrade error: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(b) != newBinaryContent {
		t.Fatalf("dest content=%q", string(b))
	}
}

func TestRunUpgradeSpecificVersion(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "ccp")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf(errWriteDestFmt, err)
	}
	client := mockUpgradeClient(defaultUpgradeRepo, "2.0.0", "linux", "amd64", []byte(newBinaryContent), false)

	restore := stubUpgradeRuntimeDeps(
		func() (string, error) { return dest, nil },
		func() string { return "linux" },
		func() string { return "amd64" },
		client,
	)
	defer restore()

	muteUpgradePrint(t)

	if err := RunUpgrade([]string{flagVersion, "2.0.0"}); err != nil {
		t.Fatalf("RunUpgrade error: %v", err)
	}
}

func TestRunUpgradeKeepsBinaryOnAPIAndDownloadError(t *testing.T) {
	cases := []struct {
		name         string
		failAPI      bool
		failDownload bool
	}{
		{name: "api-error", failAPI: true},
		{name: "download-error", failDownload: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			dest := filepath.Join(tmp, "ccp")
			if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
				t.Fatalf(errWriteDestFmt, err)
			}
			client := mockUpgradeClientWithFailures(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), tc.failAPI, tc.failDownload)

			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
			)
			defer restore()

			err := RunUpgrade([]string{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "manual download") {
				t.Fatalf("expected manual download hint, got %v", err)
			}
			b, readErr := os.ReadFile(dest)
			if readErr != nil {
				t.Fatalf("read dest: %v", readErr)
			}
			if string(b) != "old" {
				t.Fatalf("dest changed to %q", string(b))
			}
		})
	}
}

func TestRunUpgradeSetsExecutablePermissionOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable permission bits are not supported on Windows filesystems")
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "ccp")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatalf(errWriteDestFmt, err)
	}
	client := mockUpgradeClient(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), false)

	restore := stubUpgradeRuntimeDeps(
		func() (string, error) { return dest, nil },
		func() string { return "linux" },
		func() string { return "amd64" },
		client,
	)
	defer restore()

	muteUpgradePrint(t)

	if err := RunUpgrade([]string{flagVersion, "1.2.3"}); err != nil {
		t.Fatalf("RunUpgrade error: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected executable mode, got %v", info.Mode().Perm())
	}
}

func TestRunUpgradeReturnsExecutableError(t *testing.T) {
	restore := stubUpgradeRuntimeDeps(
		func() (string, error) { return "", errors.New("no executable") },
		func() string { return "linux" },
		func() string { return "amd64" },
		mockUpgradeClient(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), false),
	)
	defer restore()

	err := RunUpgrade([]string{flagVersion, "1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "no executable") {
		t.Fatalf("expected executable error, got %v", err)
	}
}

func TestRunUpgradeReturnsPrintError(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "ccp")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatalf(errWriteDestFmt, err)
	}
	client := mockUpgradeClient(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), false)

	restore := stubUpgradeRuntimeDeps(
		func() (string, error) { return dest, nil },
		func() string { return "linux" },
		func() string { return "amd64" },
		client,
	)
	defer restore()

	origPrint := upgradePrintf
	upgradePrintf = func(format string, args ...any) (int, error) { return 0, errors.New("print failed") }
	defer func() { upgradePrintf = origPrint }()

	err := RunUpgrade([]string{flagVersion, "1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "print failed") {
		t.Fatalf("expected print error, got %v", err)
	}
}

func muteUpgradePrint(t *testing.T) {
	t.Helper()
	origPrint := upgradePrintf
	upgradePrintf = func(format string, args ...any) (int, error) { return 0, nil }
	t.Cleanup(func() { upgradePrintf = origPrint })
}

func TestRunUpgradeRejectsRepoFlag(t *testing.T) {
	err := RunUpgrade([]string{"--repo", "acme/ccp"})
	if err == nil {
		t.Fatal("expected error for unsupported --repo flag")
	}
}

func mockUpgradeClient(repo, tag, goos, goarch string, binary []byte, failAPI bool) *http.Client {
	return mockUpgradeClientWithFailures(repo, tag, goos, goarch, binary, failAPI, false)
}

func mockUpgradeClientWithFailures(repo, tag, goos, goarch string, binary []byte, failAPI, failDownload bool) *http.Client {
	asset := fmt.Sprintf("ccp_%s_%s_%s.zip", tag, goos, goarch)
	zipBody := makeZipArchive("ccp", binary)

	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		u := req.URL.String()
		latestURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
		tagURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
		downloadURL := fmt.Sprintf("https://downloads.example/%s/%s", tag, asset)

		if failAPI && (u == latestURL || u == tagURL) {
			return jsonHTTPResponse(http.StatusForbidden, `{"message":"forbidden"}`), nil
		}
		switch u {
		case latestURL:
			return jsonHTTPResponse(http.StatusOK, fmt.Sprintf(`{"tag_name":"%s"}`, tag)), nil
		case tagURL:
			return jsonHTTPResponse(http.StatusOK, fmt.Sprintf(`{"tag_name":"%s","assets":[{"name":"%s","browser_download_url":"%s"}]}`, tag, asset, downloadURL)), nil
		case downloadURL:
			if failDownload {
				return jsonHTTPResponse(http.StatusBadGateway, `{"message":"bad gateway"}`), nil
			}
			return bytesHTTPResponse(http.StatusOK, "application/zip", zipBody), nil
		default:
			return jsonHTTPResponse(http.StatusNotFound, `{"message":"not found"}`), nil
		}
	})}
}

func makeZipArchive(name string, content []byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entry, _ := zw.Create(name)
	_, _ = entry.Write(content)
	_ = zw.Close()
	return buf.Bytes()
}

func jsonHTTPResponse(status int, payload string) *http.Response {
	return bytesHTTPResponse(status, "application/json", []byte(payload))
}

func bytesHTTPResponse(status int, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func stubUpgradeRuntimeDeps(
	execFn func() (string, error),
	osFn func() string,
	archFn func() string,
	httpClient *http.Client,
) func() {
	prevExec := upgradeExecutablePath
	prevOS := upgradeRuntimeOS
	prevArch := upgradeRuntimeArch
	prevHTTP := upgradeHTTPClient
	upgradeExecutablePath = execFn
	upgradeRuntimeOS = osFn
	upgradeRuntimeArch = archFn
	upgradeHTTPClient = httpClient
	return func() {
		upgradeExecutablePath = prevExec
		upgradeRuntimeOS = prevOS
		upgradeRuntimeArch = prevArch
		upgradeHTTPClient = prevHTTP
	}
}
