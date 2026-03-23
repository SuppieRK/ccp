//go:build windows

package core

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("windows execution signals", func() {
	It("only includes interrupt", func() {
		Expect(defaultExecutionSignals()).To(Equal([]os.Signal{os.Interrupt}))
	})
})
