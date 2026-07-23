package cli

import (
	"fmt"
	"slices"
	"strings"
)

func LifecycleCommands() []string {
	commands := make([]string, 0, len(lifecycleCommands)+len(filterCommands))
	for command := range lifecycleCommands {
		commands = append(commands, command)
	}
	for command := range filterCommands {
		commands = append(commands, command)
	}
	slices.Sort(commands)
	return commands
}

func ExecutionFlags() []string {
	return []string{"--confidential", "--help", "--raw", "--version", "-h"}
}

var lifecycleCommands = map[string]struct{}{
	"capture":   {},
	"init":      {},
	"gain":      {},
	"history":   {},
	"recovery":  {},
	"verify":    {},
	"upgrade":   {},
	"uninstall": {},
	"repair":    {},
}

var filterCommands = map[string]struct{}{
	"filter": {},
}

type Options struct {
	ShowHelp bool

	ShowVersion bool

	Raw bool

	ConfidentialRedactions []string

	CommandArgs []string
}

func Parse(args []string) (Options, error) {
	opts := Options{}

	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			opts.CommandArgs = args[i+1:]
			return finalizeParsedOptions(opts)
		}
		if done, nextIndex, err := parseFlagArg(args, i, &opts); done {
			if err != nil {
				return Options{}, err
			}
			i = nextIndex
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			return Options{}, fmt.Errorf("unknown flag: %s", args[i])
		}
		opts.CommandArgs = args[i:]
		return finalizeParsedOptions(opts)
	}

	return finalizeParsedOptions(opts)
}

func parseFlagArg(args []string, index int, opts *Options) (bool, int, error) {
	switch args[index] {
	case "--help", "-h":
		opts.ShowHelp = true
		return true, index, nil
	case "--version":
		opts.ShowVersion = true
		return true, index, nil
	case "--raw":
		opts.Raw = true
		return true, index, nil
	case "--confidential":
		value, next, err := requireFlagValue(args, index, "--confidential")
		if err != nil {
			return true, index, err
		}
		opts.ConfidentialRedactions = parseConfidentialRedactions(value)
		return true, next, nil
	default:
		return false, index, nil
	}
}

func finalizeParsedOptions(opts Options) (Options, error) {
	if opts.ShowHelp {
		return opts, nil
	}
	if err := validateExecutionFlagScope(opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

func requireFlagValue(args []string, index int, flag string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("missing value for %s", flag)
	}
	return args[index+1], index + 1, nil
}

func validateExecutionFlagScope(opts Options) error {
	if len(opts.CommandArgs) == 0 {
		return nil
	}
	if !IsManagedCommand(opts.CommandArgs[0]) {
		return nil
	}
	if opts.Raw {
		return fmt.Errorf("--raw is only valid for execution commands")
	}
	if len(opts.ConfidentialRedactions) > 0 {
		return fmt.Errorf("--confidential is only valid for execution commands")
	}
	return nil
}

func parseConfidentialRedactions(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func IsLifecycleCommand(token string) bool {
	_, ok := lifecycleCommands[normalizeToken(token)]
	return ok
}

func IsFilterCommand(token string) bool {
	_, ok := filterCommands[normalizeToken(token)]
	return ok
}

func IsManagedCommand(token string) bool {
	return IsLifecycleCommand(token) || IsFilterCommand(token)
}

func IsManagedArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return IsManagedCommand(args[0])
}

func ShouldSkipMetrics(tool string, args []string) bool {
	if strings.TrimSpace(tool) != "ccp" {
		return false
	}
	if len(args) < 2 {
		return false
	}
	return IsManagedCommand(args[1])
}

type ExecutionShape struct {
	UsesShell   bool
	HasPipeline bool
	HasChain    bool
	HasFindExec bool
	HasXargs    bool
	NestedCCP   bool
}

func DescribeExecutionShape(args []string) ExecutionShape {
	shape := ExecutionShape{}
	if len(args) == 0 {
		return shape
	}

	shape.UsesShell = isShellCommand(args)
	if shape.UsesShell {
		script := strings.Join(args[2:], " ")
		shape.HasPipeline = strings.Contains(script, "|")
		shape.HasChain = strings.Contains(script, "&&") || strings.Contains(script, "||") || strings.Contains(script, ";")
		shape.HasFindExec = strings.Contains(script, "find ") && strings.Contains(script, "-exec")
		shape.HasXargs = strings.Contains(script, "xargs")
		shape.NestedCCP = strings.Contains(script, "ccp ")
		return shape
	}

	for idx, arg := range args[1:] {
		switch arg {
		case "|":
			shape.HasPipeline = true
		case "&&", "||", ";":
			shape.HasChain = true
		case "-exec":
			shape.HasFindExec = true
		case "xargs":
			shape.HasXargs = true
		case "ccp":
			shape.NestedCCP = true
		}
		if args[idx] == "find" && arg == "-exec" {
			shape.HasFindExec = true
		}
	}

	return shape
}

func normalizeToken(token string) string {
	return strings.TrimSpace(strings.ToLower(token))
}

func isShellCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch normalizeToken(args[0]) {
	case "sh", "bash", "zsh":
		return strings.Contains(args[1], "c")
	default:
		return false
	}
}
