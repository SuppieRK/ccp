package product

import "path/filepath"

const (
	Name       = "cmdshape"
	ProjectDir = ".cmdshape"
	ConfigDir  = "cmdshape"
	Repository = "SuppieRK/cmdshape"
)

func HomeConfigPath(home string) string {
	return filepath.Join(home, ".config", ConfigDir)
}

func ProjectStatePath(root string) string {
	return filepath.Join(root, ProjectDir)
}
