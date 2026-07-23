package yaml

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	v2filters "go-command-compression-proxy/internal/filters"
	"go-command-compression-proxy/internal/filtertrust"
	"go-command-compression-proxy/internal/version"

	"gopkg.in/yaml.v3"
)

type LoadedFilter struct {
	Path string
	Raw  []byte
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
		return trustedProjectSources(cwd, "")
	}
	return trustedProjectSources(cwd, home)
}

func StatusSources() []v2filters.FilterSource {
	if version.Version == "dev" {
		return DefaultSources()
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	sources := []v2filters.FilterSource{v2filters.ProjectSource(cwd)}
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		sources = append(sources, v2filters.HomeSource(home))
	}
	return sources
}

func trustedProjectSources(cwd, home string) []v2filters.FilterSource {
	decision, err := filtertrust.Evaluate(cwd)
	audit.MustAppend("project_filter_trust", map[string]any{
		"project_root": decision.Root,
		"state":        decision.State,
		"reason":       decision.Reason,
	})
	sources := make([]v2filters.FilterSource, 0, 2)
	if err == nil && decision.State == filtertrust.StateTrusted {
		sources = append(sources, v2filters.ProjectSource(decision.Root))
	}
	if strings.TrimSpace(home) != "" {
		sources = append(sources, v2filters.HomeSource(home))
	}
	return sources
}

func LoadRegistryFiltersFromSources(sources []v2filters.FilterSource) (map[string]contracts.Filter, error) {
	registered, _, err := LoadRegistryFiltersFromSourcesWithTiming(sources)
	return registered, err
}

// LoadExecutionFilterFromSourcesWithTiming resolves only the invoked tool.
// Full-registry loading remains available for status, validation, repair, and
// authoring commands.
func LoadExecutionFilterFromSourcesWithTiming(sources []v2filters.FilterSource, tool string) (map[string]contracts.Filter, contracts.FilterRegistryBuildTiming, error) {
	startedAt := time.Now()
	registered := map[string]contracts.Filter{}
	timing := contracts.FilterRegistryBuildTiming{}
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return registered, timing, nil
	}

	for _, source := range sources {
		sourceStartedAt := time.Now()
		sourceTiming := contracts.FilterSourceBuildTiming{
			SourceKind: string(source.Kind),
			SourceDir:  source.Directory,
		}
		candidate, inspected, err := loadExecutionFilterFromSource(source, tool)
		sourceTiming.Definitions = int64(inspected)
		if err != nil {
			sourceTiming.DurationMS = time.Since(sourceStartedAt).Milliseconds()
			sourceTiming.Error = err.Error()
			timing.Sources = append(timing.Sources, sourceTiming)
			timing.DurationMS = time.Since(startedAt).Milliseconds()
			return nil, timing, err
		}
		if candidate != nil {
			sourceTiming.Compiled = 1
			registered[tool] = candidate
		}
		sourceTiming.DurationMS = time.Since(sourceStartedAt).Milliseconds()
		timing.Sources = append(timing.Sources, sourceTiming)
		if candidate != nil {
			break
		}
	}
	timing.DurationMS = time.Since(startedAt).Milliseconds()
	return registered, timing, nil
}

func loadExecutionFilterFromSource(source v2filters.FilterSource, tool string) (contracts.Filter, int, error) {
	mappings, mappingErr := readMappingsFile(filepath.Join(source.Directory, ".mappings.yaml"))
	if mappingErr != nil && !os.IsNotExist(mappingErr) {
		audit.MustAppend("mapping_read_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       mappingErr.Error(),
		})
		mappings = nil
	}

	targets := []string{tool}
	if target := strings.TrimSpace(mappings[tool]); target != "" && target != tool {
		targets = append(targets, target)
	}
	for _, target := range targets {
		loaded, inspected, found, err := loadExactTargetDefinition(source.Directory, target)
		if err != nil {
			return nil, inspected, err
		}
		if found {
			return compileExecutionFilter(source, loaded), inspected, nil
		}
	}

	loaded, inspected, found, err := loadLegacyTargetDefinition(source.Directory, targets)
	if err != nil || !found {
		return nil, inspected, err
	}
	return compileExecutionFilter(source, loaded), inspected, nil
}

func loadExactTargetDefinition(dir, target string) (LoadedFilter, int, bool, error) {
	paths := []string{
		filepath.Join(dir, target+".yaml"),
		filepath.Join(dir, target+".yml"),
	}
	var selected LoadedFilter
	inspected := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return LoadedFilter{}, inspected, false, fmt.Errorf("read filter %s: %w", path, err)
		}
		inspected++
		spec, err := ParseDefinition(raw)
		if err != nil {
			audit.MustAppend("filter_definition_invalid", map[string]any{
				"path":  path,
				"error": err.Error(),
			})
			continue
		}
		if spec.Filter == target {
			selected = LoadedFilter{Path: path, Raw: raw, Spec: spec}
		}
	}
	return selected, inspected, selected.Spec != nil, nil
}

func loadLegacyTargetDefinition(dir string, targets []string) (LoadedFilter, int, bool, error) {
	paths, err := matchedFilterFiles(dir)
	if err != nil {
		return LoadedFilter{}, 0, false, err
	}
	targetSet := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		targetSet[target] = struct{}{}
	}
	var selected LoadedFilter
	inspected := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return LoadedFilter{}, inspected, false, fmt.Errorf("read filter %s: %w", path, err)
		}
		inspected++
		spec, err := ParseDefinition(raw)
		if err != nil {
			audit.MustAppend("filter_definition_invalid", map[string]any{
				"path":  path,
				"error": err.Error(),
			})
			continue
		}
		if _, ok := targetSet[spec.Filter]; ok {
			selected = LoadedFilter{Path: path, Raw: raw, Spec: spec}
		}
	}
	return selected, inspected, selected.Spec != nil, nil
}

func compileExecutionFilter(source v2filters.FilterSource, loaded LoadedFilter) contracts.Filter {
	if loaded.Spec == nil {
		return nil
	}
	filter, err := NewFilter(loaded.Spec)
	if err != nil {
		audit.MustAppend("filter_compile_invalid", map[string]any{
			"path":   loaded.Path,
			"filter": loaded.Spec.Filter,
			"error":  err.Error(),
		})
		return nil
	}
	return filter.WithProvenance(filterProvenance(source, loaded.Path, loaded.Raw))
}

func LoadRegistryFiltersFromSourcesWithTiming(sources []v2filters.FilterSource) (map[string]contracts.Filter, contracts.FilterRegistryBuildTiming, error) {
	startedAt := time.Now()
	registered := map[string]contracts.Filter{}
	timing := contracts.FilterRegistryBuildTiming{}
	for _, source := range sources {
		// Source order defines override priority. The first matching filter wins, so callers
		// should pass project-local sources before home-scoped sources when they want the
		// documented repo-specific override behavior from README "Bring Your Own Filter".
		sourceStartedAt := time.Now()
		filters, sourceTiming, err := loadCompiledFiltersFromSourceWithTiming(source)
		if err != nil {
			sourceTiming.DurationMS = time.Since(sourceStartedAt).Milliseconds()
			sourceTiming.Error = err.Error()
			timing.Sources = append(timing.Sources, sourceTiming)
			timing.DurationMS = time.Since(startedAt).Milliseconds()
			return nil, timing, err
		}
		registerCompiledFilters(registered, filters, source)
		if err := registerMappedFilters(registered, filters, source); err != nil {
			sourceTiming.DurationMS = time.Since(sourceStartedAt).Milliseconds()
			sourceTiming.Error = err.Error()
			timing.Sources = append(timing.Sources, sourceTiming)
			timing.DurationMS = time.Since(startedAt).Milliseconds()
			return nil, timing, err
		}
		sourceTiming.DurationMS = time.Since(sourceStartedAt).Milliseconds()
		timing.Sources = append(timing.Sources, sourceTiming)
	}
	timing.DurationMS = time.Since(startedAt).Milliseconds()
	return registered, timing, nil
}

func LoadRegistryStatusFromSources(sources []v2filters.FilterSource) (map[string]contracts.Filter, []RegistryStatusRow, error) {
	return LoadRegistryStatusFromSourcesWithProjectState(sources, "")
}

func LoadRegistryStatusFromSourcesWithProjectState(sources []v2filters.FilterSource, projectState filtertrust.State) (map[string]contracts.Filter, []RegistryStatusRow, error) {
	registered := map[string]contracts.Filter{}
	rows := make([]RegistryStatusRow, 0)
	for order, source := range sources {
		if source.Kind == v2filters.SourceProject &&
			(projectState == filtertrust.StateUnsafe || projectState == filtertrust.StateAbsent) {
			if projectState == filtertrust.StateUnsafe {
				rows = append(rows, RegistryStatusRow{
					Tool:       "-",
					FilterPath: source.Directory,
					SourceKind: source.Kind,
					Status:     string(projectState),
					order:      order,
				})
			}
			continue
		}
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
		if source.Kind == v2filters.SourceProject && projectState != "" && projectState != filtertrust.StateTrusted {
			projectRows := make([]RegistryStatusRow, 0)
			scratch := map[string]contracts.Filter{}
			registerCompiledFilterStatuses(scratch, filters, source, order, &projectRows)
			registerMappedFilterStatuses(scratch, filters, source, order, &projectRows)
			for i := range projectRows {
				projectRows[i].Status = string(projectState)
			}
			for i := range rows {
				if rows[i].SourceKind == v2filters.SourceProject && rows[i].order == order {
					rows[i].Status = string(projectState) + "; " + rows[i].Status
				}
			}
			rows = append(rows, projectRows...)
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

func loadCompiledFiltersFromSourceWithTiming(source v2filters.FilterSource) (map[string]contracts.Filter, contracts.FilterSourceBuildTiming, error) {
	timing := contracts.FilterSourceBuildTiming{
		SourceKind: string(source.Kind),
		SourceDir:  source.Directory,
	}
	items, err := loadFilterDefinitionsFromDir(source.Directory)
	if err != nil {
		audit.MustAppend("filter_discovery_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       err.Error(),
		})
		return nil, timing, err
	}
	timing.Definitions = int64(len(items))
	filters, err := compileFiltersFromSource(items, source)
	if err != nil {
		audit.MustAppend("filter_compile_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       err.Error(),
		})
		return nil, timing, err
	}
	timing.Compiled = int64(len(filters))
	audit.MustAppend("filter_discovery", map[string]any{
		"source_kind":    string(source.Kind),
		"source_dir":     source.Directory,
		"definitions":    len(items),
		"compiled_count": len(filters),
	})
	return filters, timing, nil
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
		filter.WithProvenance(filterProvenance(source, path, raw))
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
	return compileFiltersFromSource(loaded, v2filters.FilterSource{})
}

func compileFiltersFromSource(loaded []LoadedFilter, source v2filters.FilterSource) (map[string]contracts.Filter, error) {
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
		filter.WithProvenance(filterProvenance(source, candidate.Path, candidate.Raw))
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
		out = append(out, LoadedFilter{Path: p, Raw: raw, Spec: spec})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func filterProvenance(source v2filters.FilterSource, path string, raw []byte) contracts.FilterProvenance {
	if source.Kind == "" && strings.TrimSpace(path) == "" && len(raw) == 0 {
		return contracts.FilterProvenance{}
	}
	sum := sha256.Sum256(raw)
	return contracts.FilterProvenance{
		SourceKind: string(source.Kind),
		Path:       path,
		Hash:       fmt.Sprintf("%x", sum),
	}
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
