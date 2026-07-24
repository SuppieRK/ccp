package recovery

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/SuppieRK/cmdshape/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("recovery storage", func() {
	var root string

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		previousConfigDir := userConfigDir
		userConfigDir = func() (string, error) { return root, nil }
		DeferCleanup(func() { userConfigDir = previousConfigDir })
	})

	It("is disabled by default and persists a private atomic preference", func() {
		enabled, err := Enabled()
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())

		Expect(SetEnabled(true)).To(Succeed())
		enabled, err = Enabled()
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
		info, err := os.Stat(filepath.Join(root, "cmdshape", "recovery.json"))
		Expect(err).NotTo(HaveOccurred())
		if runtime.GOOS != "windows" {
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		}
	})

	It("stores replay-compatible private artifacts and purges them", func() {
		artifact, err := Store([]string{"demo"}, []Event{
			{Sequence: 0, Stream: contracts.StreamStdout, Data: []byte("out\n")},
			{Sequence: 1, Stream: contracts.StreamStderr, Data: []byte("err\n")},
		}, 7)
		Expect(err).NotTo(HaveOccurred())
		Expect(artifact).NotTo(BeNil())

		items, err := List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].ExitCode).To(Equal(7))
		dirInfo, err := os.Stat(items[0].Path)
		Expect(err).NotTo(HaveOccurred())
		if runtime.GOOS != "windows" {
			Expect(dirInfo.Mode().Perm()).To(Equal(os.FileMode(0o700)))
		}
		for _, name := range []string{"command.yaml", "stdout.txt", "stderr.txt"} {
			info, statErr := os.Stat(filepath.Join(items[0].Path, name))
			Expect(statErr).NotTo(HaveOccurred())
			if runtime.GOOS != "windows" {
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
			}
		}

		removed, err := Purge()
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(1))
		items, err = List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
	})

	It("does not retain native-zero or oversized artifacts", func() {
		artifact, err := Store([]string{"demo"}, []Event{{Sequence: 0, Stream: contracts.StreamStdout, Data: []byte("out")}}, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(artifact).To(BeNil())

		artifact, err = Store([]string{"demo"}, []Event{{Sequence: 0, Stream: contracts.StreamStdout, Data: make([]byte, maxArtifactSize+1)}}, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(artifact).To(BeNil())
	})

	It("rotates artifacts by age and count", func() {
		for range maxArtifacts + 3 {
			artifact, err := Store([]string{"demo"}, []Event{{Sequence: 0, Stream: contracts.StreamStdout, Data: []byte("x")}}, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(artifact).NotTo(BeNil())
		}
		items, err := List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(maxArtifacts))

		old := items[len(items)-1]
		oldTime := time.Now().Add(-maxArtifactAge - time.Hour)
		Expect(os.Chtimes(old.Path, oldTime, oldTime)).To(Succeed())
		items, err = List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(maxArtifacts - 1))
	})

	It("serializes concurrent writers without exposing partial artifacts", func() {
		var wait sync.WaitGroup
		errs := make(chan error, maxArtifacts+10)
		for index := range maxArtifacts + 10 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_, err := Store(
					[]string{"demo"},
					[]Event{{Sequence: 0, Stream: contracts.StreamStdout, Data: []byte("event\n")}},
					index+1,
				)
				errs <- err
			}()
		}
		wait.Wait()
		close(errs)
		for err := range errs {
			Expect(err).NotTo(HaveOccurred())
		}
		items, err := List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(maxArtifacts))
	})

	It("rejects a symlinked recovery root", func() {
		outside := filepath.Join(root, "outside")
		Expect(os.Mkdir(outside, 0o700)).To(Succeed())
		recoveryRoot := filepath.Join(root, "cmdshape", "recovery")
		Expect(os.MkdirAll(filepath.Dir(recoveryRoot), 0o700)).To(Succeed())
		if err := os.Symlink(outside, recoveryRoot); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		artifact, err := Store(
			[]string{"demo"},
			[]Event{{Sequence: 0, Stream: contracts.StreamStdout, Data: []byte("event\n")}},
			1,
		)
		Expect(err).To(HaveOccurred())
		Expect(artifact).To(BeNil())
		entries, readErr := os.ReadDir(outside)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("handles missing stores and malformed preferences", func() {
		items, err := List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		removed, err := Purge()
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(BeZero())

		configPath, err := ConfigPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Dir(configPath), 0o700)).To(Succeed())
		Expect(os.WriteFile(configPath, []byte("{"), 0o600)).To(Succeed())
		_, err = Enabled()
		Expect(err).To(HaveOccurred())
	})

	It("rejects invalid event ordering without creating storage", func() {
		artifact, err := Store([]string{"demo"}, []Event{
			{Sequence: 1, Stream: contracts.StreamStdout, Data: []byte("late\n")},
			{Sequence: 0, Stream: contracts.StreamStderr, Data: []byte("early\n")},
		}, 1)
		Expect(err).To(HaveOccurred())
		Expect(artifact).To(BeNil())
	})

	It("ignores unrelated entries while listing and purging", func() {
		recoveryRoot, err := RootPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Join(recoveryRoot, "malformed"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(recoveryRoot, "unrelated.txt"), []byte("keep"), 0o600)).To(Succeed())

		items, err := List()
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		removed, err := Purge()
		Expect(err).NotTo(HaveOccurred())
		Expect(removed).To(Equal(1))
		Expect(filepath.Join(recoveryRoot, "unrelated.txt")).To(BeAnExistingFile())
	})

	It("propagates configuration directory failures across public operations", func() {
		previous := userConfigDir
		userConfigDir = func() (string, error) { return "", errors.New("config unavailable") }
		DeferCleanup(func() { userConfigDir = previous })

		_, err := ConfigPath()
		Expect(err).To(MatchError("config unavailable"))
		_, err = RootPath()
		Expect(err).To(MatchError("config unavailable"))
		_, err = Enabled()
		Expect(err).To(MatchError("config unavailable"))
		Expect(SetEnabled(true)).To(MatchError("config unavailable"))
		_, err = Store([]string{"demo"}, []Event{{Sequence: 0, Stream: contracts.StreamStdout, Data: []byte("x")}}, 1)
		Expect(err).To(MatchError("config unavailable"))
		_, err = List()
		Expect(err).To(MatchError("config unavailable"))
		_, err = Purge()
		Expect(err).To(MatchError("config unavailable"))
	})

	It("rotates only directories and deterministically removes overflow", func() {
		recoveryRoot, err := RootPath()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(recoveryRoot, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(recoveryRoot, "keep.txt"), []byte("keep"), 0o600)).To(Succeed())
		baseTime := time.Now().Add(-time.Hour)
		for index := range maxArtifacts + 2 {
			path := filepath.Join(recoveryRoot, string(rune('a'+index)))
			Expect(os.Mkdir(path, 0o700)).To(Succeed())
			modTime := baseTime.Add(time.Duration(index) * time.Minute)
			Expect(os.Chtimes(path, modTime, modTime)).To(Succeed())
		}

		Expect(rotate(recoveryRoot)).To(Succeed())

		entries, err := os.ReadDir(recoveryRoot)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(maxArtifacts + 1))
		Expect(filepath.Join(recoveryRoot, "a")).NotTo(BeADirectory())
		Expect(filepath.Join(recoveryRoot, "b")).NotTo(BeADirectory())
		Expect(filepath.Join(recoveryRoot, "keep.txt")).To(BeAnExistingFile())
	})

	It("measures nested artifact bytes", func() {
		root := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(root, "nested"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "one"), []byte("123"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "nested", "two"), []byte("4567"), 0o600)).To(Succeed())

		size, err := directorySize(root)

		Expect(err).NotTo(HaveOccurred())
		Expect(size).To(Equal(int64(7)))
	})
})
