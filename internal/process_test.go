package core

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("process helpers", func() {
	Describe("DefaultExecutionContext", func() {
		It("returns a cancelable context when the parent is nil", func() {
			var parent context.Context
			ctx, cancel := DefaultExecutionContext(parent)
			DeferCleanup(cancel)

			Expect(ctx).NotTo(BeNil())
			cancel()
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(Equal(context.Canceled))
		})

		It("cancels when the parent context is canceled", func() {
			parent, parentCancel := context.WithCancel(context.Background())
			DeferCleanup(parentCancel)
			ctx, cancel := DefaultExecutionContext(parent)
			DeferCleanup(cancel)

			parentCancel()
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(Equal(context.Canceled))
		})
	})

	Describe("runnerContext", func() {
		It("wraps an explicit parent context", func() {
			parent, parentCancel := context.WithCancel(context.Background())
			DeferCleanup(parentCancel)
			ctx, cancel := runnerContext(parent)
			DeferCleanup(cancel)

			parentCancel()
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(Equal(context.Canceled))
		})

		It("creates a default execution context when no parent is provided", func() {
			var parent context.Context
			ctx, cancel := runnerContext(parent)
			DeferCleanup(cancel)

			Expect(ctx).NotTo(BeNil())
			cancel()
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(Equal(context.Canceled))
		})
	})

	Describe("CommandWithPipesContext", func() {
		It("returns command pipes with stdin attached", func() {
			name, args := successCommand()
			var parent context.Context

			cmd, stdout, stderr, err := CommandWithPipesContext(parent, name, args)

			Expect(err).NotTo(HaveOccurred())
			Expect(cmd.Stdin).To(Equal(os.Stdin))
			Expect(stdout).NotTo(BeNil())
			Expect(stderr).NotTo(BeNil())
			closePipes(stdout, stderr)
		})
	})
})
