package runner

import (
	"runtime"
	"strconv"
)

func isWindows() bool {
	return runtime.GOOS == "windows"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
