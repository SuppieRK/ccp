package lifecycle

import (
	"fmt"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/recovery"
)

func RunRecovery(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cmdshape recovery enable|disable|list|purge")
	}
	action := strings.TrimSpace(args[0])
	if action == "--help" || action == "-h" || action == "help" {
		printRecoveryHelp()
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("cmdshape recovery %s does not accept arguments", action)
	}
	return runRecoveryAction(action)
}

func printRecoveryHelp() {
	fmt.Println("cmdshape recovery - manage opt-in bounded raw failure recovery")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  cmdshape recovery enable|disable|list|purge")
	fmt.Println()
	fmt.Println("Recovery is disabled by default and never records raw, passthrough, structured, confidential, terminal-fallback, or zero-byte commands.")
}

func runRecoveryAction(action string) error {
	switch action {
	case "enable":
		if err := recovery.SetEnabled(true); err != nil {
			return err
		}
		fmt.Println("Recovery enabled.")
	case "disable":
		if err := recovery.SetEnabled(false); err != nil {
			return err
		}
		fmt.Println("Recovery disabled.")
	case "list":
		items, err := recovery.List()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No recovery artifacts.")
			return nil
		}
		for _, item := range items {
			fmt.Printf("%s  exit=%d  bytes=%d  %s\n", item.Created.Format("2006-01-02T15:04:05Z"), item.ExitCode, item.Bytes, item.Path)
		}
	case "purge":
		count, err := recovery.Purge()
		if err != nil {
			return err
		}
		fmt.Printf("Purged %d recovery artifacts.\n", count)
	default:
		return fmt.Errorf("unknown recovery action %q", action)
	}
	return nil
}
