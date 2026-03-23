//go:build !windows

package core

import (
	"os"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("unix execution signals", func() {
	It("includes interrupt and terminate signals", func() {
		Expect(defaultExecutionSignals()).To(Equal([]os.Signal{os.Interrupt, syscall.SIGTERM}))
	})
})
