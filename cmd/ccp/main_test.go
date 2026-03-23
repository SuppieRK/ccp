package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go-command-compression-proxy/internal/cli"
)

var _ = Describe("ccp main", func() {
	Describe("buildRuntime", func() {
		It("builds the YAML-backed runtime by default", func() {
			r, err := buildRuntime(cli.Options{CommandArgs: []string{"ls"}})

			Expect(err).NotTo(HaveOccurred())
			Expect(r).NotTo(BeNil())
		})
	})

	Describe("usageText", func() {
		It("includes the expected usage sections", func() {
			got := usageText()

			Expect(got).NotTo(BeEmpty())
			Expect(got).To(ContainSubstring("ccp - command compression proxy for coding-agent workflows"))
			Expect(got).To(ContainSubstring("Usage:"))
			Expect(got).To(ContainSubstring("Execution flags:"))
			Expect(got).To(ContainSubstring("Lifecycle commands:"))
			Expect(got).To(ContainSubstring("Notes:"))
			Expect(got).To(ContainSubstring("--confidential"))
			Expect(got).To(ContainSubstring("capture               Write command.yaml, sequenced streams, and replay output artifacts"))
			Expect(got).To(ContainSubstring("init"))
			Expect(got).To(ContainSubstring("filter                YAML filter authoring helpers"))
			Expect(got).To(ContainSubstring("verify                Replay one fixture directory through the current filter"))
			Expect(got).To(ContainSubstring("gain                  Show token savings summary and recent proof output (--global supported)"))
			Expect(got).To(ContainSubstring("history               Show recorded command history (--global supported)"))
			Expect(got).To(ContainSubstring("uninstall             Remove selected integrations or fully uninstall ccp"))
			Expect(got).To(ContainSubstring("Run ccp gain after install or init to verify savings on real work."))
			Expect(got).To(ContainSubstring("--raw preserves native output unless --confidential is also used."))
		})
	})

	Describe("main", func() {
		var (
			cmd    *exec.Cmd
			stderr bytes.Buffer
		)

		BeforeEach(func() {
			cmd = nil
			stderr.Reset()
		})

		Context("when invoked without an execution command", func() {
			It("prints usage and exits non-zero", func() {
				runMainUsageHelperIfRequested()
				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper")
				cmd.Env = append(os.Environ(), "CCP_MAIN_TEST_HELPER=1")
				cmd.Stderr = &stderr

				err := cmd.Run()
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
				Expect(exitErr.ExitCode()).NotTo(Equal(0))
				Expect(stderr.String()).To(ContainSubstring("Usage:"))
				Expect(stderr.String()).To(ContainSubstring("Execution flags:"))
				Expect(stderr.String()).To(ContainSubstring("Lifecycle commands:"))
			})
		})

		Context("when invoked with lifecycle help", func() {
			It("routes history help through lifecycle dispatch", func() {
				handled, err := runLifecycleCommand([]string{"history", "--help"})

				Expect(err).NotTo(HaveOccurred())
				Expect(handled).To(BeTrue())
			})
		})

		Context("when audit setup is blocked", func() {
			It("still reaches the installed entrypoint smoke command", func() {
				runMainSmokeHelperIfRequested()

				home := GinkgoT().TempDir()
				Expect(os.WriteFile(home+string(os.PathSeparator)+".config", []byte("block"), 0o644)).To(Succeed())

				cmd = exec.Command(os.Args[0], "-test.run=TestCCP", "--", "helper-smoke")
				cmd.Env = append(os.Environ(),
					"CCP_MAIN_TEST_HELPER_SMOKE=1",
					"HOME="+home,
					"USERPROFILE="+home,
				)
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				Expect(cmd.Run()).To(Succeed())
				Expect(stdout.String()).To(ContainSubstring("smoke-ok"))
			})
		})
	})

	It("classifies lifecycle commands separately from wrapped execution", func() {
		Expect(cli.IsManagedArgs([]string{"capture", "--", "echo", "hi"})).To(BeTrue())
		Expect(cli.IsManagedArgs([]string{"init"})).To(BeTrue())
		Expect(cli.IsManagedArgs([]string{"repair"})).To(BeTrue())
		Expect(cli.IsManagedArgs([]string{"filter", "new", "demo"})).To(BeTrue())
		Expect(cli.IsManagedArgs([]string{"uninstall"})).To(BeTrue())
		Expect(cli.IsManagedArgs([]string{"pwd"})).To(BeFalse())
		Expect(cli.IsManagedArgs([]string{"echo", "hi"})).To(BeFalse())
		Expect(cli.IsManagedArgs(nil)).To(BeFalse())
	})
})

func runMainUsageHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER") != "1" {
		return
	}

	os.Args = []string{"ccp"}
	main()
}

func runMainSmokeHelperIfRequested() {
	if os.Getenv("CCP_MAIN_TEST_HELPER_SMOKE") != "1" {
		return
	}

	if runtime.GOOS == "windows" {
		os.Args = []string{"ccp", "cmd", "/c", "echo", "smoke-ok"}
	} else {
		os.Args = []string{"ccp", "sh", "-c", "printf 'smoke-ok\n'"}
	}
	main()
	os.Exit(0)
}
