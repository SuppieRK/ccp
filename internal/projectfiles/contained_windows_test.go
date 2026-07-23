//go:build windows

package projectfiles

import (
	"fmt"

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
