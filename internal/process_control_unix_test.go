//go:build !windows

package core

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("unix process control", func() {
	Describe("configureManagedCommand", func() {
		It("configures a separate process group and cancel hook", func() {
			cmd := &exec.Cmd{}

			configureManagedCommand(cmd)

			Expect(cmd.SysProcAttr).NotTo(BeNil())
			Expect(cmd.SysProcAttr.Setpgid).To(BeTrue())
			Expect(cmd.Cancel).NotTo(BeNil())
		})

		It("returns nil when cancellation runs before a process starts", func() {
			cmd := &exec.Cmd{}
			configureManagedCommand(cmd)

			Expect(cmd.Cancel()).To(Succeed())
		})
	})
})
