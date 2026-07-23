package projectfiles

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AtomicWriteFile", func() {
	It("writes a new file with the requested permissions", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "settings.json")

		Expect(AtomicWriteFile(path, []byte("new\n"), 0o640)).To(Succeed())

		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal([]byte("new\n")))
		if os.PathSeparator == '/' {
			info, statErr := os.Stat(path)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o640)))
		}
		Expect(atomicTemporaryFiles(root)).To(BeEmpty())
	})

	It("preserves existing destination permissions", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "hook.sh")
		Expect(os.WriteFile(path, []byte("old\n"), 0o751)).To(Succeed())

		Expect(AtomicWriteFile(path, []byte("new\n"), 0o600)).To(Succeed())

		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		if os.PathSeparator == '/' {
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o751)))
		}
	})

	It("rejects symlinked destinations without changing the target", func() {
		root := GinkgoT().TempDir()
		outside := filepath.Join(GinkgoT().TempDir(), "outside.txt")
		Expect(os.WriteFile(outside, []byte("keep\n"), 0o644)).To(Succeed())
		path := filepath.Join(root, "settings.json")
		if err := os.Symlink(outside, path); err != nil {
			Skip("symlink creation unavailable: " + err.Error())
		}

		Expect(AtomicWriteFile(path, []byte("replace\n"), 0o644)).NotTo(Succeed())

		body, err := os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal([]byte("keep\n")))
		Expect(atomicTemporaryFiles(root)).To(BeEmpty())
	})

	DescribeTable("preserves the original file after pre-replacement failures",
		func(stage string) {
			root := GinkgoT().TempDir()
			path := filepath.Join(root, "settings.json")
			Expect(os.WriteFile(path, []byte("original\n"), 0o640)).To(Succeed())
			injected := errors.New("injected " + stage + " failure")
			ops := defaultAtomicWriteOps
			switch stage {
			case "chmod", "write", "sync", "close":
				createTemp := ops.createTemp
				ops.createTemp = func(dir, pattern string) (atomicFile, error) {
					file, err := createTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return &failingAtomicFile{atomicFile: file, stage: stage, err: injected}, nil
				}
			case "replace":
				ops.replace = func(string, string) error { return injected }
			}

			err := atomicWriteFile(path, []byte("replacement\n"), 0o644, ops)

			Expect(err).To(MatchError(ContainSubstring(injected.Error())))
			body, readErr := os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(body).To(Equal([]byte("original\n")))
			Expect(atomicTemporaryFiles(root)).To(BeEmpty())
		},
		Entry("chmod fails", "chmod"),
		Entry("write fails", "write"),
		Entry("file sync fails", "sync"),
		Entry("close fails", "close"),
		Entry("replacement fails", "replace"),
	)

	It("reports a directory sync failure after installing complete replacement bytes", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "settings.json")
		Expect(os.WriteFile(path, []byte("original\n"), 0o644)).To(Succeed())
		ops := defaultAtomicWriteOps
		ops.syncDir = func(string) error { return errors.New("injected directory sync failure") }

		err := atomicWriteFile(path, []byte("replacement\n"), 0o644, ops)

		Expect(err).To(MatchError(ContainSubstring("injected directory sync failure")))
		body, readErr := os.ReadFile(path)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(body).To(Equal([]byte("replacement\n")))
		Expect(atomicTemporaryFiles(root)).To(BeEmpty())
	})

	It("keeps concurrent replacements complete", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "settings.json")
		payloads := [][]byte{
			[]byte("alpha\n"),
			[]byte("bravo-bravo\n"),
			[]byte("charlie-charlie-charlie\n"),
		}
		errs := make(chan error, 30)
		var wg sync.WaitGroup
		for index := range 30 {
			wg.Go(func() {
				errs <- AtomicWriteFile(path, payloads[index%len(payloads)], 0o644)
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			Expect(err).NotTo(HaveOccurred())
		}

		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(payloads).To(ContainElement(body))
		Expect(atomicTemporaryFiles(root)).To(BeEmpty())
	})

	It("rejects non-regular destinations", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "settings.json")
		Expect(os.Mkdir(path, 0o755)).To(Succeed())

		err := AtomicWriteFile(path, []byte("replacement\n"), 0o644)

		Expect(err).To(MatchError(ContainSubstring("refuse to replace non-regular file")))
	})

	It("reports destination inspection and temporary creation failures", func() {
		injected := errors.New("injected")
		ops := defaultAtomicWriteOps
		ops.lstat = func(string) (os.FileInfo, error) { return nil, injected }
		Expect(atomicWriteFile(filepath.Join(GinkgoT().TempDir(), "settings.json"), nil, 0o600, ops)).
			To(MatchError(ContainSubstring("inspect destination")))

		ops = defaultAtomicWriteOps
		ops.createTemp = func(string, string) (atomicFile, error) { return nil, injected }
		Expect(atomicWriteFile(filepath.Join(GinkgoT().TempDir(), "settings.json"), nil, 0o600, ops)).
			To(MatchError(ContainSubstring("create temporary file")))
	})

	It("joins temporary cleanup failures with the triggering failure", func() {
		root := GinkgoT().TempDir()
		ops := defaultAtomicWriteOps
		ops.replace = func(string, string) error { return errors.New("replace failed") }
		ops.remove = func(string) error { return errors.New("cleanup failed") }

		err := atomicWriteFile(filepath.Join(root, "settings.json"), []byte("data"), 0o600, ops)

		Expect(err).To(MatchError(And(
			ContainSubstring("replace failed"),
			ContainSubstring("cleanup failed"),
		)))
	})

	It("detects writers that make no progress", func() {
		Expect(writeAtomicBytes(zeroWriter{}, []byte("data"))).To(MatchError(io.ErrShortWrite))
		Expect(writeAtomicBytes(zeroWriter{}, nil)).To(Succeed())
	})
})

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) {
	return 0, nil
}

type failingAtomicFile struct {
	atomicFile
	stage string
	err   error
}

func (f *failingAtomicFile) Chmod(mode os.FileMode) error {
	if f.stage == "chmod" {
		return f.err
	}
	return f.atomicFile.Chmod(mode)
}

func (f *failingAtomicFile) Write(data []byte) (int, error) {
	if f.stage == "write" {
		return 0, f.err
	}
	return f.atomicFile.Write(data)
}

func (f *failingAtomicFile) Sync() error {
	if f.stage == "sync" {
		return f.err
	}
	return f.atomicFile.Sync()
}

func (f *failingAtomicFile) Close() error {
	if f.stage == "close" {
		_ = f.atomicFile.Close()
		return f.err
	}
	return f.atomicFile.Close()
}

func atomicTemporaryFiles(root string) []string {
	matches, err := filepath.Glob(filepath.Join(root, ".*.tmp-*"))
	Expect(err).NotTo(HaveOccurred())
	return matches
}
