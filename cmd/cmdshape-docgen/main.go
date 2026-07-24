package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/cli"
	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"
)

func main() {
	body, err := renderGeneratedFacts()
	if err != nil {
		panic(err)
	}
	path := filepath.Join("docs", "generated", "CLI_FACTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		panic(err)
	}
}

func renderGeneratedFacts() ([]byte, error) {
	adapters, err := agents.NewBuiltInAdapters()
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	body.WriteString("# Generated cmdshape CLI facts\n\n")
	body.WriteString("This file is generated from runtime command and integration metadata. Run `go run ./cmd/cmdshape-docgen` after changing either inventory.\n\n")
	body.WriteString("## Execution flags\n\n")
	for _, flag := range cli.ExecutionFlags() {
		fmt.Fprintf(&body, "- `%s`\n", flag)
	}
	body.WriteString("\n## Lifecycle commands\n\n")
	for _, command := range cli.LifecycleCommands() {
		fmt.Fprintf(&body, "- `%s`\n", command)
	}
	body.WriteString("\n## Supported agent integrations\n\n")
	for _, integration := range agents.SupportedTools(adapters) {
		fmt.Fprintf(&body, "- `%s`\n", integration)
	}
	return []byte(body.String()), nil
}
