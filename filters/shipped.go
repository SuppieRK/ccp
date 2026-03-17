package filters

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:.mappings.yaml *.yaml
var shippedFS embed.FS

func MaterializeShipped(dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	entries, err := fs.ReadDir(shippedFS, ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := shippedFS.ReadFile(entry.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
