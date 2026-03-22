package yaml

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	v2filters "go-command-compression-proxy/internal/filters"

	"gopkg.in/yaml.v3"
)

type LoadedFilter struct {
	Path string
	Spec *FilterDefinition
}

type mappingsFile struct {
	Version int               `yaml:"version"`
	Map     map[string]string `yaml:"map"`
}

func ProjectRootFromSource() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func LoadRegistryFiltersFromSources(sources []v2filters.FilterSource) (map[string]contracts.Filter, error) {
	registered := map[string]contracts.Filter{}
	for _, source := range sources {
		filters, err := loadCompiledFiltersFromSource(source)
		if err != nil {
			return nil, err
		}
		registerCompiledFilters(registered, filters, source)
		if err := registerMappedFilters(registered, filters, source); err != nil {
			return nil, err
		}
	}
	return registered, nil
}

func loadCompiledFiltersFromSource(source v2filters.FilterSource) (map[string]contracts.Filter, error) {
	items, err := loadFilterDefinitionsFromDir(source.Directory)
	if err != nil {
		audit.MustAppend("filter_discovery_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       err.Error(),
		})
		return nil, err
	}
	filters, err := compileFilters(items)
	if err != nil {
		audit.MustAppend("filter_compile_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       err.Error(),
		})
		return nil, err
	}
	audit.MustAppend("filter_discovery", map[string]any{
		"source_kind":    string(source.Kind),
		"source_dir":     source.Directory,
		"definitions":    len(items),
		"compiled_count": len(filters),
	})
	return filters, nil
}

func registerCompiledFilters(registered map[string]contracts.Filter, filters map[string]contracts.Filter, source v2filters.FilterSource) {
	for tool, filter := range filters {
		if _, ok := registered[tool]; ok {
			audit.MustAppend("filter_conflict", map[string]any{
				"tool":       tool,
				"source_dir": source.Directory,
				"reason":     "duplicate filter id already registered from a higher-priority source",
			})
			continue
		}
		registered[tool] = filter
	}
}

func registerMappedFilters(registered map[string]contracts.Filter, filters map[string]contracts.Filter, source v2filters.FilterSource) error {
	mappings, err := readMappingsFile(filepath.Join(source.Directory, ".mappings.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		audit.MustAppend("mapping_read_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       err.Error(),
		})
		return nil
	}
	audit.MustAppend("mapping_discovery", map[string]any{
		"source_kind": string(source.Kind),
		"source_dir":  source.Directory,
		"count":       len(mappings),
	})
	for alias, target := range mappings {
		if _, ok := registered[alias]; ok {
			audit.MustAppend("mapping_conflict", map[string]any{
				"alias":      alias,
				"target":     target,
				"source_dir": source.Directory,
				"reason":     "alias already registered from a higher-priority source",
			})
			continue
		}
		filter, ok := filters[target]
		if !ok {
			audit.MustAppend("mapping_target_missing", map[string]any{
				"alias":      alias,
				"target":     target,
				"source_dir": source.Directory,
			})
			continue
		}
		registered[alias] = filter
	}
	return nil
}

func compileFilters(loaded []LoadedFilter) (map[string]contracts.Filter, error) {
	filters := make(map[string]contracts.Filter, len(loaded))
	for _, candidate := range loaded {
		filter, err := NewFilter(candidate.Spec)
		if err != nil {
			audit.MustAppend("filter_compile_invalid", map[string]any{
				"path":   candidate.Path,
				"filter": candidate.Spec.Filter,
				"error":  err.Error(),
			})
			continue
		}
		// Intentional override behavior: within a single source directory, later
		// lexicographically loaded files replace earlier ones for the same filter
		// id. This matches CCP's documented override model and keeps precedence
		// deterministic instead of failing registry construction on collisions.
		filters[candidate.Spec.Filter] = filter
	}
	return filters, nil
}

func readMappingsFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var payload mappingsFile
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode mappings %q: %w", path, err)
	}
	if payload.Version != 1 {
		return nil, fmt.Errorf("decode mappings %q: version must be exactly 1", path)
	}
	if payload.Map == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(payload.Map))
	for alias, target := range payload.Map {
		alias = strings.TrimSpace(alias)
		target = strings.TrimSpace(target)
		if alias == "" || target == "" {
			return nil, fmt.Errorf("decode mappings %q: mapping keys and values must be non-empty", path)
		}
		out[alias] = target
	}
	return out, nil
}

func loadFilterDefinitionsFromDir(dir string) ([]LoadedFilter, error) {
	paths, err := matchedFilterFiles(dir)
	if err != nil {
		return nil, err
	}

	out := make([]LoadedFilter, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read filter %q: %w", p, err)
		}
		spec, err := ParseDefinition(raw)
		if err != nil {
			audit.MustAppend("filter_definition_invalid", map[string]any{
				"path":  p,
				"error": err.Error(),
			})
			continue
		}
		out = append(out, LoadedFilter{Path: p, Spec: spec})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func matchedFilterFiles(root string) ([]string, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		switch filepath.Ext(entry.Name()) {
		case ".yaml", ".yml":
			matches = append(matches, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(matches)
	return matches, nil
}
