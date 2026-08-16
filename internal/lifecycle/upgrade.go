package lifecycle

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/SuppieRK/cmdshape/internal/product"
	"github.com/SuppieRK/cmdshape/internal/version"
)

const (
	defaultUpgradeRepo    = "SuppieRK/cmdshape"
	releaseChecksumsAsset = "cmdshape_checksums.txt"
	maxReleaseBytes       = 1 << 20
	maxChecksumBytes      = 1 << 20
	maxArchiveBytes       = 128 << 20
	maxUpgradeBinaryBytes = 64 << 20
	maxUpgradeRedirects   = 5
)

const installedExecutableMode os.FileMode = 0o755

type upgradeAssets struct {
	archiveName string
	binaryName  string
	archiveURL  string
	checksumURL string
}

var (
	upgradeExecutablePath = os.Executable
	upgradeReplaceBinary  = replaceBinary
	upgradePrintf         = fmt.Printf
	upgradeHTTPClient     = &http.Client{Timeout: 30 * time.Second}
	upgradeRuntimeOS      = func() string { return runtime.GOOS }
	upgradeRuntimeArch    = func() string { return runtime.GOARCH }
	upgradeRunRepair      = runInstalledRepair
	upgradeValidateStaged = validateStagedBinaryVersion
	upgradeRenameBinary   = os.Rename
	upgradeRemoveBinary   = os.Remove
	upgradeScheduleRemove = scheduleExecutableRemoval
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
		"upgrade cmdshape from GitHub Releases",
		[]string{"cmdshape upgrade [--version <tag>]"},
		"When --version is omitted, the latest release is selected.",
		"Upgrade always resolves releases from the canonical repository.",
		"After replacement, the old binary immediately executes the new binary's rewrite repair path.",
		"Downgrades are rejected and require reinstalling the older version explicitly.",
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

	tag, _, err := resolveUpgradeVersion(repoName, *versionFlag, manualURL)
	if err != nil {
		return err
	}

	assets, err := resolveUpgradeAssets(repoName, tag, manualURL)
	if err != nil {
		return err
	}

	srcPath, cleanup, err := downloadAndExtractUpgradeBinary(
		assets.archiveURL,
		assets.checksumURL,
		releaseChecksumsAsset,
		assets.archiveName,
		assets.binaryName,
	)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return fmt.Errorf("download/verify/extract upgrade asset: %w; manual download: %s", err, manualURL)
	}
	if err := upgradeValidateStaged(srcPath, tag); err != nil {
		return fmt.Errorf("validate staged upgrade binary: %w", err)
	}
	return installUpgradeBinary(srcPath, assets.archiveName, tag)
}

func resolveUpgradeVersion(repoName, requested, manualURL string) (string, version.Semantic, error) {
	tag := requested
	if tag == "" {
		latest, err := latestReleaseTag(repoName)
		if err != nil {
			return "", version.Semantic{}, fmt.Errorf("resolve latest release: %w; manual download: %s", err, manualURL)
		}
		tag = latest
	}
	releaseVersion, ok := version.Parse(tag)
	if !ok {
		return "", version.Semantic{}, fmt.Errorf("invalid release version %q: must be X.Y.Z; manual download: %s", tag, manualURL)
	}
	if err := rejectDowngrade(version.Version, releaseVersion); err != nil {
		return "", version.Semantic{}, err
	}
	return releaseVersion.String(), releaseVersion, nil
}

func resolveUpgradeAssets(repoName, tag, manualURL string) (upgradeAssets, error) {
	archiveName, binaryName, err := releaseAssetName(tag, upgradeRuntimeOS(), upgradeRuntimeArch())
	if err != nil {
		return upgradeAssets{}, err
	}

	rel, err := fetchRelease(fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repoName, url.PathEscape(tag)))
	if err != nil {
		return upgradeAssets{}, fmt.Errorf("resolve release %s: %w; manual download: %s", tag, err, manualURL)
	}
	archiveURL, err := selectAssetURL(rel, archiveName)
	if err != nil {
		return upgradeAssets{}, fmt.Errorf("resolve asset %s: %w; manual download: %s", archiveName, err, manualURL)
	}
	checksumURL, err := selectAssetURL(rel, releaseChecksumsAsset)
	if err != nil {
		return upgradeAssets{}, fmt.Errorf("resolve checksum asset %s: %w; manual download: %s", releaseChecksumsAsset, err, manualURL)
	}
	return upgradeAssets{
		archiveName: archiveName,
		binaryName:  binaryName,
		archiveURL:  archiveURL,
		checksumURL: checksumURL,
	}, nil
}

func installUpgradeBinary(srcPath, assetName, tag string) error {
	exePath, err := upgradeExecutablePath()
	if err != nil {
		return err
	}
	oldBinaryPath, err := installUpgradeReplacement(exePath, srcPath)
	if err != nil {
		return err
	}
	if err := ensureUpgradeExecutablePermissions(exePath); err != nil {
		return err
	}
	repairMode := repairModeRewrite
	repairErr := upgradeRunRepair(exePath, repairMode)
	var cleanupErr error
	if oldBinaryPath != "" {
		if err := upgradeScheduleRemove(oldBinaryPath); err != nil {
			cleanupErr = fmt.Errorf("schedule removal of previous binary %q: %w; the new binary remains installed", oldBinaryPath, err)
		}
	}
	if repairErr != nil {
		return errors.Join(
			fmt.Errorf("post-upgrade repair failed after installing the new binary: %w; the new binary remains installed; rerun `cmdshape repair %s` after fixing the environment", repairErr, repairMode.flag()),
			cleanupErr,
		)
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	return printUpgradeSuccess(exePath, assetName, tag)
}

func installUpgradeReplacement(exePath, srcPath string) (_ string, err error) {
	if upgradeRuntimeOS() != "windows" {
		return "", upgradeReplaceBinary(exePath, srcPath)
	}

	backupPath := backupBinaryPath(exePath)
	if err := upgradeRenameBinary(exePath, backupPath); err != nil {
		return "", fmt.Errorf("move running binary aside: %w", err)
	}
	installed := false
	defer func() {
		if installed {
			return
		}
		removeErr := upgradeRemoveBinary(exePath)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		restoreErr := upgradeRenameBinary(backupPath, exePath)
		err = errors.Join(
			err,
			wrapOptionalError("remove partial Windows binary", removeErr),
			wrapOptionalError("restore previous binary", restoreErr),
		)
	}()
	if err := upgradeReplaceBinary(exePath, srcPath); err != nil {
		return "", fmt.Errorf("install staged Windows binary: %w", err)
	}
	installed = true
	return backupPath, nil
}

func wrapOptionalError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func ensureUpgradeExecutablePermissions(exePath string) error {
	if upgradeRuntimeOS() == "windows" {
		return nil
	}
	return os.Chmod(exePath, installedExecutableMode)
}

func printUpgradeSuccess(exePath, assetName, tag string) error {
	_, err := upgradePrintf("cmdshape upgrade: replaced %s with %s (%s)\n", exePath, assetName, tag)
	return err
}

func rejectDowngrade(currentVersion string, target version.Semantic) error {
	current, ok := version.Parse(currentVersion)
	if !ok {
		return nil
	}
	if target.Less(current) {
		return fmt.Errorf("cmdshape upgrade: refusing downgrade from %s to %s; uninstall cmdshape and install the older version explicitly", current.String(), target.String())
	}
	return nil
}

func latestReleaseTag(repo string) (string, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	rel, err := fetchRelease(endpoint)
	if err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", errors.New("latest release has empty tag_name")
	}
	return rel.TagName, nil
}

func fetchRelease(endpoint string) (rel githubRelease, err error) {
	if err := validateUpgradeURL(endpoint); err != nil {
		return githubRelease{}, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := upgradeClient().Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer closeWithErr(resp.Body, &err)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubRelease{}, fmt.Errorf("github api %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBytes+1))
	if err := decoder.Decode(&rel); err != nil {
		return githubRelease{}, err
	}
	return rel, nil
}

func upgradeClient() *http.Client {
	client := *upgradeHTTPClient
	originalCheck := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxUpgradeRedirects {
			return errors.New("too many upgrade redirects")
		}
		if err := validateUpgradeURL(req.URL.String()); err != nil {
			return err
		}
		if originalCheck != nil {
			return originalCheck(req, via)
		}
		return nil
	}
	return &client
}

func validateUpgradeURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid upgrade URL: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.User != nil {
		return fmt.Errorf("upgrade URL must not contain credentials: %s", raw)
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return fmt.Errorf("upgrade URL must use the default HTTPS port: %s", raw)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("upgrade URL must use HTTPS: %s", raw)
	}
	if host != "github.com" && host != "api.github.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return fmt.Errorf("upgrade URL host is not allowed: %s", host)
	}
	return nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	return body, nil
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

	binary = product.Name
	if goos == "windows" {
		binary += ".exe"
	}
	return fmt.Sprintf("%s_%s_%s_%s.zip", product.Name, tag, goos, goarch), binary, nil
}

func selectAssetURL(rel githubRelease, assetName string) (string, error) {
	selected := ""
	for _, a := range rel.Assets {
		if a.Name == assetName && strings.TrimSpace(a.BrowserDownloadURL) != "" {
			if selected != "" {
				return "", fmt.Errorf("duplicate asset %q", assetName)
			}
			if err := validateUpgradeURL(a.BrowserDownloadURL); err != nil {
				return "", err
			}
			selected = a.BrowserDownloadURL
		}
	}
	if selected != "" {
		return selected, nil
	}
	return "", fmt.Errorf("asset %q not found", assetName)
}

func downloadAndExtractUpgradeBinary(assetURL, checksumURL, checksumsAsset, assetName, binaryName string) (srcPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "cmdshape-upgrade-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	zipPath := filepath.Join(tmpDir, "asset.zip")
	if err := downloadFile(assetURL, zipPath, maxArchiveBytes); err != nil {
		return "", cleanup, err
	}
	checksumsPath := filepath.Join(tmpDir, checksumsAsset)
	if err := downloadFile(checksumURL, checksumsPath, maxChecksumBytes); err != nil {
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
	body, err := readBoundedFile(checksumsPath, maxChecksumBytes)
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
	var match string
	for line := range strings.SplitSeq(contents, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if name != assetName {
			continue
		}
		if len(fields[0]) != sha256.Size*2 {
			return "", fmt.Errorf("checksum for asset %q must contain exactly 64 hex digits", assetName)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return "", fmt.Errorf("checksum for asset %q is not hexadecimal: %w", assetName, err)
		}
		if match != "" {
			return "", fmt.Errorf("duplicate checksum for asset %q", assetName)
		}
		match = fields[0]
	}
	if match != "" {
		return match, nil
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
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadFile(rawURL, dst string, limit int64) (err error) {
	if err := validateUpgradeURL(rawURL); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := upgradeClient().Do(req)
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
	written, err := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if written > limit {
		return fmt.Errorf("download exceeds %d-byte limit", limit)
	}
	return f.Sync()
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

	df, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".new-*")
	if err != nil {
		return err
	}
	tmp := df.Name()
	dfClosed := false
	defer func() {
		if !dfClosed {
			closeWithErr(df, &err)
		}
		if removeErr := os.Remove(tmp); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if _, err := io.Copy(df, sf); err != nil {
		return err
	}
	if err := df.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := df.Sync(); err != nil {
		return err
	}
	if err := df.Close(); err != nil {
		return err
	}
	dfClosed = true

	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	if err := syncUpgradeDirectory(filepath.Dir(dest)); err != nil {
		return err
	}
	return nil
}

func syncUpgradeDirectory(dir string) (err error) {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer closeWithErr(file, &err)
	return file.Sync()
}

func closeWithErr(c io.Closer, retErr *error) {
	if cerr := c.Close(); *retErr == nil && cerr != nil {
		*retErr = cerr
	}
}

func extractBinaryFromZip(files []*zip.File, binaryName, tmpDir string) (string, error) {
	var selected *zip.File
	for _, f := range files {
		if err := validateUpgradeArchiveEntry(f, binaryName); err != nil {
			return "", err
		}
		if selected != nil {
			return "", fmt.Errorf("archive contains duplicate binary entry %q", binaryName)
		}
		selected = f
	}
	if selected == nil {
		return "", fmt.Errorf("binary %s not found in archive", binaryName)
	}
	dstPath := filepath.Join(tmpDir, binaryName)
	if err := copyZipFileToPath(selected, dstPath); err != nil {
		return "", err
	}
	if err := ensureExecutableIfNeeded(dstPath); err != nil {
		return "", err
	}
	return dstPath, nil
}

func validateUpgradeArchiveEntry(f *zip.File, binaryName string) error {
	name := filepath.ToSlash(f.Name)
	if filepath.IsAbs(f.Name) || strings.HasPrefix(name, "/") || strings.Contains(name, "../") || strings.Contains(name, `\`) {
		return fmt.Errorf("unsafe archive entry %q", f.Name)
	}
	if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().Mode().IsRegular() && !f.FileInfo().IsDir()) {
		return fmt.Errorf("archive entry %q is not a regular file", f.Name)
	}
	if !f.FileInfo().Mode().IsRegular() {
		return fmt.Errorf("unexpected archive entry %q", f.Name)
	}
	if name != binaryName {
		return fmt.Errorf("binary %s not found in archive: unexpected archive entry %q", binaryName, f.Name)
	}
	if f.UncompressedSize64 > maxUpgradeBinaryBytes {
		return fmt.Errorf("archive binary exceeds %d-byte limit", maxUpgradeBinaryBytes)
	}
	return nil
}

func copyZipFileToPath(f *zip.File, dstPath string) (err error) {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer closeWithErr(rc, &err)

	df, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, installedExecutableMode)
	if err != nil {
		return err
	}
	defer closeWithErr(df, &err)

	written, err := io.Copy(df, io.LimitReader(rc, maxUpgradeBinaryBytes+1))
	if err != nil {
		return err
	}
	if written != int64(f.UncompressedSize64) || written > maxUpgradeBinaryBytes {
		return errors.New("archive binary size mismatch")
	}
	return df.Sync()
}

func ensureExecutableIfNeeded(path string) error {
	if upgradeRuntimeOS() == "windows" {
		return nil
	}
	return os.Chmod(path, installedExecutableMode)
}

func backupBinaryPath(exePath string) string {
	dir := filepath.Dir(exePath)
	base := filepath.Base(exePath)
	return filepath.Join(dir, fmt.Sprintf("%s.backup.%d", base, time.Now().UnixNano()))
}

func validateStagedBinaryVersion(path, expected string) error {
	cmd := exec.Command(path, "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("run staged --version: %w", err)
	}
	if string(output) != expected+"\n" {
		return fmt.Errorf("staged version %q does not match requested %q followed by one LF", string(output), expected)
	}
	return nil
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
