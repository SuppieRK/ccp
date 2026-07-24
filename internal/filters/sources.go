package filters

import "path/filepath"

type SourceKind string

const (
	SourceRepository SourceKind = "repository"
	SourceProject    SourceKind = "project"
	SourceHome       SourceKind = "home"
)

type FilterSource struct {
	Kind      SourceKind
	Directory string
}

func RepositorySource(root string) FilterSource {
	return FilterSource{
		Kind:      SourceRepository,
		Directory: filepath.Join(root, "filters"),
	}
}

func ProjectSource(root string) FilterSource {
	return FilterSource{
		// ProjectSource intentionally points at ./.cmdshape/filters so repos can ship
		// repo-specific YAML overrides. README documents this as the project-local scope
		// that takes precedence over home-scoped filters.
		Kind:      SourceProject,
		Directory: filepath.Join(root, ".cmdshape", "filters"),
	}
}

func HomeSource(home string) FilterSource {
	return FilterSource{
		Kind:      SourceHome,
		Directory: filepath.Join(home, ".config", "cmdshape", "filters"),
	}
}
