package lifecycle

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultUpgradeRepo = "SuppieRK/ccp"
)

var (
	upgradeExecutablePath = os.Executable
	upgradeReplaceBinary  = replaceBinary
	upgradePrintf         = fmt.Printf
	upgradeHTTPClient     = &http.Client{Timeout: 30 * time.Second}
	upgradeRuntimeOS      = func() string { return runtime.GOOS }
	upgradeRuntimeArch    = func() string { return runtime.GOARCH }
	upgradeInstalledVer   = currentInstalledVersion
	upgradeRunRepair      = runInstalledRepair
)

type githubRelease struct {
	TagName string            `json:"tag_name"`
	Assets  []githubAssetInfo `json:"assets"`
}

type githubAssetInfo struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func RunUpgrade(args []string) error {
	fs := newLifecycleFlagSet("upgrade")
	version := fs.String("version", "", "specific release tag (for example 1.2.3)")
	setLifecycleUsage(
		fs,
		"upgrade ccp from GitHub Releases",
		[]string{"ccp upgrade [--version <tag>]"},
		"When --version is omitted, the latest release is selected.",
		"Upgrade always resolves releases from the canonical repository.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	repoName := defaultUpgradeRepo
	manualURL := fmt.Sprintf("https://github.com/%s/releases", repoName)

	tag := strings.TrimSpace(*version)
	if tag == "" {
		tag, err = latestReleaseTag(repoName)
		if err != nil {
			return fmt.Errorf("resolve latest release: %w; manual download: %s", err, manualURL)
		}
	}

	assetName, binaryName, err := releaseAssetName(tag, upgradeRuntimeOS(), upgradeRuntimeArch())
	if err != nil {
		return err
	}

	rel, err := fetchRelease(fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repoName, url.PathEscape(tag)))
	if err != nil {
		return fmt.Errorf("resolve release %s: %w; manual download: %s", tag, err, manualURL)
	}
	assetURL, err := selectAssetURL(rel, assetName)
	if err != nil {
		return fmt.Errorf("resolve asset %s: %w; manual download: %s", assetName, err, manualURL)
	}

	srcPath, cleanup, err := downloadAndExtractUpgradeBinary(assetURL, binaryName)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return fmt.Errorf("download/extract upgrade asset: %w; manual download: %s", err, manualURL)
	}

	return installUpgradeBinary(srcPath, assetName, tag)
}

func installUpgradeBinary(srcPath, assetName, tag string) error {
	exePath, err := upgradeExecutablePath()
	if err != nil {
		return err
	}
	installedVersion, _ := upgradeInstalledVer(exePath)
	backupPath, err := backupBinaryPath(exePath)
	if err != nil {
		return err
	}
	if err := upgradeReplaceBinary(backupPath, exePath); err != nil {
		return fmt.Errorf("backup existing binary: %w", err)
	}
	restoreBackup := true
	defer func() {
		if restoreBackup {
			_ = upgradeReplaceBinary(exePath, backupPath)
		}
		_ = os.Remove(backupPath)
	}()

	if err := upgradeReplaceBinary(exePath, srcPath); err != nil {
		return err
	}
	if err := ensureUpgradeExecutablePermissions(exePath); err != nil {
		return err
	}
	if shouldRunLegacyRepair(installedVersion) {
		if err := upgradeRunRepair(exePath); err != nil {
			return fmt.Errorf("run repair after upgrade: %w; restored previous binary; recommend running ccp restore", err)
		}
	}
	restoreBackup = false
	return printUpgradeSuccess(exePath, assetName, tag)
}

func ensureUpgradeExecutablePermissions(exePath string) error {
	if upgradeRuntimeOS() == "windows" {
		return nil
	}
	return os.Chmod(exePath, 0o755)
}

func printUpgradeSuccess(exePath, assetName, tag string) error {
	_, err := upgradePrintf("ccp upgrade: replaced %s with %s (%s)\n", exePath, assetName, tag)
	return err
}

func latestReleaseTag(repo string) (string, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	rel, err := fetchRelease(endpoint)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return "", errors.New("latest release has empty tag_name")
	}
	return rel.TagName, nil
}

func fetchRelease(endpoint string) (rel githubRelease, err error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer closeWithErr(resp.Body, &err)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubRelease{}, fmt.Errorf("github api %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return githubRelease{}, err
	}
	return rel, nil
}

func releaseAssetName(tag, goos, goarch string) (asset string, binary string, err error) {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", "", fmt.Errorf("unsupported os: %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", "", fmt.Errorf("unsupported arch: %s", goarch)
	}

	binary = "ccp"
	if goos == "windows" {
		binary = "ccp.exe"
	}
	return fmt.Sprintf("ccp_%s_%s_%s.zip", tag, goos, goarch), binary, nil
}

func selectAssetURL(rel githubRelease, assetName string) (string, error) {
	for _, a := range rel.Assets {
		if a.Name == assetName && strings.TrimSpace(a.BrowserDownloadURL) != "" {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("asset %q not found", assetName)
}

func downloadAndExtractUpgradeBinary(assetURL, binaryName string) (srcPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "ccp-upgrade-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	zipPath := filepath.Join(tmpDir, "asset.zip")
	if err := downloadFile(assetURL, zipPath); err != nil {
		return "", cleanup, err
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", cleanup, err
	}
	dstPath, err := extractBinaryFromZip(zr.File, binaryName, tmpDir)
	closeErr := zr.Close()
	if err != nil {
		return "", cleanup, err
	}
	if closeErr != nil {
		return "", cleanup, closeErr
	}
	return dstPath, cleanup, nil
}

func downloadFile(url, dst string) (err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer closeWithErr(resp.Body, &err)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("download returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer closeWithErr(f, &err)
	_, err = io.Copy(f, resp.Body)
	return err
}

func replaceBinary(dest, src string) (err error) {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer closeWithErr(sf, &err)

	info, err := sf.Stat()
	if err != nil {
		return err
	}

	tmp := dest + ".new"
	df, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	dfClosed := false
	defer func() {
		if !dfClosed {
			closeWithErr(df, &err)
		}
	}()
	if _, err := io.Copy(df, sf); err != nil {
		return err
	}
	if err := df.Close(); err != nil {
		return err
	}
	dfClosed = true

	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	if err := os.Chmod(dest, info.Mode()); err != nil {
		return err
	}
	if err := os.Remove(filepath.Clean(tmp)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func closeWithErr(c io.Closer, retErr *error) {
	if cerr := c.Close(); *retErr == nil && cerr != nil {
		*retErr = cerr
	}
}

func extractBinaryFromZip(files []*zip.File, binaryName, tmpDir string) (string, error) {
	for _, f := range files {
		if filepath.Base(f.Name) != binaryName {
			continue
		}
		dstPath := filepath.Join(tmpDir, binaryName)
		if err := copyZipFileToPath(f, dstPath); err != nil {
			return "", err
		}
		if err := ensureExecutableIfNeeded(dstPath); err != nil {
			return "", err
		}
		return dstPath, nil
	}
	return "", fmt.Errorf("binary %s not found in archive", binaryName)
}

func copyZipFileToPath(f *zip.File, dstPath string) (err error) {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer closeWithErr(rc, &err)

	df, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer closeWithErr(df, &err)

	_, err = io.Copy(df, rc)
	return err
}

func ensureExecutableIfNeeded(path string) error {
	if upgradeRuntimeOS() == "windows" {
		return nil
	}
	return os.Chmod(path, 0o755)
}

func backupBinaryPath(exePath string) (string, error) {
	dir := filepath.Dir(exePath)
	base := filepath.Base(exePath)
	return filepath.Join(dir, fmt.Sprintf("%s.backup.%d", base, time.Now().UnixNano())), nil
}

func runInstalledRepair(exePath string) error {
	cmd := exec.Command(exePath, "repair", "--yes")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
