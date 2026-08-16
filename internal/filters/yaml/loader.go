package yaml

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/filtermappings"
	v2filters "github.com/SuppieRK/cmdshape/internal/filters"
	"github.com/SuppieRK/cmdshape/internal/filtertrust"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"
	"github.com/SuppieRK/cmdshape/internal/version"
)

const mappingsFileName = ".mappings.yaml"

type LoadedFilter struct {
	Path string
	Raw  []byte
	Spec *FilterDefinition
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

type inspectedFilterSource struct {
	filters     map[string]compiledStatusFilter
	rows        []RegistryStatusRow
	definitions int64
}

type preparedFilterSource struct {
	source       v2filters.FilterSource
	projectFiles map[string][]byte
}

func prepareFilterSource(source v2filters.FilterSource) (preparedFilterSource, bool, error) {
	if source.Kind != v2filters.SourceProject {
		return preparedFilterSource{source: source}, true, nil
	}

	projectRoot := filepath.Dir(filepath.Dir(source.Directory))
	decision, files, err := filtertrust.EvaluateSource(projectRoot)
	audit.MustAppend("project_filter_trust", map[string]any{
		"project_root": decision.Root,
		"state":        decision.State,
		"reason":       decision.Reason,
	})
	if err != nil || decision.State != filtertrust.StateTrusted {
		return preparedFilterSource{source: source}, false, nil
	}

	source.Directory = filepath.Join(decision.Root, ".cmdshape", "filters")
	projectFiles := make(map[string][]byte, len(files))
	for _, file := range files {
		projectFiles[file.Name] = file.Raw
	}
	return preparedFilterSource{source: source, projectFiles: projectFiles}, true, nil
}

func (s preparedFilterSource) readFile(path string) ([]byte, error) {
	if s.projectFiles == nil {
		return os.ReadFile(path)
	}
	raw, ok := s.projectFiles[filepath.Base(path)]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return raw, nil
}

func (s preparedFilterSource) matchedFilterFiles() ([]string, error) {
	if s.projectFiles == nil {
		return matchedFilterFiles(s.source.Directory)
	}
	matches := make([]string, 0, len(s.projectFiles))
	for name := range s.projectFiles {
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch filepath.Ext(name) {
		case ".yaml", ".yml":
			matches = append(matches, filepath.Join(s.source.Directory, name))
		}
	}
	slices.Sort(matches)
	return matches, nil
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
	root, err := projectfiles.ResolveProjectRoot(cwd)
	if err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return trustedProjectSources(root, "")
	}
	return trustedProjectSources(root, home)
}

func StatusSources() []v2filters.FilterSource {
	if version.Version == "dev" {
		return DefaultSources()
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	root, err := projectfiles.ResolveProjectRoot(cwd)
	if err != nil {
		return nil
	}
	sources := []v2filters.FilterSource{v2filters.ProjectSource(root)}
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
	prepared, enabled, err := prepareFilterSource(source)
	if err != nil || !enabled {
		return nil, 0, err
	}
	source = prepared.source
	mappings := readExecutionMappings(prepared)
	targets := executionFilterTargets(tool, mappings)
	for _, target := range targets {
		filter, inspected, found, err := loadExactExecutionFilter(prepared, source, target)
		if err != nil || found {
			return filter, inspected, err
		}
	}

	loaded, inspected, found, err := loadLegacyTargetDefinitionAfter(prepared, targets, "")
	if err != nil || !found {
		return nil, inspected, err
	}
	return compileExecutionFilter(source, loaded), inspected, nil
}

func readExecutionMappings(source preparedFilterSource) map[string]string {
	mappings, err := source.readMappingsFile(filepath.Join(source.source.Directory, mappingsFileName))
	if err == nil || os.IsNotExist(err) {
		return mappings
	}
	audit.MustAppend("mapping_read_error", map[string]any{
		"source_kind": string(source.source.Kind),
		"source_dir":  source.source.Directory,
		"error":       err.Error(),
	})
	return nil
}

func executionFilterTargets(tool string, mappings map[string]string) []string {
	targets := []string{tool}
	if target := strings.TrimSpace(mappings[tool]); target != "" && target != tool {
		targets = append(targets, target)
	}
	return targets
}

func loadExactExecutionFilter(prepared preparedFilterSource, source v2filters.FilterSource, target string) (contracts.Filter, int, bool, error) {
	loaded, inspected, found, err := loadExactTargetDefinition(prepared, target)
	if err != nil || !found {
		return nil, inspected, found, err
	}
	override, scanned, overridden, err := loadLegacyTargetDefinitionAfter(prepared, []string{target}, loaded.Path)
	inspected += scanned
	if err != nil {
		return nil, inspected, true, err
	}
	if overridden {
		loaded = override
	}
	return compileExecutionFilter(source, loaded), inspected, true, nil
}

type registryStatusBuilder struct {
	projectState filtertrust.State
	registered   map[string]contracts.Filter
	rows         []RegistryStatusRow
}

func (b *registryStatusBuilder) addSource(source v2filters.FilterSource, order int) {
	if b.skipUnavailableProjectSource(source, order) {
		return
	}
	filters, filterRows, err := inspectCompiledFiltersFromSource(source, order)
	b.rows = append(b.rows, filterRows...)
	if err != nil {
		b.rows = append(b.rows, RegistryStatusRow{
			Tool:       "-",
			FilterPath: source.Directory,
			SourceKind: source.Kind,
			Status:     "source error: " + err.Error(),
			order:      order,
		})
		return
	}
	if b.appendUntrustedProjectStatuses(filters, source, order) {
		return
	}
	registerCompiledFilterStatuses(b.registered, filters, source, order, &b.rows)
	registerMappedFilterStatuses(b.registered, filters, source, order, &b.rows)
}

func (b *registryStatusBuilder) skipUnavailableProjectSource(source v2filters.FilterSource, order int) bool {
	if source.Kind != v2filters.SourceProject ||
		(b.projectState != filtertrust.StateUnsafe && b.projectState != filtertrust.StateAbsent) {
		return false
	}
	if b.projectState == filtertrust.StateUnsafe {
		b.rows = append(b.rows, RegistryStatusRow{
			Tool:       "-",
			FilterPath: source.Directory,
			SourceKind: source.Kind,
			Status:     string(b.projectState),
			order:      order,
		})
	}
	return true
}

func (b *registryStatusBuilder) appendUntrustedProjectStatuses(filters map[string]compiledStatusFilter, source v2filters.FilterSource, order int) bool {
	if source.Kind != v2filters.SourceProject || b.projectState == "" || b.projectState == filtertrust.StateTrusted {
		return false
	}
	projectRows := make([]RegistryStatusRow, 0)
	scratch := map[string]contracts.Filter{}
	registerCompiledFilterStatuses(scratch, filters, source, order, &projectRows)
	registerMappedFilterStatuses(scratch, filters, source, order, &projectRows)
	for i := range projectRows {
		projectRows[i].Status = string(b.projectState)
	}
	for i := range b.rows {
		if b.rows[i].SourceKind == v2filters.SourceProject && b.rows[i].order == order {
			b.rows[i].Status = string(b.projectState) + "; " + b.rows[i].Status
		}
	}
	b.rows = append(b.rows, projectRows...)
	return true
}

func LoadRegistryStatusFromSourcesWithProjectState(sources []v2filters.FilterSource, projectState filtertrust.State) (map[string]contracts.Filter, []RegistryStatusRow, error) {
	builder := registryStatusBuilder{
		projectState: projectState,
		registered:   map[string]contracts.Filter{},
		rows:         make([]RegistryStatusRow, 0),
	}
	for order, source := range sources {
		builder.addSource(source, order)
	}
	slices.SortStableFunc(builder.rows, compareStatusRows)
	return builder.registered, builder.rows, nil
}

func loadExactTargetDefinition(source preparedFilterSource, target string) (LoadedFilter, int, bool, error) {
	dir := source.source.Directory
	paths := []string{
		filepath.Join(dir, target+".yaml"),
		filepath.Join(dir, target+".yml"),
	}
	var selected LoadedFilter
	inspected := 0
	for _, path := range paths {
		raw, err := source.readFile(path)
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

func loadLegacyTargetDefinitionAfter(source preparedFilterSource, targets []string, after string) (LoadedFilter, int, bool, error) {
	paths, err := source.matchedFilterFiles()
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
		if after != "" && strings.Compare(path, after) <= 0 {
			continue
		}
		raw, err := source.readFile(path)
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
	filter, err := compileLoadedFilter(source, loaded)
	if err != nil {
		audit.MustAppend("filter_compile_invalid", map[string]any{
			"path":   loaded.Path,
			"filter": loaded.Spec.Filter,
			"error":  err.Error(),
		})
		return nil
	}
	return filter
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
		prepared, enabled, prepareErr := prepareFilterSource(source)
		if prepareErr != nil {
			timing.DurationMS = time.Since(startedAt).Milliseconds()
			return nil, timing, prepareErr
		}
		if !enabled {
			timing.Sources = append(timing.Sources, contracts.FilterSourceBuildTiming{
				SourceKind: string(source.Kind),
				SourceDir:  source.Directory,
				DurationMS: time.Since(sourceStartedAt).Milliseconds(),
			})
			continue
		}
		source = prepared.source
		filters, sourceTiming, err := loadCompiledFiltersFromSourceWithTiming(prepared)
		if err != nil {
			sourceTiming.DurationMS = time.Since(sourceStartedAt).Milliseconds()
			sourceTiming.Error = err.Error()
			timing.Sources = append(timing.Sources, sourceTiming)
			timing.DurationMS = time.Since(startedAt).Milliseconds()
			return nil, timing, err
		}
		registerCompiledFilters(registered, filters, source)
		if err := registerMappedFilters(registered, filters, prepared); err != nil {
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

func loadCompiledFiltersFromSourceWithTiming(prepared preparedFilterSource) (map[string]contracts.Filter, contracts.FilterSourceBuildTiming, error) {
	source := prepared.source
	timing := contracts.FilterSourceBuildTiming{
		SourceKind: string(source.Kind),
		SourceDir:  source.Directory,
	}
	inspected, err := inspectFilterSource(prepared, 0, true)
	if err != nil {
		audit.MustAppend("filter_discovery_error", map[string]any{
			"source_kind": string(source.Kind),
			"source_dir":  source.Directory,
			"error":       err.Error(),
		})
		return nil, timing, err
	}
	timing.Definitions = inspected.definitions
	filters := make(map[string]contracts.Filter, len(inspected.filters))
	for tool, entry := range inspected.filters {
		filters[tool] = entry.filter
	}
	timing.Compiled = int64(len(filters))
	audit.MustAppend("filter_discovery", map[string]any{
		"source_kind":    string(source.Kind),
		"source_dir":     source.Directory,
		"definitions":    timing.Definitions,
		"compiled_count": len(filters),
	})
	return filters, timing, nil
}

func inspectCompiledFiltersFromSource(source v2filters.FilterSource, order int) (map[string]compiledStatusFilter, []RegistryStatusRow, error) {
	inspected, err := inspectFilterSource(preparedFilterSource{source: source}, order, false)
	if err != nil {
		return nil, nil, err
	}
	return inspected.filters, inspected.rows, nil
}

func inspectFilterSource(source preparedFilterSource, order int, auditInvalid bool) (inspectedFilterSource, error) {
	paths, err := source.matchedFilterFiles()
	if err != nil {
		return inspectedFilterSource{}, err
	}
	compiled := make(map[string]compiledStatusFilter, len(paths))
	rows := make([]RegistryStatusRow, 0)
	for _, path := range paths {
		raw, err := source.readFile(path)
		if err != nil {
			return inspectedFilterSource{filters: compiled, rows: rows}, fmt.Errorf("read filter %s: %w", path, err)
		}
		spec, err := ParseDefinition(raw)
		if err != nil {
			if auditInvalid {
				audit.MustAppend("filter_definition_invalid", map[string]any{
					"path":  path,
					"error": err.Error(),
				})
			}
			rows = append(rows, RegistryStatusRow{
				Tool:       "-",
				FilterPath: path,
				SourceKind: source.source.Kind,
				Status:     "invalid filter: " + err.Error(),
				order:      order,
			})
			continue
		}
		loaded := LoadedFilter{Path: path, Raw: raw, Spec: spec}
		filter, compileErr := compileLoadedFilter(source.source, loaded)
		if compileErr != nil {
			if auditInvalid {
				audit.MustAppend("filter_compile_invalid", map[string]any{
					"path":   loaded.Path,
					"filter": loaded.Spec.Filter,
					"error":  compileErr.Error(),
				})
			}
			rows = append(rows, RegistryStatusRow{
				Tool:       spec.Filter,
				FilterPath: path,
				SourceKind: source.source.Kind,
				Status:     "invalid filter: " + compileErr.Error(),
				order:      order,
			})
			continue
		}
		if previous, ok := compiled[spec.Filter]; ok {
			rows = append(rows, RegistryStatusRow{
				Tool:       previous.tool,
				FilterPath: previous.path,
				SourceKind: source.source.Kind,
				Status:     "overridden",
				order:      order,
			})
		}
		compiled[spec.Filter] = compiledStatusFilter{tool: spec.Filter, path: path, filter: filter}
	}
	return inspectedFilterSource{
		filters:     compiled,
		rows:        rows,
		definitions: int64(len(paths) - countInvalidDefinitionRows(rows)),
	}, nil
}

func countInvalidDefinitionRows(rows []RegistryStatusRow) int {
	count := 0
	for _, row := range rows {
		if row.Tool == "-" && strings.HasPrefix(row.Status, "invalid filter:") {
			count++
		}
	}
	return count
}

func compileLoadedFilter(source v2filters.FilterSource, loaded LoadedFilter) (contracts.Filter, error) {
	if loaded.Spec == nil {
		return nil, nil
	}
	filter, err := NewFilter(loaded.Spec)
	if err != nil {
		return nil, err
	}
	return filter.WithProvenance(filterProvenance(source, loaded.Path, loaded.Raw)), nil
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

func registerMappedFilters(registered map[string]contracts.Filter, filters map[string]contracts.Filter, prepared preparedFilterSource) error {
	source := prepared.source
	mappings, err := prepared.readMappingsFile(filepath.Join(source.Directory, mappingsFileName))
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
	keys := slices.Sorted(maps.Keys(filters))
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
	mappingsPath := filepath.Join(source.Directory, mappingsFileName)
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
	aliases := slices.Sorted(maps.Keys(mappings))
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
		filter, err := compileLoadedFilter(source, candidate)
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
		// id. This matches cmdshape's documented override model and keeps precedence
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
	return parseMappingsFile(path, raw)
}

func (s preparedFilterSource) readMappingsFile(path string) (map[string]string, error) {
	raw, err := s.readFile(path)
	if err != nil {
		return nil, err
	}
	return parseMappingsFile(path, raw)
}

func parseMappingsFile(path string, raw []byte) (map[string]string, error) {
	return filtermappings.Decode(path, raw)
}

func loadFilterDefinitionsFromDir(dir string) ([]LoadedFilter, error) {
	return loadFilterDefinitions(preparedFilterSource{
		source: v2filters.FilterSource{Directory: dir},
	})
}

func loadFilterDefinitions(source preparedFilterSource) ([]LoadedFilter, error) {
	paths, err := source.matchedFilterFiles()
	if err != nil {
		return nil, err
	}

	out := make([]LoadedFilter, 0, len(paths))
	for _, p := range paths {
		raw, err := source.readFile(p)
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
	slices.SortFunc(out, func(left, right LoadedFilter) int {
		return cmp.Compare(left.Path, right.Path)
	})
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
		Hash:       hex.EncodeToString(sum[:]),
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
	slices.Sort(matches)
	return matches, nil
}
