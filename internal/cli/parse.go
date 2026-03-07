package cli

import (
	"fmt"
	"strings"
)

// Options is the parsed command-line configuration for ccp.
type Options struct {
	// ShowHelp prints usage/help and exits without running a command.
	ShowHelp bool
	// ShowVersion prints version and exits without running a command.
	ShowVersion bool
	// Raw bypasses semantic compaction for wrapped execution commands.
	Raw bool
	// CaptureRaw runs in raw mode and writes timestamped stdout/stderr capture files.
	CaptureRaw bool
	// CaptureRawDir sets output directory for --capture-raw files.
	CaptureRawDir string
	// ConfidentialRedactions are comma-separated substrings replaced with "***" in capture files.
	ConfidentialRedactions []string
	// DebugFilter emits filter metadata while writing compacted output.
	DebugFilter bool
	// CommandArgs are the command and arguments forwarded to runner/lifecycle logic.
	CommandArgs []string
}

// Parse reads CLI flags and returns normalized options.
func Parse(args []string) (Options, error) {
	opts := Options{}

	for i := 0; i < len(args); i++ {
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
	case "--capture-raw":
		opts.Raw = true
		opts.CaptureRaw = true
		return true, index, nil
	case "--capture-raw-dir":
		value, next, err := requireFlagValue(args, index, "--capture-raw-dir")
		if err != nil {
			return true, index, err
		}
		opts.CaptureRawDir = value
		return true, next, nil
	case "--confidential":
		value, next, err := requireFlagValue(args, index, "--confidential")
		if err != nil {
			return true, index, err
		}
		opts.ConfidentialRedactions = parseConfidentialRedactions(value)
		return true, next, nil
	case "--debug-filter":
		opts.DebugFilter = true
		return true, index, nil
	default:
		return false, index, nil
	}
}

func finalizeParsedOptions(opts Options) (Options, error) {
	if opts.ShowHelp {
		return opts, nil
	}
	if err := validateRawScope(opts); err != nil {
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

func validateRawScope(opts Options) error {
	if opts.CaptureRawDir != "" && !opts.CaptureRaw {
		return fmt.Errorf("--capture-raw-dir requires --capture-raw")
	}
	if len(opts.ConfidentialRedactions) > 0 && !opts.CaptureRaw {
		return fmt.Errorf("--confidential requires --capture-raw")
	}
	if !opts.Raw || len(opts.CommandArgs) == 0 {
		return nil
	}

	switch strings.TrimSpace(strings.ToLower(opts.CommandArgs[0])) {
	case "init", "gain", "history", "upgrade", "uninstall":
		return fmt.Errorf("--raw is only valid for execution commands")
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
