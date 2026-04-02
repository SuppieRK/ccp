package lifecycle

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func newLifecycleFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func parseLifecycleFlags(fs *flag.FlagSet, args []string) (bool, error) {
	helpRequested := lifecycleHelpRequested(args)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, err
	}
	return helpRequested, nil
}

func setLifecycleUsage(fs *flag.FlagSet, summary string, usageLines []string, notes ...string) {
	fs.Usage = func() {
		out := fs.Output()
		_, _ = fmt.Fprintf(out, "ccp %s - %s\n\n", fs.Name(), summary)
		_, _ = fmt.Fprintln(out, "Usage:")
		for _, line := range usageLines {
			_, _ = fmt.Fprintf(out, "  %s\n", line)
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Flags:")
		fs.PrintDefaults()
		if len(notes) == 0 {
			return
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Notes:")
		for _, note := range notes {
			_, _ = fmt.Fprintf(out, "  - %s\n", note)
		}
	}
}

func lifecycleHelpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}
