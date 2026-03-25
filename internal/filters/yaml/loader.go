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
	"go-command-compression-proxy/internal/version"

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

type RegistryStatusRow struct {
	Tool       string
	FilterPath string
	Target     string
	SourceKind v2filters.SourceKind
	Status     string
	order      int
}

type compiledStatusFilter struct {
	tool   string
	path   string
	filter contracts.Filter
}

func ProjectRootFromSource() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func DefaultSources() []v2filters.FilterSource {
	if version.Version == "dev" {
		return []v2filters.FilterSource{
			v2filters.RepositorySource(ProjectRootFromSource()),
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return []v2filters.FilterSource{
			v2filters.ProjectSource(cwd),
		}
	}
	return []v2filters.FilterSource{
		v2filters.ProjectSource(cwd),
		v2filters.HomeSource(home),
	}
}

func LoadRegistryFiltersFromSources(sources []v2filters.FilterSource) (map[string]contracts.Filter, error) {
	registered := map[string]contracts.Filter{}
	for _, source := range sources {
		// Source order defines override priority. The first matching filter wins, so callers
		// should pass project-local sources before home-scoped sources when they want the
		// documented repo-specific override behavior from README "Bring Your Own Filter".
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

func LoadRegistryStatusFromSources(sources []v2filters.FilterSource) (map[string]contracts.Filter, []RegistryStatusRow, error) {
	registered := map[string]contracts.Filter{}
	rows := make([]RegistryStatusRow, 0)
	for order, source := range sources {
		filters, filterRows, err := inspectCompiledFiltersFromSource(source, order)
		rows = append(rows, filterRows...)
		if err != nil {
			rows = append(rows, RegistryStatusRow{
				Tool:       "-",
				FilterPath: source.Directory,
				SourceKind: source.Kind,
				Status:     "source error: " + err.Error(),
				order:      order,
			})
			continue
		}
		registerCompiledFilterStatuses(registered, filters, source, order, &rows)
		registerMappedFilterStatuses(registered, filters, source, order, &rows)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if diff := compareStatusRows(rows[i], rows[j]); diff != 0 {
			return diff < 0
		}
		return false
	})
	return registered, rows, nil
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

func inspectCompiledFiltersFromSource(source v2filters.FilterSource, order int) (map[string]compiledStatusFilter, []RegistryStatusRow, error) {
	paths, err := matchedFilterFiles(source.Directory)
	if err != nil {
		return nil, nil, err
	}

	compiled := make(map[string]compiledStatusFilter, len(paths))
	rows := make([]RegistryStatusRow, 0)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, rows, fmt.Errorf("read filter %s: %w", path, err)
		}
		spec, err := ParseDefinition(raw)
		if err != nil {
			rows = append(rows, RegistryStatusRow{
				Tool:       "-",
				FilterPath: path,
				SourceKind: source.Kind,
				Status:     "invalid filter: " + err.Error(),
				order:      order,
			})
			continue
		}
		filter, err := NewFilter(spec)
		if err != nil {
			rows = append(rows, RegistryStatusRow{
				Tool:       spec.Filter,
				FilterPath: path,
				SourceKind: source.Kind,
				Status:     "invalid filter: " + err.Error(),
				order:      order,
			})
			continue
		}
		if previous, ok := compiled[spec.Filter]; ok {
			rows = append(rows, RegistryStatusRow{
				Tool:       previous.tool,
				FilterPath: previous.path,
				SourceKind: source.Kind,
				Status:     "overridden",
				order:      order,
			})
		}
		compiled[spec.Filter] = compiledStatusFilter{tool: spec.Filter, path: path, filter: filter}
	}
	return compiled, rows, nil
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

func registerCompiledFilterStatuses(registered map[string]contracts.Filter, filters map[string]compiledStatusFilter, source v2filters.FilterSource, order int, rows *[]RegistryStatusRow) {
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := filters[key]
		status := "ok"
		if _, ok := registered[key]; ok {
			status = "overridden"
		} else {
			registered[key] = entry.filter
		}
		*rows = append(*rows, RegistryStatusRow{
			Tool:       entry.tool,
			FilterPath: entry.path,
			SourceKind: source.Kind,
			Status:     status,
			order:      order,
		})
	}
}

func registerMappedFilterStatuses(registered map[string]contracts.Filter, filters map[string]compiledStatusFilter, source v2filters.FilterSource, order int, rows *[]RegistryStatusRow) {
	mappingsPath := filepath.Join(source.Directory, ".mappings.yaml")
	mappings, err := readMappingsFile(mappingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			*rows = append(*rows, RegistryStatusRow{
				Tool:       "-",
				FilterPath: mappingsPath,
				SourceKind: source.Kind,
				Status:     "invalid mappings: " + err.Error(),
				order:      order,
			})
		}
		return
	}
	aliases := make([]string, 0, len(mappings))
	for alias := range mappings {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		target := mappings[alias]
		entry, ok := filters[target]
		if !ok {
			*rows = append(*rows, RegistryStatusRow{
				Tool:       alias,
				FilterPath: mappingsPath,
				Target:     target,
				SourceKind: source.Kind,
				Status:     "missing target: " + target,
				order:      order,
			})
			continue
		}
		status := "ok"
		if _, ok := registered[alias]; ok {
			status = "overridden"
		} else {
			registered[alias] = entry.filter
		}
		*rows = append(*rows, RegistryStatusRow{
			Tool:       alias,
			FilterPath: mappingsPath,
			Target:     target,
			SourceKind: source.Kind,
			Status:     status,
			order:      order,
		})
	}
}

func compareStatusRows(a, b RegistryStatusRow) int {
	if diff := compareStatusPriority(a.Status, b.Status); diff != 0 {
		return diff
	}
	if diff := compareSourceOrder(a.order, b.order); diff != 0 {
		return diff
	}
	if diff := strings.Compare(a.Tool, b.Tool); diff != 0 {
		return diff
	}
	if diff := strings.Compare(a.FilterPath, b.FilterPath); diff != 0 {
		return diff
	}
	return strings.Compare(a.Target, b.Target)
}

func compareStatusPriority(left, right string) int {
	return compareSourceOrder(statusPriority(left), statusPriority(right))
}

func statusPriority(status string) int {
	switch status {
	case "ok":
		return 0
	case "overridden":
		return 1
	default:
		return 2
	}
}

func compareSourceOrder(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
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
		return nil, fmt.Errorf("decode mappings %s: %w", path, err)
	}
	if payload.Version != 1 {
		return nil, fmt.Errorf("decode mappings %s: version must be exactly 1", path)
	}
	if payload.Map == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(payload.Map))
	for alias, target := range payload.Map {
		alias = strings.TrimSpace(alias)
		target = strings.TrimSpace(target)
		if alias == "" || target == "" {
			return nil, fmt.Errorf("decode mappings %s: mapping keys and values must be non-empty", path)
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
			return nil, fmt.Errorf("read filter %s: %w", p, err)
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
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("read filters %s: not a directory", root)
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
