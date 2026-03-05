package lifecycle

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setHomeDirForTest(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" {
		return
	}

	t.Setenv("USERPROFILE", home)
	vol := filepath.VolumeName(home)
	if vol != "" {
		t.Setenv("HOMEDRIVE", vol)
		t.Setenv("HOMEPATH", strings.TrimPrefix(home, vol))
	}
}
