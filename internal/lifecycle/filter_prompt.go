package lifecycle

import (
	_ "embed" // Required by go:embed for the agent-facing filter prompt below.
	"strings"
)

const promptFilterIDPlaceholder = "<filter-id>"
const promptCommandPlaceholder = "<tool> <args...>"

//go:embed filter_prompt.md
var embeddedFilterPrompt string

func renderFilterPrompt(filterID string) string {
	filterID = strings.TrimSpace(filterID)
	command := promptCommandPlaceholder
	if filterID == "" {
		filterID = promptFilterIDPlaceholder
	} else {
		command = filterID + " <args...>"
	}
	out := strings.ReplaceAll(embeddedFilterPrompt, "{{FILTER_ID}}", filterID)
	out = strings.ReplaceAll(out, "{{COMMAND_EXAMPLE}}", command)
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}
