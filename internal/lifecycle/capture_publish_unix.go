//go:build !windows

package lifecycle

import "os"

func replaceCaptureArtifact(sourcePath, destinationPath string) error {
	return os.Rename(sourcePath, destinationPath)
}

func syncCaptureDirectory(path string) (err error) {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeWithErr(dir, &err)
	return dir.Sync()
}
