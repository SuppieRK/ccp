package engine

import (
	"fmt"
	"strings"
)

// ToolFilterRegistry stores tool filters by canonical name and alias.
type ToolFilterRegistry struct {
	byTool  map[string]ToolFilter
	byAlias map[string]string
}

// NewToolFilterRegistry creates an empty filter registry.
func NewToolFilterRegistry() *ToolFilterRegistry {
	return &ToolFilterRegistry{
		byTool:  map[string]ToolFilter{},
		byAlias: map[string]string{},
	}
}

// Register adds one filter and validates unique tool/alias ownership.
func (r *ToolFilterRegistry) Register(f ToolFilter) error {
	if r == nil {
		return fmt.Errorf("nil registry")
	}
	if f == nil {
		return fmt.Errorf("nil filter")
	}

	tool := normalizeLookupKey(f.Tool())
	if tool == "" {
		return fmt.Errorf("filter with empty tool")
	}
	if _, exists := r.byTool[tool]; exists {
		return fmt.Errorf("duplicate tool registration: %s", tool)
	}
	r.byTool[tool] = f

	for _, alias := range f.Aliases() {
		key := normalizeLookupKey(alias)
		if key == "" {
			continue
		}
		if existingTool, exists := r.byAlias[key]; exists {
			return fmt.Errorf("duplicate alias registration: %s (tools: %s, %s)", key, existingTool, tool)
		}
		r.byAlias[key] = tool
	}
	return nil
}

// Resolve returns a filter by canonical tool name or alias.
func (r *ToolFilterRegistry) Resolve(name string) ToolFilter {
	if r == nil {
		return nil
	}
	key := name
	if key == "" || !isNormalizedLookupKey(key) {
		key = normalizeLookupKey(key)
	}
	if key == "" {
		return nil
	}
	if f, ok := r.byTool[key]; ok {
		return f
	}
	tool, ok := r.byAlias[key]
	if !ok {
		return nil
	}
	return r.byTool[tool]
}

func normalizeLookupKey(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

func isNormalizedLookupKey(name string) bool {
	return name != "" &&
		strings.TrimSpace(name) == name &&
		strings.ToLower(name) == name
}
