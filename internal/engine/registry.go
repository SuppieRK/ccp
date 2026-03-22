package engine

import (
	"strings"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	v2filters "go-command-compression-proxy/internal/filters"
)

type Registry struct {
	filters map[string]contracts.Filter
}

func NewRegistry() *Registry {
	return &Registry{
		filters: map[string]contracts.Filter{},
	}
}

func (r *Registry) Register(tool string, filter contracts.Filter) {
	if strings.TrimSpace(tool) == "" || filter == nil {
		return
	}
	r.filters[strings.TrimSpace(tool)] = filter
}

func (r *Registry) RegisterAll(filters map[string]contracts.Filter) {
	for tool, filter := range filters {
		r.Register(tool, filter)
	}
}

func (r *Registry) Resolve(command contracts.Command) contracts.Filter {
	if r == nil {
		audit.MustAppend("filter_fallback", map[string]any{
			"tool":   command.Tool,
			"reason": "registry unavailable",
		})
		return v2filters.Passthrough{}
	}
	if filter, ok := r.filters[strings.TrimSpace(command.Tool)]; ok && filter != nil {
		if cloneable, ok := filter.(contracts.CloneableFilter); ok {
			return cloneable.CloneFilter()
		}
		return filter
	}
	audit.MustAppend("filter_fallback", map[string]any{
		"tool":   command.Tool,
		"reason": "no matching filter",
	})
	return v2filters.Passthrough{}
}
