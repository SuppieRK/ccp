package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/SuppieRK/cmdshape/internal/filtertrust"
	"github.com/SuppieRK/cmdshape/internal/product"
	"github.com/SuppieRK/cmdshape/internal/workspaces"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("cmdshape brand migration", func() {
	var (
		home       string
		project    string
		executable string
	)

	BeforeEach(func() {
		root := GinkgoT().TempDir()
		home = filepath.Join(root, "home")
		project = filepath.Join(root, "project")
		executable = filepath.Join(root, "bin", product.Name)
		legacyExecutable := filepath.Join(root, "bin", legacyExecutableName)
		Expect(os.MkdirAll(home, 0o700)).To(Succeed())
		Expect(os.MkdirAll(project, 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Dir(executable), 0o755)).To(Succeed())
		Expect(os.WriteFile(executable, []byte("cmdshape executable"), 0o755)).To(Succeed())
		Expect(os.WriteFile(legacyExecutable, []byte("cmdshape executable"), 0o755)).To(Succeed())

		previousHome := brandMigrationHomeDir
		previousConfig := brandMigrationConfigDir
		previousWorking := brandMigrationWorkingDir
		previousExecutable := brandMigrationExecutable
		previousCandidates := brandMigrationCandidatePaths
		previousBuildInfo := brandMigrationReadBuildInfo
		previousStdout := brandMigrationStdout
		previousNow := brandMigrationNow
		brandMigrationHomeDir = func() (string, error) { return home, nil }
		brandMigrationConfigDir = func() (string, error) {
			return filepath.Join(home, ".config"), nil
		}
		brandMigrationWorkingDir = func() (string, error) { return project, nil }
		brandMigrationExecutable = func() (string, error) { return executable, nil }
		brandMigrationCandidatePaths = func(string, string, string) []string {
			return []string{legacyExecutable}
		}
		DeferCleanup(func() {
			brandMigrationHomeDir = previousHome
			brandMigrationConfigDir = previousConfig
			brandMigrationWorkingDir = previousWorking
			brandMigrationExecutable = previousExecutable
			brandMigrationCandidatePaths = previousCandidates
			brandMigrationReadBuildInfo = previousBuildInfo
			brandMigrationStdout = previousStdout
			brandMigrationNow = previousNow
		})
	})

	It("moves legacy state, rewrites registered metrics paths, and is idempotent", func() {
		legacyProject := legacyProjectStatePath(project)
		Expect(os.MkdirAll(filepath.Join(legacyProject, "filters"), 0o755)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(legacyProject, "filters", "git.yaml"),
			[]byte(
				"# yaml-language-server: $schema="+legacyRawSchemaURL+"\n"+
					"filter: git # canonical id used by .ccp/filters/.mappings.yaml, benchmark fixtures, and current filename.\n"+
					"about: user-authored CCP literal remains data\n"+
					"# flags_consuming_next_arg lists tool flags that consume the next argv token when CCP\n"+
					"# decides whether a token is a real positional argument.\n"+
					"# 3. compress_output rewrites stdout/stderr/combined output through the fixed CCP DSL\n",
			),
			0o644,
		)).To(Succeed())
		legacyHome := legacyHomeConfigPath(home)
		Expect(os.MkdirAll(legacyHome, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(legacyHome, "recovery.json"), []byte("{\"enabled\":true}\n"), 0o600)).To(Succeed())
		recoveryDir := filepath.Join(legacyHome, "recovery", "artifact")
		Expect(os.MkdirAll(recoveryDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(recoveryDir, "stdout.txt"),
			[]byte("00000|"+legacyReplayPrefix+"b2xkCg==\n"),
			0o600,
		)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(legacyHome, "audit"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(legacyHome, "audit", "audit.log"), []byte("old audit"), 0o600)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(home, legacyProjectDir), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(home, legacyProjectDir, "stale"), []byte("stale"), 0o600)).To(Succeed())
		legacyRegistry := filepath.Join(legacyHome, "workspaces.db")
		Expect(workspaces.UpsertPath(
			legacyRegistry,
			project,
			filepath.Join(project, legacyProjectDir, "gain.db"),
		)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		Expect(filepath.Join(project, product.ProjectDir, "filters", "git.yaml")).To(BeAnExistingFile())
		filterBody, err := os.ReadFile(filepath.Join(project, product.ProjectDir, "filters", "git.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(filterBody)).To(ContainSubstring(currentSchemaURL))
		Expect(string(filterBody)).NotTo(ContainSubstring(legacyRawSchemaURL))
		Expect(string(filterBody)).To(ContainSubstring(
			"# canonical id used by .cmdshape/filters/.mappings.yaml, benchmark fixtures, and current filename.",
		))
		Expect(string(filterBody)).To(ContainSubstring(
			"# flags_consuming_next_arg lists tool flags that consume the next argv token when cmdshape",
		))
		Expect(string(filterBody)).To(ContainSubstring(
			"# 3. compress_output rewrites stdout/stderr/combined output through the fixed cmdshape DSL",
		))
		Expect(string(filterBody)).To(ContainSubstring("about: user-authored CCP literal remains data"))
		Expect(filepath.Join(product.HomeConfigPath(home), "recovery.json")).To(BeAnExistingFile())
		recoveryBody, err := os.ReadFile(filepath.Join(product.HomeConfigPath(home), "recovery", "artifact", "stdout.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(recoveryBody)).To(ContainSubstring(currentReplayPrefix))
		Expect(string(recoveryBody)).NotTo(ContainSubstring(legacyReplayPrefix))
		Expect(filepath.Join(product.HomeConfigPath(home), "audit")).NotTo(BeAnExistingFile())
		Expect(filepath.Join(home, legacyProjectDir)).NotTo(BeAnExistingFile())
		Expect(filepath.Join(filepath.Dir(executable), legacyExecutableName)).NotTo(BeAnExistingFile())
		entries, err := workspaces.ListPath(filepath.Join(product.HomeConfigPath(home), "workspaces.db"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].MetricsPath).To(Equal(filepath.Join(project, product.ProjectDir, "gain.db")))

		second, err := executeBrandMigration()
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Complete).To(BeTrue())
		Expect(filepath.Join(project, product.ProjectDir, "filters", "git.yaml")).To(BeAnExistingFile())
	})

	It("translates exact filter trust approvals and discards stale approvals", func() {
		filtersDir := filepath.Join(product.ProjectStatePath(project), "filters")
		Expect(os.MkdirAll(filtersDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(filtersDir, "git.yaml"), []byte("filter: git\n"), 0o644)).To(Succeed())
		canonical, oldDigest, present, err := filtertrust.ProjectDigestForDomain(project, legacyTrustDomain)
		Expect(err).NotTo(HaveOccurred())
		Expect(present).To(BeTrue())
		legacyHome := legacyHomeConfigPath(home)
		Expect(os.MkdirAll(legacyHome, 0o700)).To(Succeed())
		store := migrationTrustStore{
			Version: 1,
			Projects: []migrationTrustApproval{
				{Root: canonical, Digest: oldDigest, TrustedAt: time.Unix(1, 0).UTC()},
				{Root: filepath.Join(project, "missing"), Digest: strings.Repeat("0", 64), TrustedAt: time.Unix(1, 0).UTC()},
			},
		}
		raw, err := json.Marshal(store)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(legacyHome, "filter-trust.json"), raw, 0o600)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		updatedRaw, err := os.ReadFile(filepath.Join(product.HomeConfigPath(home), "filter-trust.json"))
		Expect(err).NotTo(HaveOccurred())
		var updated migrationTrustStore
		Expect(json.Unmarshal(updatedRaw, &updated)).To(Succeed())
		Expect(updated.Projects).To(HaveLen(1))
		_, currentDigest, present, err := filtertrust.ProjectDigest(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(present).To(BeTrue())
		Expect(updated.Projects[0].Digest).To(Equal(currentDigest))
	})

	It("keeps cmdshape state and removes the superseded tree without merging", func() {
		Expect(os.MkdirAll(legacyProjectStatePath(project), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(legacyProjectStatePath(project), "old"), []byte("old"), 0o600)).To(Succeed())
		Expect(os.MkdirAll(product.ProjectStatePath(project), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(product.ProjectStatePath(project), "new"), []byte("new"), 0o600)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		Expect(slices.ContainsFunc(journal.Locations, func(location brandMigrationLocation) bool {
			return location.Kind == "project" && location.State == brandMigrationCompleted
		})).To(BeTrue())
		Expect(legacyProjectStatePath(project)).NotTo(BeAnExistingFile())
		Expect(filepath.Join(product.ProjectStatePath(project), "new")).To(BeAnExistingFile())
	})

	It("does not merge a historical home tree into existing cmdshape state", func() {
		legacyHome := legacyHomeConfigPath(home)
		currentHome := product.HomeConfigPath(home)
		Expect(os.MkdirAll(legacyHome, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(legacyHome, "old.json"), []byte("old"), 0o600)).To(Succeed())
		Expect(os.MkdirAll(currentHome, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(currentHome, "current.json"), []byte("current"), 0o600)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		Expect(legacyHome).NotTo(BeAnExistingFile())
		Expect(filepath.Join(currentHome, "old.json")).NotTo(BeAnExistingFile())
		Expect(filepath.Join(currentHome, "current.json")).To(BeAnExistingFile())
	})

	It("migrates every project recorded in either workspace registry", func() {
		otherProject := filepath.Join(filepath.Dir(project), "other-project")
		Expect(os.MkdirAll(filepath.Join(otherProject, legacyProjectDir), 0o755)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(otherProject, legacyProjectDir, "gain.db"),
			[]byte("metrics"),
			0o600,
		)).To(Succeed())
		legacyRegistry := filepath.Join(legacyHomeConfigPath(home), "workspaces.db")
		Expect(workspaces.UpsertPath(
			legacyRegistry,
			otherProject,
			filepath.Join(otherProject, legacyProjectDir, "gain.db"),
		)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		Expect(filepath.Join(otherProject, product.ProjectDir, "gain.db")).To(BeAnExistingFile())
		entries, err := workspaces.ListPath(filepath.Join(product.HomeConfigPath(home), "workspaces.db"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(ContainElement(HaveField(
			"MetricsPath",
			filepath.Join(otherProject, product.ProjectDir, "gain.db"),
		)))
	})

	It("journals a broken registry while continuing other cleanup", func() {
		legacyRegistry := filepath.Join(legacyHomeConfigPath(home), "workspaces.db")
		Expect(os.MkdirAll(filepath.Dir(legacyRegistry), 0o700)).To(Succeed())
		Expect(os.WriteFile(legacyRegistry, []byte("not a bolt database"), 0o600)).To(Succeed())
		Expect(os.MkdirAll(legacyProjectStatePath(project), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(legacyProjectStatePath(project), "gain.db"), []byte("metrics"), 0o600)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeFalse())
		Expect(slices.ContainsFunc(journal.Locations, func(location brandMigrationLocation) bool {
			return location.Kind == "workspace-registry-scan" &&
				location.State == brandMigrationSkipped
		})).To(BeTrue())
		Expect(filepath.Join(product.ProjectStatePath(project), "gain.db")).To(BeAnExistingFile())
		Expect(brandMigrationJournalPath(home)).To(BeAnExistingFile())
	})

	It("moves and rewrites recovery artifacts in the platform config directory", func() {
		platformConfig := filepath.Join(filepath.Dir(home), "platform-config")
		brandMigrationConfigDir = func() (string, error) { return platformConfig, nil }
		legacyRecovery := filepath.Join(platformConfig, legacyConfigDir, "recovery", "artifact")
		Expect(os.MkdirAll(legacyRecovery, 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(legacyRecovery, "stderr.txt"),
			[]byte("00000|"+legacyReplayPrefix+"b2xkCg==\n"),
			0o600,
		)).To(Succeed())

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		currentPath := filepath.Join(platformConfig, product.ConfigDir, "recovery", "artifact", "stderr.txt")
		body, err := os.ReadFile(currentPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring(currentReplayPrefix))
		Expect(string(body)).NotTo(ContainSubstring(legacyReplayPrefix))
		Expect(filepath.Join(platformConfig, legacyConfigDir)).NotTo(BeAnExistingFile())
	})

	It("removes a symbolic-link source without following it", func() {
		outside := filepath.Join(filepath.Dir(project), "outside")
		Expect(os.MkdirAll(outside, 0o755)).To(Succeed())
		legacyProject := legacyProjectStatePath(project)
		if err := os.Symlink(outside, legacyProject); err != nil {
			Skip("symlinks unavailable: " + err.Error())
		}

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		Expect(slices.ContainsFunc(journal.Locations, func(location brandMigrationLocation) bool {
			return location.Kind == "project" &&
				location.State == brandMigrationCompleted
		})).To(BeTrue())
		Expect(legacyProject).NotTo(BeAnExistingFile())
		Expect(outside).To(BeADirectory())
		Expect(product.ProjectStatePath(project)).NotTo(BeAnExistingFile())
	})

	It("reports an executable whose ownership cannot be verified", func() {
		legacyExecutable := filepath.Join(filepath.Dir(executable), legacyExecutableName)
		Expect(os.WriteFile(legacyExecutable, []byte("unrelated executable"), 0o755)).To(Succeed())
		var output bytes.Buffer
		brandMigrationStdout = &output

		Expect(RunBrandMigration([]string{"retry"})).To(Succeed())
		Expect(output.String()).To(ContainSubstring("cmdshape migration: pending"))
		Expect(output.String()).To(ContainSubstring("ownership is not verifiable"))

		output.Reset()
		Expect(RunBrandMigration([]string{"status"})).To(Succeed())
		Expect(output.String()).To(ContainSubstring("cmdshape migration: pending"))
		Expect(legacyExecutable).To(BeAnExistingFile())
	})

	It("removes an executable identified by historical Go build metadata", func() {
		legacyExecutable := filepath.Join(filepath.Dir(executable), legacyExecutableName)
		Expect(os.WriteFile(legacyExecutable, []byte("different bytes"), 0o755)).To(Succeed())
		brandMigrationReadBuildInfo = func(path string) (*debug.BuildInfo, error) {
			Expect(path).To(Equal(legacyExecutable))
			return &debug.BuildInfo{Path: legacyMainPackage}, nil
		}

		journal, err := executeBrandMigration()

		Expect(err).NotTo(HaveOccurred())
		Expect(journal.Complete).To(BeTrue())
		Expect(legacyExecutable).NotTo(BeAnExistingFile())
	})

	It("never blocks normal startup when migration infrastructure is unavailable", func() {
		brandMigrationHomeDir = func() (string, error) { return "", errors.New("home unavailable") }

		Expect(RunBrandMigrationAuto()).To(Succeed())
	})

	It("renders migration help", func() {
		out, err := captureStderrOutput(func() error { return RunBrandMigration([]string{"--help"}) })

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("cmdshape migrate - inspect or retry"))
		Expect(out).To(ContainSubstring("cmdshape migrate status"))
		Expect(out).To(ContainSubstring("Current cmdshape state wins"))
	})

	It("does not rerun a completed automatic migration", func() {
		first := time.Date(2026, time.July, 24, 10, 0, 0, 0, time.UTC)
		brandMigrationNow = func() time.Time { return first }

		Expect(RunBrandMigrationAuto()).To(Succeed())
		journalPath := brandMigrationJournalPath(home)
		before, err := os.ReadFile(journalPath)
		Expect(err).NotTo(HaveOccurred())

		brandMigrationNow = func() time.Time { return first.Add(time.Hour) }
		Expect(RunBrandMigrationAuto()).To(Succeed())
		Expect(os.ReadFile(journalPath)).To(Equal(before))
	})
})
