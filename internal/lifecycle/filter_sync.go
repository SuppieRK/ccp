package lifecycle

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/SuppieRK/cmdshape/internal/filtermappings"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"

	"gopkg.in/yaml.v3"
)

type lifecycleMappingsFile struct {
	Version int               `yaml:"version"`
	Map     map[string]string `yaml:"map"`
}

func syncMissingPackagedFilters(homeDir string) error {
	tmpDir, err := materializedPackagedFiltersDir()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	dstDir := filepath.Join(homeDir, configDirName, "cmdshape", "filters")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".mappings.yaml" {
			continue
		}
		if err := syncMissingPackagedFilterEntry(tmpDir, dstDir, entry); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if entry.Name() == ".mappings.yaml" {
			return syncMissingPackagedFilterEntry(tmpDir, dstDir, entry)
		}
	}
	return errors.New("shipped filters are missing .mappings.yaml")
}

func materializedPackagedFiltersDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "cmdshape-filters-*")
	if err != nil {
		return "", err
	}
	if err := materializeHomeFilters(tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpDir, nil
}

func syncMissingPackagedFilterEntry(srcDir, dstDir string, entry os.DirEntry) error {
	if entry.IsDir() {
		return nil
	}
	srcPath := filepath.Join(srcDir, entry.Name())
	dstPath := filepath.Join(dstDir, entry.Name())
	if entry.Name() == ".mappings.yaml" {
		return mergeMissingMappings(srcPath, dstPath)
	}
	return copyMissingFilterFile(srcPath, dstPath)
}

func copyMissingFilterFile(srcPath, dstPath string) error {
	missing, err := fileMissing(dstPath)
	if err != nil || !missing {
		return err
	}
	body, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return projectfiles.AtomicWriteFile(dstPath, body, 0o644)
}

func fileMissing(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func mergeMissingMappings(srcPath, dstPath string) error {
	source, err := readLifecycleMappings(srcPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		body, err := marshalLifecycleMappings(source)
		if err != nil {
			return err
		}
		return projectfiles.AtomicWriteFile(dstPath, body, 0o644)
	} else if err != nil {
		return err
	}

	current, err := readLifecycleMappings(dstPath)
	if err != nil {
		return err
	}
	for alias, target := range source.Map {
		if _, ok := current.Map[alias]; ok {
			continue
		}
		current.Map[alias] = target
	}
	body, err := marshalLifecycleMappings(current)
	if err != nil {
		return err
	}
	return projectfiles.AtomicWriteFile(dstPath, body, 0o644)
}

func readLifecycleMappings(path string) (lifecycleMappingsFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return lifecycleMappingsFile{}, err
	}
	mappings, err := filtermappings.Decode(path, raw)
	if err != nil {
		return lifecycleMappingsFile{}, err
	}
	return lifecycleMappingsFile{Version: 1, Map: mappings}, nil
}

func marshalLifecycleMappings(payload lifecycleMappingsFile) ([]byte, error) {
	keys := slices.Sorted(maps.Keys(payload.Map))
	ordered := lifecycleMappingsFile{
		Version: payload.Version,
		Map:     make(map[string]string, len(payload.Map)),
	}
	for _, key := range keys {
		ordered.Map[key] = payload.Map[key]
	}
	return yaml.Marshal(ordered)
}
