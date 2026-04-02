package lifecycle

import (
	"archive/zip"
	"crypto/sha256"
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

	"go-command-compression-proxy/internal/version"
)

const (
	defaultUpgradeRepo    = "SuppieRK/ccp"
	releaseChecksumsAsset = "ccp_checksums.txt"
	upgradeRepairCutover  = "0.6.0"
)

var (
	upgradeExecutablePath = os.Executable
	upgradeReplaceBinary  = replaceBinary
	upgradePrintf         = fmt.Printf
	upgradeHTTPClient     = &http.Client{Timeout: 30 * time.Second}
	upgradeRuntimeOS      = func() string { return runtime.GOOS }
	upgradeRuntimeArch    = func() string { return runtime.GOARCH }
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
	versionFlag := fs.String("version", "", "specific release tag (for example 1.2.3)")
	setLifecycleUsage(
		fs,
		"upgrade ccp from GitHub Releases",
		[]string{"ccp upgrade [--version <tag>]"},
		"When --version is omitted, the latest release is selected.",
		"Upgrade always resolves releases from the canonical repository.",
		"After replacement, the old binary immediately executes the new binary's repair path.",
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

	tag := strings.TrimSpace(*versionFlag)
	if tag == "" {
		tag, err = latestReleaseTag(repoName)
		if err != nil {
			return fmt.Errorf("resolve latest release: %w; manual download: %s", err, manualURL)
		}
	}
	releaseVersion, ok := version.Parse(tag)
	if !ok {
		return fmt.Errorf("invalid release version %q: must be X.Y.Z; manual download: %s", tag, manualURL)
	}
	tag = releaseVersion.String()

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
	checksumURL, err := selectAssetURL(rel, releaseChecksumsAsset)
	if err != nil {
		return fmt.Errorf("resolve checksum asset %s: %w; manual download: %s", releaseChecksumsAsset, err, manualURL)
	}

	srcPath, cleanup, err := downloadAndExtractUpgradeBinary(assetURL, checksumURL, assetName, binaryName)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return fmt.Errorf("download/verify/extract upgrade asset: %w; manual download: %s", err, manualURL)
	}

	return installUpgradeBinary(srcPath, assetName, tag)
}

func installUpgradeBinary(srcPath, assetName, tag string) error {
	exePath, err := upgradeExecutablePath()
	if err != nil {
		return err
	}
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
	restoreBackup = false
	repairMode := selectedUpgradeRepairMode(version.Version)
	if err := upgradeRunRepair(exePath, repairMode); err != nil {
		return fmt.Errorf("post-upgrade repair failed after installing the new binary: %w; the new binary remains installed; rerun `ccp repair %s` after fixing the environment", err, repairMode.flag())
	}
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

func selectedUpgradeRepairMode(currentVersion string) repairMode {
	current, ok := version.Parse(currentVersion)
	if !ok {
		return repairModePreserve
	}
	cutover, ok := version.Parse(upgradeRepairCutover)
	if !ok || current.Less(cutover) {
		return repairModePreserve
	}
	return repairModeRewrite
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

func downloadAndExtractUpgradeBinary(assetURL, checksumURL, assetName, binaryName string) (srcPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "ccp-upgrade-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	zipPath := filepath.Join(tmpDir, "asset.zip")
	if err := downloadFile(assetURL, zipPath); err != nil {
		return "", cleanup, err
	}
	checksumsPath := filepath.Join(tmpDir, releaseChecksumsAsset)
	if err := downloadFile(checksumURL, checksumsPath); err != nil {
		return "", cleanup, err
	}
	if err := verifyDownloadedAssetChecksum(checksumsPath, zipPath, assetName); err != nil {
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

func verifyDownloadedAssetChecksum(checksumsPath, assetPath, assetName string) error {
	body, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	expected, err := checksumForAsset(string(body), assetName)
	if err != nil {
		return err
	}
	actual, err := fileSHA256(assetPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func checksumForAsset(contents, assetName string) (string, error) {
	for line := range strings.SplitSeq(contents, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if filepath.Base(name) != assetName {
			continue
		}
		return fields[0], nil
	}
	return "", fmt.Errorf("checksum for asset %q not found", assetName)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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

func runInstalledRepair(exePath string, mode repairMode) error {
	cmd := exec.Command(exePath, "repair", mode.flag())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (m repairMode) flag() string {
	if m == repairModeRewrite {
		return "--yes"
	}
	return "--no"
}
