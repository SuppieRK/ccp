//go:build windows

package projectfiles

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("Windows missing path errors",
	func(err error) {
		Expect(isWindowsPathNotFound(err)).To(BeTrue())
		Expect(isWindowsPathNotFound(fmt.Errorf("inspect: %w", err))).To(BeTrue())
	},
	Entry("Win32 file not found", windows.ERROR_FILE_NOT_FOUND),
	Entry("Win32 path not found", windows.ERROR_PATH_NOT_FOUND),
	Entry("NT no such file", windows.STATUS_NO_SUCH_FILE),
	Entry("NT object name not found", windows.STATUS_OBJECT_NAME_NOT_FOUND),
	Entry("NT object path not found", windows.STATUS_OBJECT_PATH_NOT_FOUND),
)

var _ = DescribeTable("Windows atomic replacement retry errors",
	func(err error, retryable bool) {
		Expect(retryableWindowsReplaceError(err)).To(Equal(retryable))
		if err != nil {
			Expect(retryableWindowsReplaceError(fmt.Errorf("replace: %w", err))).To(Equal(retryable))
		}
	},
	Entry("access denied", windows.ERROR_ACCESS_DENIED, true),
	Entry("sharing violation", windows.ERROR_SHARING_VIOLATION, true),
	Entry("lock violation", windows.ERROR_LOCK_VIOLATION, true),
	Entry("invalid parameter", windows.ERROR_INVALID_PARAMETER, false),
	Entry("no error", nil, false),
)

var _ = Describe("Windows contained file helpers", func() {
	It("removes a regular file relative to an opened parent", func() {
		root := GinkgoT().TempDir()
		parent, base, err := openWindowsContainedParent(root, filepath.Join(".cmdshape", "pending.json"))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = windows.CloseHandle(parent)
		})
		path := filepath.Join(root, ".cmdshape", base)
		Expect(os.WriteFile(path, []byte("pending"), 0o600)).To(Succeed())

		Expect(removeWindowsRelative(parent, base)).To(Succeed())
		Expect(path).NotTo(BeAnExistingFile())
		Expect(removeWindowsRelative(parent, base)).To(Succeed())
	})

	DescribeTable("opens files with native write dispositions",
		func(flag int, initial, write, expected string) {
			root := GinkgoT().TempDir()
			parent, base, err := openWindowsContainedParent(root, filepath.Join(".cmdshape", "state.txt"))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = windows.CloseHandle(parent)
			})
			path := filepath.Join(root, ".cmdshape", base)
			if initial != "" {
				Expect(os.WriteFile(path, []byte(initial), 0o600)).To(Succeed())
			}

			handle, err := openWindowsRelative(parent, base, flag, windows.FILE_NON_DIRECTORY_FILE)
			Expect(err).NotTo(HaveOccurred())
			file := os.NewFile(uintptr(handle), path)
			Expect(file).NotTo(BeNil())
			Expect(writeAtomicBytes(file, []byte(write))).To(Succeed())
			Expect(file.Close()).To(Succeed())

			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal(expected))
		},
		Entry("creates and truncates", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, "", "new", "new"),
		Entry("truncates an existing file", os.O_WRONLY|os.O_TRUNC, "old", "new", "new"),
		Entry("appends to an existing file", os.O_WRONLY|os.O_APPEND, "old", "new", "oldnew"),
	)

	DescribeTable("rejects invalid relative components",
		func(relative string) {
			handle, _, err := openWindowsContainedParent(GinkgoT().TempDir(), relative)
			Expect(handle).To(Equal(windows.InvalidHandle))
			Expect(err).To(MatchError(ContainSubstring("invalid contained path component")))
		},
		Entry("current directory", "."),
		Entry("parent directory", filepath.Join("..", "outside")),
	)

	It("reports invalid roots, names, handles, and rename destinations", func() {
		handle, _, err := openWindowsContainedParent("invalid\x00root", "file")
		Expect(handle).To(Equal(windows.InvalidHandle))
		Expect(err).To(HaveOccurred())

		handle, _, err = openWindowsContainedParent(filepath.Join(GinkgoT().TempDir(), "missing"), "file")
		Expect(handle).To(Equal(windows.InvalidHandle))
		Expect(err).To(MatchError(ContainSubstring("open contained root")))

		handle, err = openWindowsDirectoryRelative(windows.InvalidHandle, "invalid\x00name")
		Expect(handle).To(Equal(windows.InvalidHandle))
		Expect(err).To(HaveOccurred())

		handle, err = openWindowsRelative(windows.InvalidHandle, "invalid\x00name", os.O_RDONLY, 0)
		Expect(handle).To(Equal(windows.InvalidHandle))
		Expect(err).To(HaveOccurred())

		Expect(validateWindowsContainedHandle(windows.InvalidHandle, "invalid", false)).
			To(MatchError(ContainSubstring("inspect contained target")))
		Expect(renameWindowsRelative(windows.InvalidHandle, windows.InvalidHandle, "invalid\x00name")).
			To(HaveOccurred())
	})
})
