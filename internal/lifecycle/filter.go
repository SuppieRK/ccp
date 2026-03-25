package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go-command-compression-proxy/internal/engine"
	filteryaml "go-command-compression-proxy/internal/filters/yaml"

	"gopkg.in/yaml.v3"
)

var filterIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type filterMappingsFile struct {
	Version int               `yaml:"version"`
	Map     map[string]string `yaml:"map"`
}

func RunFilter(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing filter subcommand")
	}
	switch args[0] {
	case "new":
		return RunFilterNew(args[1:])
	case "status":
		return RunFilterStatus(args[1:])
	default:
		return fmt.Errorf("unknown filter subcommand %q", args[0])
	}
}

func RunFilterStatus(args []string) error {
	fs := newLifecycleFlagSet("filter status")
	setLifecycleUsage(
		fs,
		"show active, overridden, and broken filter registrations",
		[]string{"ccp filter status"},
		"status shows all discovered rows from the current filter sources.",
		"project-local filters override home-scoped filters when both define the same tool or alias.",
		"filter paths are compacted for readability; mappings show their target with '->'.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("filter status does not accept positional arguments")
	}

	sources := filteryaml.DefaultSources()
	filters, rows, err := filteryaml.LoadRegistryStatusFromSources(sources)
	if err != nil {
		return err
	}
	registry := engine.NewRegistry()
	registry.RegisterAll(filters)

	fmt.Println("ccp filter status")
	fmt.Println()
	if len(rows) == 0 {
		fmt.Println("No filters found.")
		return nil
	}
	fmt.Printf("showing %d rows\n\n", len(rows))

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		if row.Status == "ok" && !registry.Has(row.Tool) {
			continue
		}
		tableRows = append(tableRows, []string{
			truncateForDisplay(displayFilterStatusTool(row.Tool), 10),
			truncateTailForDisplay(displayFilterStatusPath(row), 38),
			string(row.SourceKind),
			truncateForDisplay(row.Status, 31),
		})
	}

	fmt.Print(renderTextTable([]textTableColumn{
		{header: "TOOL"},
		{header: "FILTER"},
		{header: "SOURCE"},
		{header: "STATUS"},
	}, tableRows))
	return nil
}

func RunFilterNew(args []string) error {
	fs := newLifecycleFlagSet("filter new")
	setLifecycleUsage(
		fs,
		"generate a commented YAML scaffold for a new filter",
		[]string{"ccp filter new <name>"},
		"name must be a lowercase filter id using letters, digits, and hyphens only.",
		"ccp writes the scaffold to ./.ccp/filters/<name>.yaml relative to the current working directory.",
		"ccp also ensures ./.ccp/filters/.mappings.yaml contains an identity mapping for the new filter id.",
		"the generated scaffold is valid YAML and starts in safe passthrough mode until you author real behavior.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("expected exactly one filter name")
	}

	filterID, err := normalizeNewFilterID(fs.Arg(0))
	if err != nil {
		return err
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	filtersDir := filepath.Join(root, ".ccp", "filters")
	if err := os.MkdirAll(filtersDir, 0o755); err != nil {
		return fmt.Errorf("create filters directory: %w", err)
	}

	filterPath := filepath.Join(filtersDir, filterID+".yaml")
	if _, err := os.Stat(filterPath); err == nil {
		return fmt.Errorf("filter scaffold already exists at %s", filterPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check existing scaffold: %w", err)
	}
	if err := os.WriteFile(filterPath, []byte(newFilterScaffold(filterID)), 0o644); err != nil {
		return fmt.Errorf("write filter scaffold: %w", err)
	}

	mappingsPath := filepath.Join(filtersDir, ".mappings.yaml")
	if err := ensureIdentityFilterMapping(mappingsPath, filterID); err != nil {
		return err
	}

	fmt.Printf("ccp filter new: wrote %s\n", filterPath)
	fmt.Printf("ccp filter new: ensured %s maps %s -> %s\n", mappingsPath, filterID, filterID)
	return nil
}

func normalizeNewFilterID(input string) (string, error) {
	filterID := strings.TrimSpace(strings.ToLower(input))
	if filterID == "" {
		return "", fmt.Errorf("filter name must not be empty")
	}
	if !filterIDPattern.MatchString(filterID) {
		return "", fmt.Errorf("invalid filter name %q: use lowercase letters, digits, and hyphens only", input)
	}
	return filterID, nil
}

func ensureIdentityFilterMapping(path, filterID string) error {
	mappings := filterMappingsFile{
		Version: 1,
		Map:     map[string]string{},
	}
	if raw, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(raw, &mappings); err != nil {
			return fmt.Errorf("read mappings file: %w", err)
		}
		if mappings.Version == 0 {
			mappings.Version = 1
		}
		if mappings.Map == nil {
			mappings.Map = map[string]string{}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read mappings file: %w", err)
	}

	if existing, ok := mappings.Map[filterID]; ok && strings.TrimSpace(existing) != "" {
		return nil
	}
	mappings.Map[filterID] = filterID

	ordered := make([]string, 0, len(mappings.Map))
	for key := range mappings.Map {
		ordered = append(ordered, key)
	}
	slices.Sort(ordered)

	var b strings.Builder
	b.WriteString("version: 1\n")
	b.WriteString("map:\n")
	for _, key := range ordered {
		_, _ = fmt.Fprintf(&b, "  %s: %s\n", key, mappings.Map[key])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write mappings file: %w", err)
	}
	return nil
}

func newFilterScaffold(filterID string) string {
	return fmt.Sprintf(`# yaml-language-server: $schema=https://raw.githubusercontent.com/SuppieRK/ccp/refs/heads/main/schemas/ccp-filter.schema.json
version: 1
filter: %s # canonical id used by .ccp/filters/.mappings.yaml, benchmark fixtures, and current filename.
about: TODO describe what this filter should compress
# flags_consuming_next_arg lists tool flags that consume the next argv token when CCP
# decides whether a token is a real positional argument. List split-form flags like
# "-run" for '-run value'; attached forms like '--format=json' are already self-contained.
# flags_consuming_next_arg: ["-run"]

# Cases are evaluated in order - the first matching case wins.
# Keep the default passthrough case until you are ready to author real behavior.
cases:
  - id: passthrough-default
    passthrough: true

# Uncomment and adapt the example below when you are ready to author behavior.
# 1. when_arguments decides when the case should apply
# 2. normalize_command adjusts command arguments only when the filter contract needs it
# 3. compress_output rewrites stdout/stderr/combined output through the fixed CCP DSL
#
# cases:
#   - id: status-summary
#     # Match only "tool status" style invocations and skip help or machine-readable modes.
#     when_arguments:
#       first_is: status
#       lack_any: ["--help", "--porcelain"]
#     # Add flags only when the filter requires a stable command shape.
#     normalize_command:
#       append_if_missing: ["--short"]
#     # Choose one output scope: combined, stdout, or stderr.
#     compress_output:
#       stdout:
#         lines:
#           # skip removes noisy lines before later processing.
#           skip:
#             - starts_with: "hint:"
#           # replace rewrites matching lines into a shorter human-facing summary. Supports capturing groups.
#           replace:
#             - regex: "^M[[:space:]]+(.+)$"
#               to: "modified: $1"
#           # max is the final cap for the rendered scope.
#           max:
#             count: 20
#             print: "... truncated to {{value}} lines"
#
# Authoring reference:
# - flags_consuming_next_arg
#   Tool-level argv metadata shared by all cases. Declare flags that consume the next token
#   so positional-sensitive predicates and append_if_no_positionals behave correctly.
# - cases[].when_arguments
#   Match on argv shape with:
#   first_is, first_in, have_any, lack_any, have_sequence, have_short_flag, positionals_lack_any
# - cases[].normalize_command
#   Mutate argv only through:
#   append_if_missing, add_short_flags
# - cases[].compress_output
#   Pick exactly one view to transform:
#   combined, stdout, stderr
# - cases[].compress_output.<scope>.lines
#   Apply line-oriented transforms:
#   replace, skip, keep, max
# - cases[].compress_output.<scope>.groups
#   Build repeated grouped sections with:
#   id, starts_with, starts_with_regex, matches_regex, variables, group_by, initially, lines, finally
`, filterID)
}

func displayFilterStatusTool(tool string) string {
	if strings.TrimSpace(tool) == "" {
		return "-"
	}
	return tool
}

func displayFilterStatusPath(row filteryaml.RegistryStatusRow) string {
	path := compactFilterStatusPath(row.FilterPath)
	if strings.TrimSpace(row.Target) == "" {
		return path
	}
	return path + " -> " + row.Target
}

func compactFilterStatusPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return path
	}
	if cwd, err := os.Getwd(); err == nil {
		if rel, ok := compactPathFromRoot(path, cwd, "."); ok {
			return rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, ok := compactPathFromRoot(path, home, "~"); ok {
			return rel
		}
	}
	return path
}

func compactPathFromRoot(path, root, prefix string) (string, bool) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	if rel == "." {
		return prefix, true
	}
	if prefix == "." {
		return "." + string(filepath.Separator) + rel, true
	}
	return prefix + string(filepath.Separator) + rel, true
}
