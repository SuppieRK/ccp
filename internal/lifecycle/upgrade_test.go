package lifecycle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go-command-compression-proxy/internal/version"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	newBinaryContent = "new-binary"
	testDownloadURL  = "https://example/a.zip"
	flagVersion      = "--version"
)

var _ = Describe("replaceBinary", func() {
	var (
		tmpDir string
		src    string
		dst    string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		src = filepath.Join(tmpDir, "src")
		dst = filepath.Join(tmpDir, "dst")
	})

	Context("when the source and destination are valid", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(src, []byte(newBinaryContent), 0o755)).To(Succeed())
			Expect(os.WriteFile(dst, []byte("old-binary"), 0o644)).To(Succeed())
		})

		It("replaces the destination binary and applies the source permissions", func() {
			Expect(replaceBinary(dst, src)).To(Succeed())

			b, err := os.ReadFile(dst)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(newBinaryContent))

			info, err := os.Stat(dst)
			Expect(err).NotTo(HaveOccurred())
			if runtime.GOOS == "windows" {
				Expect(info.Mode().IsRegular()).To(BeTrue())
				Expect(info.Mode() & 0o111).To(BeZero())
			} else {
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o755)))
			}

			_, err = os.Stat(dst + ".new")
			Expect(err).To(MatchError(os.ErrNotExist))
		})
	})

	Context("when the source is missing", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(dst, []byte("old-binary"), 0o755)).To(Succeed())
		})

		It("returns an error", func() {
			err := replaceBinary(dst, filepath.Join(tmpDir, "missing-src"))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the destination directory is missing", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(src, []byte(newBinaryContent), 0o755)).To(Succeed())
		})

		It("returns an error", func() {
			err := replaceBinary(filepath.Join(tmpDir, "missing", "dst"), src)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the destination path is a directory", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(src, []byte(newBinaryContent), 0o755)).To(Succeed())
			Expect(os.MkdirAll(dst, 0o755)).To(Succeed())
		})

		It("returns the rename error and keeps the staged file", func() {
			err := replaceBinary(dst, src)
			Expect(err).To(HaveOccurred())
			Expect(dst).To(BeADirectory())
			Expect(dst + ".new").To(BeAnExistingFile())
		})
	})
})

var _ = Describe("releaseAssetName", func() {
	DescribeTable("resolving supported release asset names",
		func(goos string, goarch string, wantAsset string, wantBin string) {
			asset, bin, err := releaseAssetName("1.2.3", goos, goarch)
			Expect(err).NotTo(HaveOccurred())
			Expect(asset).To(Equal(wantAsset))
			Expect(bin).To(Equal(wantBin))
		},
		Entry("linux amd64", "linux", "amd64", "ccp_1.2.3_linux_amd64.zip", "ccp"),
		Entry("windows arm64", "windows", "arm64", "ccp_1.2.3_windows_arm64.zip", "ccp.exe"),
	)

	It("rejects unsupported operating systems", func() {
		_, _, err := releaseAssetName("1.2.3", "plan9", "amd64")
		Expect(err).To(MatchError(ContainSubstring("unsupported os")))
	})

	It("rejects unsupported architectures", func() {
		_, _, err := releaseAssetName("1.2.3", "linux", "386")
		Expect(err).To(MatchError(ContainSubstring("unsupported arch")))
	})
})

var _ = Describe("selectAssetURL", func() {
	It("selects the expected asset url", func() {
		rel := githubRelease{Assets: []githubAssetInfo{{Name: "a.zip", BrowserDownloadURL: testDownloadURL}}}

		got, err := selectAssetURL(rel, "a.zip")

		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(testDownloadURL))
	})

	It("fails when the asset is missing", func() {
		rel := githubRelease{Assets: []githubAssetInfo{{Name: "a.zip", BrowserDownloadURL: testDownloadURL}}}

		_, err := selectAssetURL(rel, "b.zip")

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("selectedUpgradeRepairMode", func() {
	DescribeTable("selecting repair mode from the running version",
		func(currentVersion string, expected repairMode) {
			Expect(selectedUpgradeRepairMode(currentVersion)).To(Equal(expected))
		},
		Entry("older plain version preserves existing filters", "0.5.0", repairModePreserve),
		Entry("pre-cutover patch version preserves existing filters", "0.5.9", repairModePreserve),
		Entry("cutover version rewrites managed state", "0.6.0", repairModeRewrite),
		Entry("newer version rewrites managed state", "1.2.3", repairModeRewrite),
		Entry("v-prefixed versions preserve existing filters", "v0.6.0", repairModePreserve),
		Entry("pre-release versions preserve existing filters", "0.6.0-rc.1", repairModePreserve),
		Entry("whitespace versions preserve existing filters", " 1.2.3 ", repairModePreserve),
		Entry("dev preserves existing filters", "dev", repairModePreserve),
		Entry("invalid version preserves existing filters", "not-a-version", repairModePreserve),
	)
})

var _ = Describe("upgrade helper functions", func() {
	It("uses the default HTTP timeout for release downloads", func() {
		Expect(upgradeHTTPClient.Timeout).To(Equal(30 * time.Second))
	})

	It("rejects latest releases with empty tags", func() {
		prev := upgradeHTTPClient
		upgradeHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusOK, `{"tag_name":""}`), nil
		})}
		DeferCleanup(func() { upgradeHTTPClient = prev })

		_, err := latestReleaseTag(defaultUpgradeRepo)
		Expect(err).To(MatchError("latest release has empty tag_name"))
	})

	DescribeTable("finding checksums for assets",
		func(contents string, asset string, expected string) {
			sum, err := checksumForAsset(contents, asset)
			Expect(err).NotTo(HaveOccurred())
			Expect(sum).To(Equal(expected))
		},
		Entry("matches plain filenames", "abc123  ccp_1.2.3_linux_amd64.zip\n", "ccp_1.2.3_linux_amd64.zip", "abc123"),
		Entry("matches starred checksum entries", "def456 *ccp_1.2.3_linux_amd64.zip\n", "ccp_1.2.3_linux_amd64.zip", "def456"),
		Entry("matches dot-slash checksum entries", "fedcba  ./ccp_1.2.3_linux_amd64.zip\n", "ccp_1.2.3_linux_amd64.zip", "fedcba"),
		Entry("ignores uppercase checksum text differences during later verification", "ABC123  ./ccp_1.2.3_linux_amd64.zip\n", "ccp_1.2.3_linux_amd64.zip", "ABC123"),
	)

	It("returns an error when the archive does not contain the binary", func() {
		tmpDir := GinkgoT().TempDir()
		zipPath := filepath.Join(tmpDir, "asset.zip")
		Expect(os.WriteFile(zipPath, makeZipArchive("other-binary", []byte("x")), 0o644)).To(Succeed())

		zr, err := zip.OpenReader(zipPath)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = zr.Close() })

		_, err = extractBinaryFromZip(zr.File, "ccp", tmpDir)
		Expect(err).To(MatchError(ContainSubstring("binary ccp not found in archive")))
	})

	It("preserves an existing error when a closer also fails", func() {
		baseErr := errors.New("base error")
		closeWithErr(errorCloser{err: errors.New("close error")}, &baseErr)
		Expect(baseErr).To(MatchError("base error"))
	})

	It("captures closer errors when no prior error exists", func() {
		var err error
		closeWithErr(errorCloser{err: errors.New("close error")}, &err)
		Expect(err).To(MatchError("close error"))
	})

	DescribeTable("running installed repair with the expected flag",
		func(mode repairMode, expectedFlag string) {
			if runtime.GOOS == "windows" {
				Skip("repair helper script uses unix sh")
			}

			tmpDir := GinkgoT().TempDir()
			logPath := filepath.Join(tmpDir, "repair.log")
			exePath := filepath.Join(tmpDir, "ccp")
			script := "#!/bin/sh\nprintf '%s %s' \"$1\" \"$2\" >" + shellQuoteArg(logPath) + "\n"
			Expect(os.WriteFile(exePath, []byte(script), 0o755)).To(Succeed())

			Expect(runInstalledRepair(exePath, mode)).To(Succeed())

			body, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("repair " + expectedFlag))
		},
		Entry("preserve mode uses --no", repairModePreserve, "--no"),
		Entry("rewrite mode uses --yes", repairModeRewrite, "--yes"),
	)
})

var _ = Describe("verifyDownloadedAssetChecksum", func() {
	var (
		tmpDir        string
		assetPath     string
		checksumsPath string
		assetName     string
		assetBody     []byte
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		assetName = "ccp_1.2.3_linux_amd64.zip"
		assetBody = makeZipArchive("ccp", []byte(newBinaryContent))
		assetPath = filepath.Join(tmpDir, assetName)
		checksumsPath = filepath.Join(tmpDir, releaseChecksumsAsset)
		Expect(os.WriteFile(assetPath, assetBody, 0o644)).To(Succeed())
	})

	It("accepts checksum files whose hex digest casing differs", func() {
		sum := sha256.Sum256(assetBody)
		Expect(os.WriteFile(checksumsPath, []byte(fmt.Sprintf("%X  ./%s\n", sum[:], assetName)), 0o644)).To(Succeed())

		err := verifyDownloadedAssetChecksum(checksumsPath, assetPath, assetName)

		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("returns underlying checksum verification errors",
		func(setup func(), assertErr func(error)) {
			setup()

			err := verifyDownloadedAssetChecksum(checksumsPath, assetPath, assetName)

			Expect(err).To(HaveOccurred())
			assertErr(err)
		},
		Entry("when the checksum file is missing", func() {}, func(err error) {
			Expect(os.IsNotExist(err)).To(BeTrue())
		}),
		Entry("when the asset is absent from the checksum file", func() {
			Expect(os.WriteFile(checksumsPath, []byte("deadbeef  ./other.zip\n"), 0o644)).To(Succeed())
		}, func(err error) {
			Expect(err.Error()).To(ContainSubstring(`checksum for asset "ccp_1.2.3_linux_amd64.zip" not found`))
		}),
		Entry("when the downloaded asset cannot be hashed", func() {
			Expect(os.WriteFile(checksumsPath, checksumFixtureBody(assetName, assetBody, false), 0o644)).To(Succeed())
			Expect(os.Remove(assetPath)).To(Succeed())
		}, func(err error) {
			Expect(os.IsNotExist(err)).To(BeTrue())
		}),
	)
})

var _ = Describe("upgrade permission helpers", func() {
	DescribeTable("keeps permission changes platform-aware",
		func(osName string, ensure func(string) error, expectExecutable bool) {
			if runtime.GOOS == "windows" && expectExecutable {
				Skip("unix executable bits are not observable on Windows filesystems")
			}

			path := filepath.Join(GinkgoT().TempDir(), "ccp")
			Expect(os.WriteFile(path, []byte("binary"), 0o644)).To(Succeed())

			prevOS := upgradeRuntimeOS
			upgradeRuntimeOS = func() string { return osName }
			DeferCleanup(func() { upgradeRuntimeOS = prevOS })

			Expect(ensure(path)).To(Succeed())

			info, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			if expectExecutable {
				Expect(info.Mode() & 0o111).NotTo(BeZero())
				return
			}
			Expect(info.Mode().IsRegular()).To(BeTrue())
			Expect(info.Mode() & 0o111).To(BeZero())
		},
		Entry("installed binaries stay unchanged on windows", "windows", ensureUpgradeExecutablePermissions, false),
		Entry("extracted binaries stay unchanged on windows", "windows", ensureExecutableIfNeeded, false),
		Entry("installed binaries become executable on unix", "linux", ensureUpgradeExecutablePermissions, true),
		Entry("extracted binaries become executable on unix", "linux", ensureExecutableIfNeeded, true),
	)
})

var _ = Describe("RunUpgrade", func() {
	var (
		tmpDir string
		dest   string
		args   []string
		client *http.Client
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		dest = filepath.Join(tmpDir, "ccp")
		Expect(os.WriteFile(dest, []byte("old"), 0o755)).To(Succeed())
		args = nil
		client = mockUpgradeClient(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), false)

		restore := stubUpgradeRuntimeDeps(
			func() (string, error) { return dest, nil },
			func() string { return "linux" },
			func() string { return "amd64" },
			client,
			func(string, repairMode) error { return nil },
		)
		DeferCleanup(restore)

		prevVersion := version.Version
		version.Version = "1.2.2"
		DeferCleanup(func() { version.Version = prevVersion })
	})

	Context("when upgrading to the latest release", func() {
		It("replaces the existing binary", func() {
			Expect(RunUpgrade(args)).To(Succeed())

			b, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(newBinaryContent))
		})

		It("prints the upgraded asset and version", func() {
			var printed string
			prevPrintf := upgradePrintf
			upgradePrintf = func(format string, args ...any) (int, error) {
				printed = fmt.Sprintf(format, args...)
				return len(printed), nil
			}
			DeferCleanup(func() { upgradePrintf = prevPrintf })

			Expect(RunUpgrade(args)).To(Succeed())
			Expect(printed).To(Equal(fmt.Sprintf("ccp upgrade: replaced %s with %s (%s)\n", dest, "ccp_1.2.3_linux_amd64.zip", "1.2.3")))
		})

		It("cleans up temporary extracted upgrade directories", func() {
			pattern := filepath.Join(os.TempDir(), "ccp-upgrade-*")
			before, err := filepath.Glob(pattern)
			Expect(err).NotTo(HaveOccurred())

			Expect(RunUpgrade(args)).To(Succeed())

			after, err := filepath.Glob(pattern)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(ConsistOf(before))
		})
	})

	Context("when upgrading to a specific version", func() {
		BeforeEach(func() {
			args = []string{flagVersion, "2.0.0"}
			client = mockUpgradeClient(defaultUpgradeRepo, "2.0.0", "linux", "amd64", []byte(newBinaryContent), false)

			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
				func(string, repairMode) error { return nil },
			)
			DeferCleanup(restore)
		})

		It("downloads and installs that version", func() {
			Expect(RunUpgrade(args)).To(Succeed())
		})
	})

	Context("when the requested version is not strict X.Y.Z", func() {
		BeforeEach(func() {
			args = []string{flagVersion, "v1.2.3"}
		})

		It("rejects the upgrade before download", func() {
			err := RunUpgrade(args)
			Expect(err).To(MatchError(ContainSubstring("invalid release version")))

			body, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("old"))
		})
	})

	Context("when the latest release tag is not strict X.Y.Z", func() {
		BeforeEach(func() {
			client = mockUpgradeClient(defaultUpgradeRepo, "v1.2.3", "linux", "amd64", []byte(newBinaryContent), false)
			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
				func(string, repairMode) error { return nil },
			)
			DeferCleanup(restore)
		})

		It("rejects the release tag as invalid", func() {
			err := RunUpgrade(nil)
			Expect(err).To(MatchError(ContainSubstring("invalid release version")))
		})
	})

	Context("when fetching the release fails", func() {
		DescribeTable("keeping the existing binary",
			func(failAPI bool, failDownload bool) {
				restore := stubUpgradeRuntimeDeps(
					func() (string, error) { return dest, nil },
					func() string { return "linux" },
					func() string { return "amd64" },
					mockUpgradeClientWithFailures(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent), failAPI, failDownload),
					func(string, repairMode) error { return nil },
				)
				DeferCleanup(restore)

				err := RunUpgrade(args)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("manual download")))

				b, readErr := os.ReadFile(dest)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(b)).To(Equal("old"))
			},
			Entry("because the API request fails", true, false),
			Entry("because the asset download fails", false, true),
		)
	})

	Context("when the release does not publish checksums", func() {
		BeforeEach(func() {
			args = []string{flagVersion, "1.2.3"}
			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				mockUpgradeClientWithoutChecksums(defaultUpgradeRepo, "1.2.3", "linux", "amd64", []byte(newBinaryContent)),
				func(string, repairMode) error { return nil },
			)
			DeferCleanup(restore)
		})

		It("refuses to install the binary", func() {
			err := RunUpgrade(args)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("checksum")))

			b, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("old"))
		})
	})

	Context("when running on unix", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("unix executable permission bits are not supported on Windows filesystems")
			}

			Expect(os.WriteFile(dest, []byte("old"), 0o644)).To(Succeed())
			args = []string{flagVersion, "1.2.3"}
		})

		It("sets executable permission bits on the installed binary", func() {
			Expect(RunUpgrade(args)).To(Succeed())

			info, err := os.Stat(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode() & 0o111).NotTo(BeZero())
		})
	})

	Context("when determining the executable path fails", func() {
		BeforeEach(func() {
			args = []string{flagVersion, "1.2.3"}
			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return "", errors.New("no executable") },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
				func(string, repairMode) error { return nil },
			)
			DeferCleanup(restore)
		})

		It("returns the path error", func() {
			err := RunUpgrade(args)
			Expect(err).To(MatchError(ContainSubstring("no executable")))
		})
	})

	Context("when repair fails after installing the new binary", func() {
		BeforeEach(func() {
			args = []string{flagVersion, "1.2.3"}
			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
				func(string, repairMode) error { return errors.New("repair failed") },
			)
			DeferCleanup(restore)
		})

		It("keeps the new binary installed and returns a post-upgrade repair error", func() {
			err := RunUpgrade(args)
			Expect(err).To(MatchError(ContainSubstring("repair failed")))
			Expect(err).To(MatchError(ContainSubstring("post-upgrade repair failed after installing the new binary")))
			Expect(err).To(MatchError(ContainSubstring("the new binary remains installed")))
			Expect(err).To(MatchError(ContainSubstring("ccp repair --yes")))

			b, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(newBinaryContent))
		})
	})

	Context("when the removed repo flag is provided", func() {
		BeforeEach(func() {
			args = []string{"--repo", "acme/ccp"}
		})

		It("rejects the flag", func() {
			err := RunUpgrade(args)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when upgrading from 0.6.0 or newer", func() {
		var seenMode repairMode

		BeforeEach(func() {
			args = []string{flagVersion, "1.2.3"}
			seenMode = ""
			version.Version = "0.6.0"
			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
				func(_ string, mode repairMode) error {
					seenMode = mode
					return nil
				},
			)
			DeferCleanup(restore)
		})

		It("runs rewrite repair through the new binary", func() {
			Expect(RunUpgrade(args)).To(Succeed())
			Expect(seenMode).To(Equal(repairModeRewrite))

			b, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(newBinaryContent))
		})
	})

	Context("when upgrading from older than 0.6.0", func() {
		var seenMode repairMode

		BeforeEach(func() {
			args = []string{flagVersion, "1.2.3"}
			seenMode = ""
			version.Version = "0.5.9"
			restore := stubUpgradeRuntimeDeps(
				func() (string, error) { return dest, nil },
				func() string { return "linux" },
				func() string { return "amd64" },
				client,
				func(_ string, mode repairMode) error {
					seenMode = mode
					return nil
				},
			)
			DeferCleanup(restore)
		})

		It("runs preserve repair through the new binary", func() {
			Expect(RunUpgrade(args)).To(Succeed())
			Expect(seenMode).To(Equal(repairModePreserve))
		})
	})
})

func mockUpgradeClient(repo string, tag string, goos string, goarch string, binary []byte, failAPI bool) *http.Client {
	return mockUpgradeClientWithOptions(repo, tag, goos, goarch, binary, failAPI, false, true, false)
}

func mockUpgradeClientWithFailures(repo string, tag string, goos string, goarch string, binary []byte, failAPI bool, failDownload bool) *http.Client {
	return mockUpgradeClientWithOptions(repo, tag, goos, goarch, binary, failAPI, failDownload, true, false)
}

func mockUpgradeClientWithoutChecksums(repo string, tag string, goos string, goarch string, binary []byte) *http.Client {
	return mockUpgradeClientWithOptions(repo, tag, goos, goarch, binary, false, false, false, false)
}

func mockUpgradeClientWithOptions(repo string, tag string, goos string, goarch string, binary []byte, failAPI bool, failDownload bool, includeChecksums bool, mismatchChecksum bool) *http.Client {
	asset := fmt.Sprintf("ccp_%s_%s_%s.zip", tag, goos, goarch)
	zipBody := makeZipArchive("ccp", binary)
	checksumBody := checksumFixtureBody(asset, zipBody, mismatchChecksum)

	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		u := req.URL.String()
		latestURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
		tagURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
		downloadURL := fmt.Sprintf("https://downloads.example/%s/%s", tag, asset)
		checksumURL := fmt.Sprintf("https://downloads.example/%s/%s", tag, releaseChecksumsAsset)

		if failAPI && (u == latestURL || u == tagURL) {
			return jsonHTTPResponse(http.StatusForbidden, `{"message":"forbidden"}`), nil
		}
		switch u {
		case latestURL:
			return jsonHTTPResponse(http.StatusOK, fmt.Sprintf(`{"tag_name":"%s"}`, tag)), nil
		case tagURL:
			assets := fmt.Sprintf(`[{"name":"%s","browser_download_url":"%s"}]`, asset, downloadURL)
			if includeChecksums {
				assets = fmt.Sprintf(`[{"name":"%s","browser_download_url":"%s"},{"name":"%s","browser_download_url":"%s"}]`, asset, downloadURL, releaseChecksumsAsset, checksumURL)
			}
			return jsonHTTPResponse(http.StatusOK, fmt.Sprintf(`{"tag_name":"%s","assets":%s}`, tag, assets)), nil
		case downloadURL:
			if failDownload {
				return jsonHTTPResponse(http.StatusBadGateway, `{"message":"bad gateway"}`), nil
			}
			return bytesHTTPResponse(http.StatusOK, "application/zip", zipBody), nil
		case checksumURL:
			return bytesHTTPResponse(http.StatusOK, "text/plain", checksumBody), nil
		default:
			return jsonHTTPResponse(http.StatusNotFound, `{"message":"not found"}`), nil
		}
	})}
}

func checksumFixtureBody(asset string, zipBody []byte, mismatch bool) []byte {
	sum := sha256.Sum256(zipBody)
	value := fmt.Sprintf("%x", sum[:])
	if mismatch {
		value = strings.Repeat("0", 64)
	}
	return []byte(fmt.Sprintf("%s  ./%s\n", value, asset))
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

type errorCloser struct {
	err error
}

func (e errorCloser) Close() error {
	return e.err
}

func stubUpgradeRuntimeDeps(
	execFn func() (string, error),
	osFn func() string,
	archFn func() string,
	httpClient *http.Client,
	repairFn func(string, repairMode) error,
) func() {
	prevExec := upgradeExecutablePath
	prevOS := upgradeRuntimeOS
	prevArch := upgradeRuntimeArch
	prevHTTP := upgradeHTTPClient
	prevRepair := upgradeRunRepair
	upgradeExecutablePath = execFn
	upgradeRuntimeOS = osFn
	upgradeRuntimeArch = archFn
	upgradeHTTPClient = httpClient
	upgradeRunRepair = repairFn
	return func() {
		upgradeExecutablePath = prevExec
		upgradeRuntimeOS = prevOS
		upgradeRuntimeArch = prevArch
		upgradeHTTPClient = prevHTTP
		upgradeRunRepair = prevRepair
	}
}
