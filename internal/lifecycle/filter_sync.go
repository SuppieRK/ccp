package lifecycle

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/projectfiles"

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
	dstDir := filepath.Join(homeDir, configDirName, "ccp", "filters")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := syncMissingPackagedFilterEntry(tmpDir, dstDir, entry); err != nil {
			return err
		}
	}
	return nil
}

func materializedPackagedFiltersDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "ccp-filters-*")
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
	if current.Version == 0 {
		current.Version = 1
	}
	if current.Map == nil {
		current.Map = map[string]string{}
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
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var payload lifecycleMappingsFile
	if err := dec.Decode(&payload); err != nil {
		return lifecycleMappingsFile{}, fmt.Errorf("decode mappings %q: %w", path, err)
	}
	if payload.Version == 0 {
		payload.Version = 1
	}
	if payload.Map == nil {
		payload.Map = map[string]string{}
	}
	for alias, target := range payload.Map {
		alias = strings.TrimSpace(alias)
		target = strings.TrimSpace(target)
		if alias == "" || target == "" {
			return lifecycleMappingsFile{}, fmt.Errorf("decode mappings %q: mapping keys and values must be non-empty", path)
		}
	}
	return payload, nil
}

func marshalLifecycleMappings(payload lifecycleMappingsFile) ([]byte, error) {
	keys := make([]string, 0, len(payload.Map))
	for key := range payload.Map {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := lifecycleMappingsFile{
		Version: payload.Version,
		Map:     make(map[string]string, len(payload.Map)),
	}
	for _, key := range keys {
		ordered.Map[key] = payload.Map[key]
	}
	return yaml.Marshal(ordered)
}
