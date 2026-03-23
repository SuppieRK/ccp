//go:build windows

package core

import "os"

func defaultExecutionSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
