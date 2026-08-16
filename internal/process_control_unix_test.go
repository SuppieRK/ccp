//go:build !windows

package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("unix process control", func() {
	Describe("configureManagedCommand", func() {
		It("configures a separate process group and cancel hook", func() {
			cmd := &exec.Cmd{}

			configureManagedCommand(cmd, context.Background())

			Expect(cmd.SysProcAttr).NotTo(BeNil())
			Expect(cmd.SysProcAttr.Setpgid).To(BeTrue())
			Expect(cmd.Cancel).NotTo(BeNil())
		})

		It("returns nil when cancellation runs before a process starts", func() {
			cmd := &exec.Cmd{}
			configureManagedCommand(cmd, context.Background())

			Expect(cmd.Cancel()).To(Succeed())
		})

		It("kills a running managed process group", func() {
			cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 30")
			configureManagedCommand(cmd, context.Background())

			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Process).NotTo(BeNil())
			Expect(cmd.Cancel()).To(Succeed())
			Expect(cmd.Wait()).To(HaveOccurred())
		})

		It("treats already-exited processes as canceled", func() {
			cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit 0")
			configureManagedCommand(cmd, context.Background())

			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Wait()).To(Succeed())
			Expect(cmd.Cancel()).To(Succeed())
		})

		It("forwards a termination signal that the child traps", func() {
			root := GinkgoT().TempDir()
			ready := filepath.Join(root, "ready")
			trapped := filepath.Join(root, "trapped")
			ctx, cancel := context.WithCancelCause(context.Background())
			cmd := exec.CommandContext(ctx, "sh", "-c", "trap 'echo yes > \"$2\"; exit 42' TERM; echo yes > \"$1\"; while :; do sleep .05; done", "sh", ready, trapped)
			configureManagedCommand(cmd, ctx)
			Expect(cmd.Start()).To(Succeed())
			Eventually(func() error { _, err := os.Stat(ready); return err }).Should(Succeed())

			cancel(executionSignal{signal: syscall.SIGTERM})
			err := cmd.Wait()
			exitErr, ok := errors.AsType[*exec.ExitError](err)
			Expect(ok).To(BeTrue())
			Expect(exitErr.ExitCode()).To(Equal(42))
			Expect(trapped).To(BeAnExistingFile())
		})

		It("kills the process group after the signal grace period", func() {
			root := GinkgoT().TempDir()
			ready := filepath.Join(root, "ready")
			childPIDPath := filepath.Join(root, "child-pid")
			previousGrace := managedSignalGracePeriod
			managedSignalGracePeriod = 100 * time.Millisecond
			DeferCleanup(func() { managedSignalGracePeriod = previousGrace })
			ctx, cancel := context.WithCancelCause(context.Background())
			cmd := exec.CommandContext(ctx, "sh", "-c", "trap '' TERM; sleep 30 & echo $! > \"$2\"; echo yes > \"$1\"; wait", "sh", ready, childPIDPath)
			configureManagedCommand(cmd, ctx)
			Expect(cmd.Start()).To(Succeed())
			Eventually(func() error { _, err := os.Stat(ready); return err }).Should(Succeed())
			body, err := os.ReadFile(childPIDPath)
			Expect(err).NotTo(HaveOccurred())
			childPID, err := strconv.Atoi(strings.TrimSpace(string(body)))
			Expect(err).NotTo(HaveOccurred())

			cancel(executionSignal{signal: syscall.SIGTERM})
			Expect(cmd.Wait()).To(HaveOccurred())
			Eventually(func() bool {
				return errors.Is(syscall.Kill(childPID, 0), syscall.ESRCH)
			}).Should(BeTrue())
		})

		It("preserves trapped exits and maps grace escalation to the forwarded signal", func() {
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(executionSignal{signal: syscall.SIGTERM})
			code, ok := forwardedSignalResult(ctx, 42)
			Expect(ok).To(BeTrue())
			Expect(code).To(Equal(42))
			code, ok = forwardedSignalResult(ctx, 128+int(syscall.SIGKILL))
			Expect(ok).To(BeTrue())
			Expect(code).To(Equal(128 + int(syscall.SIGTERM)))
		})

		It("preserves a filtered child's trapped zero exit", func() {
			root := GinkgoT().TempDir()
			ready := filepath.Join(root, "ready")
			previousTerminal := terminalDescriptorAttached
			terminalDescriptorAttached = func() bool { return false }
			DeferCleanup(func() { terminalDescriptorAttached = previousTerminal })
			ctx, cancel := context.WithCancelCause(context.Background())
			type result struct {
				code int
				err  error
			}
			done := make(chan result, 1)
			go func() {
				code, err := NewRunnerWithOptions(Options{MetricsPath: filepath.Join(root, "gain.db")}).RunContext(ctx, []string{
					"sh", "-c", `trap 'exit 0' TERM; echo ready > "$1"; while :; do sleep .05; done`, "sh", ready,
				})
				done <- result{code: code, err: err}
			}()
			Eventually(ready).Should(BeAnExistingFile())
			cancel(executionSignal{signal: syscall.SIGTERM})

			var got result
			Eventually(done).Should(Receive(&got))
			Expect(got.err).NotTo(HaveOccurred())
			Expect(got.code).To(BeZero())
		})
	})
})
