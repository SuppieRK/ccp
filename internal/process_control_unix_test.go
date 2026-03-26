//go:build !windows

package core

import (
	"context"
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

		It("kills a running managed process group", func() {
			cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 30")
			configureManagedCommand(cmd)

			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Process).NotTo(BeNil())
			Expect(cmd.Cancel()).To(Succeed())
			Expect(cmd.Wait()).To(HaveOccurred())
		})

		It("treats already-exited processes as canceled", func() {
			cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit 0")
			configureManagedCommand(cmd)

			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Wait()).To(Succeed())
			Expect(cmd.Cancel()).To(Succeed())
		})
	})
})
