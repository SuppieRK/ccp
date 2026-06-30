package lifecycle

import (
	_ "embed"
	"strings"
)

const defaultPromptFilterID = "my-tool"

//go:embed filter_prompt.md
var embeddedFilterPrompt string

func renderFilterPrompt(filterID string) string {
	filterID = strings.TrimSpace(filterID)
	if filterID == "" {
		filterID = defaultPromptFilterID
	}
	out := strings.ReplaceAll(embeddedFilterPrompt, "{{FILTER_ID}}", filterID)
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}
