package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/engine"
	"github.com/SuppieRK/cmdshape/internal/filtermappings"
	filteryaml "github.com/SuppieRK/cmdshape/internal/filters/yaml"
	"github.com/SuppieRK/cmdshape/internal/filtertrust"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"
	"github.com/SuppieRK/cmdshape/internal/version"
)

var filterIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func RunFilter(args []string) error {
	return RunFilterWithMetrics(args, "")
}

func RunFilterWithMetrics(args []string, metricsPath string) error {
	if len(args) == 0 {
		return errors.New("missing filter subcommand")
	}
	if args[0] == "--help" || args[0] == "-h" {
		fs := newLifecycleFlagSet("filter")
		setLifecycleUsage(
			fs,
			"YAML filter authoring and inspection helpers",
			[]string{"cmdshape filter <subcommand> [args...]"},
			"subcommands: new, performance, prompt, status, trust, untrust",
			"agents creating or improving filters should start with 'cmdshape filter prompt [name]' for the embedded workflow.",
			"use 'cmdshape filter new --help', 'cmdshape filter performance --help', 'cmdshape filter prompt --help', or 'cmdshape filter status --help' for subcommand details.",
		)
		handled, err := parseLifecycleFlags(fs, args)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}
	switch args[0] {
	case "new":
		return RunFilterNew(args[1:])
	case "performance":
		return RunFilterPerformance(args[1:], metricsPath)
	case "prompt":
		return RunFilterPrompt(args[1:])
	case "status":
		return RunFilterStatus(args[1:])
	case "trust":
		return RunFilterTrust(args[1:])
	case "untrust":
		return RunFilterUntrust(args[1:])
	default:
		return fmt.Errorf("unknown filter subcommand %q", args[0])
	}
}

func RunFilterPrompt(args []string) error {
	fs := newLifecycleFlagSet("filter prompt")
	setLifecycleUsage(
		fs,
		"print an embedded agent prompt for creating or improving filters",
		[]string{"cmdshape filter prompt [name]"},
		"name is optional and must be a lowercase filter id using letters, digits, and hyphens only.",
		"the prompt is embedded in the cmdshape binary and does not depend on repository-local docs.",
		"the prompt instructs agents to copy global filters into the resolved project root's .cmdshape/filters before editing.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if fs.NArg() > 1 {
		return errors.New("expected at most one filter name")
	}

	filterID := ""
	if fs.NArg() == 1 {
		filterID, err = normalizeNewFilterID(fs.Arg(0))
		if err != nil {
			return err
		}
	}

	fmt.Print(renderFilterPrompt(filterID))
	return nil
}

func RunFilterStatus(args []string) error {
	fs := newLifecycleFlagSet("filter status")
	setLifecycleUsage(
		fs,
		"show active, overridden, and broken filter registrations",
		[]string{"cmdshape filter status"},
		"status shows all discovered rows from the current filter sources.",
		"project-local filters override home-scoped filters when both define the same tool or alias.",
		"filter paths are compacted for readability; mappings show their target with '->'.",
		"agents creating or improving filters should run 'cmdshape filter prompt [name]' before editing.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if fs.NArg() != 0 {
		return errors.New("filter status does not accept positional arguments")
	}

	sources := filteryaml.StatusSources()
	projectState := filtertrust.State("")
	var trustDecision filtertrust.Decision
	if version.Version != "dev" {
		trustDecision, _ = filtertrust.Evaluate("")
		projectState = trustDecision.State
	}
	filters, rows, err := filteryaml.LoadRegistryStatusFromSourcesWithProjectState(sources, projectState)
	if err != nil {
		return err
	}
	registry := engine.NewRegistry()
	registry.RegisterAll(filters)

	fmt.Println("cmdshape filter status")
	fmt.Println()
	if version.Version != "dev" {
		fmt.Printf("project trust: %s", trustDecision.State)
		if trustDecision.Root != "" {
			fmt.Printf(" (%s)", compactFilterStatusPath(trustDecision.Root))
		}
		fmt.Println()
		fmt.Println()
	}
	if len(rows) == 0 {
		fmt.Println("No filters found.")
		fmt.Println()
		printFilterPromptHint()
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
	fmt.Println()
	printFilterPromptHint()
	return nil
}

func RunFilterTrust(args []string) error {
	fs := newLifecycleFlagSet("filter trust")
	setLifecycleUsage(
		fs,
		"approve the exact current project filter source",
		[]string{"cmdshape filter trust"},
		"the nearest enclosing Git worktree root is the implicit project target; outside Git, the current directory is used.",
		"approval covers every project YAML filter and .mappings.yaml by path and exact bytes.",
		"any addition, removal, rename, mapping change, or content change requires approval again.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if fs.NArg() != 0 {
		return errors.New("filter trust does not accept positional arguments")
	}
	decision, err := filtertrust.Trust("")
	if err != nil {
		return err
	}
	audit.MustAppend("project_filter_trusted", map[string]any{
		"project_root": decision.Root,
		"digest":       decision.Digest,
	})
	fmt.Printf("cmdshape filter trust: trusted %s\n", decision.Root)
	fmt.Printf("digest: %s\n", decision.Digest)
	return nil
}

func RunFilterUntrust(args []string) error {
	fs := newLifecycleFlagSet("filter untrust")
	setLifecycleUsage(
		fs,
		"remove approval for the current project filter source",
		[]string{"cmdshape filter untrust"},
		"the nearest enclosing Git worktree root is the implicit project target; outside Git, the current directory is used.",
		"project filters remain on disk but are ignored until explicitly trusted again.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if fs.NArg() != 0 {
		return errors.New("filter untrust does not accept positional arguments")
	}
	decision, err := filtertrust.Untrust("")
	if err != nil {
		return err
	}
	audit.MustAppend("project_filter_untrusted", map[string]any{
		"project_root": decision.Root,
		"state":        decision.State,
	})
	fmt.Printf("cmdshape filter untrust: removed approval for %s\n", decision.Root)
	return nil
}

func printFilterPromptHint() {
	fmt.Println("Next: run `cmdshape filter prompt <filter-id>` for the embedded agent workflow before editing or creating filters.")
}

func RunFilterNew(args []string) error {
	fs := newLifecycleFlagSet("filter new")
	setLifecycleUsage(
		fs,
		"generate a commented YAML scaffold for a new filter",
		[]string{"cmdshape filter new <name>"},
		"name must be a lowercase filter id using letters, digits, and hyphens only.",
		"agents should prefer 'cmdshape filter prompt <name>' first when creating or improving filters.",
		"cmdshape writes the scaffold to <project-root>/.cmdshape/filters/<name>.yaml; the project root is the nearest enclosing Git worktree root or the current directory outside Git.",
		"cmdshape also ensures <project-root>/.cmdshape/filters/.mappings.yaml contains an identity mapping for the new filter id.",
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
		return errors.New("expected exactly one filter name")
	}

	filterID, err := normalizeNewFilterID(fs.Arg(0))
	if err != nil {
		return err
	}

	root, err := projectfiles.ResolveProjectRoot("")
	if err != nil {
		return err
	}
	filtersDir := filepath.Join(root, ".cmdshape", "filters")
	if err := os.MkdirAll(filtersDir, 0o755); err != nil {
		return fmt.Errorf("create filters directory: %w", err)
	}

	filterPath := filepath.Join(filtersDir, filterID+".yaml")
	if _, err := os.Stat(filterPath); err == nil {
		return fmt.Errorf("filter scaffold already exists at %s", filterPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check existing scaffold: %w", err)
	}
	if err := projectfiles.AtomicWriteFile(filterPath, []byte(newFilterScaffold(filterID)), 0o644); err != nil {
		return fmt.Errorf("write filter scaffold: %w", err)
	}

	mappingsPath := filepath.Join(filtersDir, ".mappings.yaml")
	if err := ensureIdentityFilterMapping(mappingsPath, filterID); err != nil {
		return err
	}

	fmt.Printf("cmdshape filter new: wrote %s\n", filterPath)
	fmt.Printf("cmdshape filter new: ensured %s maps %s -> %s\n", mappingsPath, filterID, filterID)
	return nil
}

func normalizeNewFilterID(input string) (string, error) {
	filterID := strings.TrimSpace(strings.ToLower(input))
	if filterID == "" {
		return "", errors.New("filter name must not be empty")
	}
	if !filterIDPattern.MatchString(filterID) {
		return "", fmt.Errorf("invalid filter name %q: use lowercase letters, digits, and hyphens only", input)
	}
	return filterID, nil
}

func ensureIdentityFilterMapping(path, filterID string) error {
	mappings := lifecycleMappingsFile{Version: 1, Map: map[string]string{}}
	if raw, err := os.ReadFile(path); err == nil {
		decoded, err := filtermappings.Decode(path, raw)
		if err != nil {
			return fmt.Errorf("read mappings file: %w", err)
		}
		mappings.Map = decoded
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read mappings file: %w", err)
	}

	if existing, ok := mappings.Map[filterID]; ok && strings.TrimSpace(existing) != "" {
		return nil
	}
	mappings.Map[filterID] = filterID
	body, err := marshalLifecycleMappings(mappings)
	if err != nil {
		return fmt.Errorf("write mappings file: %w", err)
	}
	if err := projectfiles.AtomicWriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write mappings file: %w", err)
	}
	return nil
}

func newFilterScaffold(filterID string) string {
	return fmt.Sprintf(`# yaml-language-server: $schema=https://raw.githubusercontent.com/SuppieRK/cmdshape/refs/heads/main/schemas/cmdshape-filter.schema.json
version: 1
filter: %s # canonical id used by .cmdshape/filters/.mappings.yaml, benchmark fixtures, and current filename.
about: TODO describe what this filter should compress
# flags_consuming_next_arg lists tool flags that consume the next argv token when cmdshape
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
# 3. compress_output rewrites stdout/stderr/combined output through the fixed cmdshape DSL
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
	rel, ok := relativeToRoot(root, path)
	if !ok {
		resolvedRoot, rootErr := filepath.EvalSymlinks(root)
		resolvedPath, pathErr := filepath.EvalSymlinks(path)
		if rootErr != nil || pathErr != nil {
			return "", false
		}
		rel, ok = relativeToRoot(resolvedRoot, resolvedPath)
		if !ok {
			return "", false
		}
	}
	if rel == "." {
		return prefix, true
	}
	if prefix == "." {
		return "." + string(filepath.Separator) + rel, true
	}
	return prefix + string(filepath.Separator) + rel, true
}

func relativeToRoot(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
