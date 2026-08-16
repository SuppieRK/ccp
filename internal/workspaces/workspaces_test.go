package workspaces

import (
	"errors"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	bolt "go.etcd.io/bbolt"
)

var _ = Describe("workspace registry", func() {
	It("uses the documented Bolt write timeout", func() {
		Expect(writeTimeout).To(Equal(100 * time.Millisecond))
	})

	It("uses ~/.config/cmdshape/workspaces.db by default", func() {
		home := GinkgoT().TempDir()
		restore := WithTestConfig(home, nil)
		DeferCleanup(restore)

		path, err := DefaultPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(filepath.Join(home, ".config", "cmdshape", "workspaces.db")))
	})

	It("returns the home directory lookup error", func() {
		prev := userHomeDir
		userHomeDir = func() (string, error) {
			return "", errors.New("no home")
		}
		DeferCleanup(func() {
			userHomeDir = prev
		})

		_, err := DefaultPath()
		Expect(err).To(MatchError("no home"))
	})

	It("returns an empty list for a missing registry", func() {
		entries, err := ListPath(filepath.Join(GinkgoT().TempDir(), "missing.db"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("normalizes paths and upserts unique workspaces", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")
		timestamps := []time.Time{
			time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 20, 12, 5, 0, 0, time.UTC),
		}
		index := 0
		restore := WithTestConfig(base, func() time.Time {
			ts := timestamps[index]
			index++
			return ts
		})
		DeferCleanup(restore)

		cwd := filepath.Join(base, "repo", ".", "subdir")
		metricsPath := filepath.Join(base, "repo", ".cmdshape", "gain.db")

		Expect(UpsertPath(registryPath, cwd, metricsPath)).To(Succeed())
		Expect(UpsertPath(registryPath, filepath.Join(base, "repo", "subdir"), metricsPath)).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].CWD).To(Equal(filepath.Join(base, "repo", "subdir")))
		Expect(entries[0].MetricsPath).To(Equal(metricsPath))
		Expect(entries[0].FirstSeenAt).To(Equal(timestamps[0]))
		Expect(entries[0].LastSeenAt).To(Equal(timestamps[1]))
	})

	It("lists workspaces sorted by cwd", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")

		Expect(UpsertPath(registryPath, filepath.Join(base, "z-repo"), filepath.Join(base, "z-repo", ".cmdshape", "gain.db"))).To(Succeed())
		Expect(UpsertPath(registryPath, filepath.Join(base, "a-repo"), filepath.Join(base, "a-repo", ".cmdshape", "gain.db"))).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
		Expect(entries[0].CWD).To(Equal(filepath.Join(base, "a-repo")))
		Expect(entries[1].CWD).To(Equal(filepath.Join(base, "z-repo")))
	})

	It("deletes a registered workspace by cwd", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")
		Expect(UpsertPath(registryPath, filepath.Join(base, "repo"), filepath.Join(base, "repo", ".cmdshape", "gain.db"))).To(Succeed())

		Expect(DeletePath(registryPath, filepath.Join(base, "repo"))).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("supports the default-path helpers and blank path noops", func() {
		home := GinkgoT().TempDir()
		restore := WithTestConfig(home, func() time.Time {
			return time.Date(2026, 3, 20, 12, 30, 0, 0, time.UTC)
		})
		DeferCleanup(restore)
		registryPath, err := DefaultPath()
		Expect(err).NotTo(HaveOccurred())

		Expect(UpsertPath(registryPath, filepath.Join(home, "repo"), filepath.Join(home, "repo", ".cmdshape", "gain.db"))).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))

		Expect(DeletePath(registryPath, filepath.Join(home, "repo"))).To(Succeed())
		entries, err = ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())

		Expect(UpsertPath("", filepath.Join(home, "repo"), filepath.Join(home, "repo", ".cmdshape", "gain.db"))).To(Succeed())
		Expect(DeletePath("", filepath.Join(home, "repo"))).To(Succeed())
	})

	It("treats deleting from a missing registry as a noop", func() {
		Expect(DeletePath(filepath.Join(GinkgoT().TempDir(), "missing.db"), filepath.Join(GinkgoT().TempDir(), "repo"))).To(Succeed())
	})

	It("rejects empty workspace and metrics paths", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")

		Expect(UpsertPath(registryPath, "", filepath.Join(base, "repo", ".cmdshape", "gain.db"))).To(MatchError(ContainSubstring("path must not be empty")))
		Expect(DeletePath(registryPath, "")).To(MatchError(ContainSubstring("path must not be empty")))
	})

	It("keeps an existing metrics path when upserting a repo-only entry", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")
		cwd := filepath.Join(base, "repo")
		metricsPath := filepath.Join(base, "repo", ".cmdshape", "gain.db")

		Expect(UpsertPath(registryPath, cwd, metricsPath)).To(Succeed())
		Expect(UpsertPath(registryPath, cwd, "")).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].MetricsPath).To(Equal(metricsPath))
	})

	It("returns an error when a stored workspace entry is malformed", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")

		db, err := bolt.Open(registryPath, 0o600, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(workspacesBucket)
			if err != nil {
				return err
			}
			return b.Put([]byte(filepath.Join(base, "repo")), []byte("{not-json"))
		})).To(Succeed())
		Expect(db.Close()).To(Succeed())

		_, err = ListPath(registryPath)
		Expect(err).To(HaveOccurred())
	})

	It("treats an existing registry without the workspaces bucket as empty", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")

		db, err := bolt.Open(registryPath, 0o600, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Close()).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})
})

var _ = Describe("workspace helper functions", func() {
	DescribeTable("normalizes workspace paths deterministically",
		func(input string, expected string, wantErr bool) {
			got, err := normalizePath(input)
			if wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(expected))
		},
		Entry("rejects empty paths after trimming", "   ", "", true),
		Entry("returns cleaned absolute paths", "./testdata/../repo", filepath.Clean(func() string {
			abs, err := filepath.Abs("./repo")
			Expect(err).NotTo(HaveOccurred())
			return abs
		}()), false),
	)

	DescribeTable("normalizes optional metrics paths deterministically",
		func(input string, expected string, wantErr bool) {
			got, err := normalizeOptionalPath(input)
			if wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(expected))
		},
		Entry("keeps blank optional paths empty", "   ", "", false),
		Entry("returns cleaned absolute optional paths", "./testdata/../gain.db", filepath.Clean(func() string {
			abs, err := filepath.Abs("./gain.db")
			Expect(err).NotTo(HaveOccurred())
			return abs
		}()), false),
	)

	DescribeTable("normalizes upsert inputs before touching the registry",
		func(path, cwd, metricsPath string, expected normalizedWorkspaceInput, expectedOK bool, wantErr bool) {
			got, ok, err := normalizeUpsertPathInput(path, cwd, metricsPath)
			if wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(Equal(expectedOK))
			Expect(got).To(Equal(expected))
		},
		Entry("treats blank registry paths as a noop",
			"   ",
			filepath.Join("repo"),
			filepath.Join("repo", ".cmdshape", "gain.db"),
			normalizedWorkspaceInput{},
			false,
			false,
		),
		Entry("returns normalized absolute paths for valid input",
			"./workspaces.db",
			"./repo",
			"./repo/.cmdshape/gain.db",
			normalizedWorkspaceInput{
				CWD: func() string {
					abs, err := filepath.Abs("./repo")
					Expect(err).NotTo(HaveOccurred())
					return filepath.Clean(abs)
				}(),
				MetricsPath: func() string {
					abs, err := filepath.Abs("./repo/.cmdshape/gain.db")
					Expect(err).NotTo(HaveOccurred())
					return filepath.Clean(abs)
				}(),
			},
			true,
			false,
		),
		Entry("rejects invalid cwd values when the registry path is set",
			"./workspaces.db",
			"   ",
			"./repo/.cmdshape/gain.db",
			normalizedWorkspaceInput{},
			false,
			true,
		),
	)

	It("merges existing workspace entries while preserving first seen and prior metrics path", func() {
		ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
		raw := []byte(`{"cwd":"/repo","metrics_path":"/repo/.cmdshape/gain.db","first_seen_at":"2026-03-19T12:00:00Z","last_seen_at":"2026-03-19T13:00:00Z"}`)

		entry, err := mergedWorkspaceEntry(raw, normalizedWorkspaceInput{
			CWD:         "/repo",
			MetricsPath: "",
		}, ts)

		Expect(err).NotTo(HaveOccurred())
		Expect(entry.CWD).To(Equal("/repo"))
		Expect(entry.MetricsPath).To(Equal("/repo/.cmdshape/gain.db"))
		Expect(entry.FirstSeenAt).To(Equal(time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)))
		Expect(entry.LastSeenAt).To(Equal(ts))
	})

	It("treats deleting from a registry without the workspaces bucket as a noop", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")

		db, err := bolt.Open(registryPath, 0o600, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Close()).To(Succeed())

		Expect(DeletePath(registryPath, filepath.Join(base, "repo"))).To(Succeed())
	})

	It("preserves the existing clock hook when test config does not override it", func() {
		prevNow := nowUTC
		sentinel := time.Date(2026, 3, 21, 8, 0, 0, 0, time.UTC)
		nowUTC = func() time.Time { return sentinel }
		DeferCleanup(func() { nowUTC = prevNow })

		restore := WithTestConfig(GinkgoT().TempDir(), nil)
		Expect(nowUTC()).To(Equal(sentinel))
		restore()
		Expect(nowUTC()).To(Equal(sentinel))
	})

})
