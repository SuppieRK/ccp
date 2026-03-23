package workspaces

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("workspace registry", func() {
	It("uses ~/.config/ccp/workspaces.db by default", func() {
		home := GinkgoT().TempDir()
		restore := WithTestConfig(home, nil)
		DeferCleanup(restore)

		path, err := DefaultPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(filepath.Join(home, ".config", "ccp", "workspaces.db")))
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
		metricsPath := filepath.Join(base, "repo", ".ccp", "gain.db")

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

	It("deletes a registered workspace by cwd", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")
		Expect(UpsertPath(registryPath, filepath.Join(base, "repo"), filepath.Join(base, "repo", ".ccp", "gain.db"))).To(Succeed())

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

		Expect(UpsertPath(registryPath, filepath.Join(home, "repo"), filepath.Join(home, "repo", ".ccp", "gain.db"))).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))

		Expect(DeletePath(registryPath, filepath.Join(home, "repo"))).To(Succeed())
		entries, err = ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())

		Expect(UpsertPath("", filepath.Join(home, "repo"), filepath.Join(home, "repo", ".ccp", "gain.db"))).To(Succeed())
		Expect(DeletePath("", filepath.Join(home, "repo"))).To(Succeed())
	})

	It("rejects empty workspace and metrics paths", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")

		Expect(UpsertPath(registryPath, "", filepath.Join(base, "repo", ".ccp", "gain.db"))).To(MatchError(ContainSubstring("path must not be empty")))
		Expect(DeletePath(registryPath, "")).To(MatchError(ContainSubstring("path must not be empty")))
	})

	It("keeps an existing metrics path when upserting a repo-only entry", func() {
		base := GinkgoT().TempDir()
		registryPath := filepath.Join(base, "workspaces.db")
		cwd := filepath.Join(base, "repo")
		metricsPath := filepath.Join(base, "repo", ".ccp", "gain.db")

		Expect(UpsertPath(registryPath, cwd, metricsPath)).To(Succeed())
		Expect(UpsertPath(registryPath, cwd, "")).To(Succeed())

		entries, err := ListPath(registryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].MetricsPath).To(Equal(metricsPath))
	})
})
