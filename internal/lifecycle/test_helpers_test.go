package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type lifecycleWorkspace struct {
	root string
	home string
	work string
}

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

func newLifecycleWorkspace(t *testing.T) lifecycleWorkspace {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	setHomeDirForTest(t, home)
	chdirForTest(t, work)
	return lifecycleWorkspace{
		root: root,
		home: home,
		work: work,
	}
}
