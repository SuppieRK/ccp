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
			Expect(os.WriteFile(dst, []byte("old-binary"), 0o755)).To(Succeed())
		})

		It("replaces the destination binary", func() {
			Expect(replaceBinary(dst, src)).To(Succeed())

			b, err := os.ReadFile(dst)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(newBinaryContent))
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
			func(string) error { return nil },
		)
		DeferCleanup(restore)

		origPrint := upgradePrintf
		upgradePrintf = func(format string, args ...any) (int, error) { return 0, nil }
		DeferCleanup(func() { upgradePrintf = origPrint })
	})

	Context("when upgrading to the latest release", func() {
		It("replaces the existing binary", func() {
			Expect(RunUpgrade(args)).To(Succeed())

			b, err := os.ReadFile(dest)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(newBinaryContent))
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
				func(string) error { return nil },
			)
			DeferCleanup(restore)
		})

		It("downloads and installs that version", func() {
			Expect(RunUpgrade(args)).To(Succeed())
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
					func(string) error { return nil },
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
				func(string) error { return nil },
			)
			DeferCleanup(restore)
		})

		It("returns the path error", func() {
			err := RunUpgrade(args)
			Expect(err).To(MatchError(ContainSubstring("no executable")))
		})
	})

	Context("when printing progress fails", func() {
		BeforeEach(func() {
			args = []string{flagVersion, "1.2.3"}
			origPrint := upgradePrintf
			upgradePrintf = func(format string, args ...any) (int, error) { return 0, errors.New("print failed") }
			DeferCleanup(func() { upgradePrintf = origPrint })
		})

		It("returns the print error", func() {
			err := RunUpgrade(args)
			Expect(err).To(MatchError(ContainSubstring("print failed")))
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
				func(string) error { return errors.New("repair failed") },
			)
			DeferCleanup(restore)
		})

		It("restores the previous binary and returns an error", func() {
			err := RunUpgrade(args)
			Expect(err).To(MatchError(ContainSubstring("repair failed")))
			Expect(err).To(MatchError(ContainSubstring("restored previous binary")))

			b, readErr := os.ReadFile(dest)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("old"))
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
})

func mockUpgradeClient(repo string, tag string, goos string, goarch string, binary []byte, failAPI bool) *http.Client {
	return mockUpgradeClientWithFailures(repo, tag, goos, goarch, binary, failAPI, false)
}

func mockUpgradeClientWithFailures(repo string, tag string, goos string, goarch string, binary []byte, failAPI bool, failDownload bool) *http.Client {
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
	repairFn func(string) error,
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
