//go:build !windows

package projectfiles

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Unix contained file helpers", func() {
	It("rejects invalid relative path components", func() {
		for _, relative := range []string{"", ".", "../outside"} {
			_, err := containedPathParts(relative)
			Expect(err).To(MatchError(ContainSubstring("invalid contained path component")))
		}
	})

	It("opens existing contained parents without creating missing directories", func() {
		root := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(root, "present"), 0o755)).To(Succeed())

		fd, base, err := openContainedParent(root, filepath.Join("present", "file"), false)
		Expect(err).NotTo(HaveOccurred())
		Expect(base).To(Equal("file"))
		Expect(unix.Close(fd)).To(Succeed())

		_, _, err = openContainedParent(root, filepath.Join("missing", "file"), false)
		Expect(err).To(MatchError(ContainSubstring(`open contained directory "missing"`)))

		_, _, err = openContainedParent(filepath.Join(root, "absent-root"), "file", false)
		Expect(err).To(MatchError(ContainSubstring("open contained root")))
	})

	It("cleans up an abandoned temporary and tolerates a completed replacement", func() {
		root := GinkgoT().TempDir()
		parentFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(unix.Close(parentFD)).To(Succeed()) })
		path := filepath.Join(root, ".temporary")
		file, err := os.Create(path)
		Expect(err).NotTo(HaveOccurred())

		Expect(cleanupContainedTemporary(file, parentFD, ".temporary", true)).To(Succeed())
		Expect(path).NotTo(BeAnExistingFile())
		Expect(cleanupContainedTemporary(nil, parentFD, ".missing", true)).To(Succeed())
		Expect(cleanupContainedTemporary(nil, parentFD, ".missing", false)).To(Succeed())

		Expect(os.Mkdir(filepath.Join(root, ".directory"), 0o700)).To(Succeed())
		Expect(cleanupContainedTemporary(nil, parentFD, ".directory", true)).
			To(MatchError(ContainSubstring("remove contained temporary file")))
	})

	It("rejects non-regular and hard-linked atomic destinations", func() {
		root := GinkgoT().TempDir()
		parentFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(unix.Close(parentFD)).To(Succeed()) })

		Expect(os.Mkdir(filepath.Join(root, "directory"), 0o755)).To(Succeed())
		_, err = containedDestinationMode(parentFD, "directory", 0o600, "directory")
		Expect(err).To(MatchError(ContainSubstring("refuse non-regular contained file")))

		original := filepath.Join(root, "original")
		Expect(os.WriteFile(original, []byte("data"), 0o600)).To(Succeed())
		Expect(os.Link(original, filepath.Join(root, "linked"))).To(Succeed())
		_, err = containedDestinationMode(parentFD, "linked", 0o600, "linked")
		Expect(err).To(MatchError(ContainSubstring("refuse hard-linked contained file")))

		Expect(validateContainedFileFD(-1, "closed")).To(MatchError(ContainSubstring("inspect contained file")))
		_, err = containedDestinationMode(-1, "missing", 0o600, "closed")
		Expect(err).To(MatchError(ContainSubstring("inspect contained destination")))
	})
})
