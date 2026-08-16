//go:build !windows

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/SuppieRK/cmdshape/internal/replay"
)

var _ = Describe("native signal parity", func() {
	DescribeTable("returns the child signal result",
		func(mode string, signal syscall.Signal, expected int) {
			root := GinkgoT().TempDir()
			ready := filepath.Join(root, "ready")
			cmd := exec.Command(os.Args[0], "-test.run=TestCmdshape", "--", "signal-helper")
			cmd.Env = append(os.Environ(),
				"CMDSHAPE_SIGNAL_HELPER="+mode,
				"CMDSHAPE_SIGNAL_READY="+ready,
				"CMDSHAPE_SIGNAL_CAPTURE_DIR="+filepath.Join(root, "capture"),
				"HOME="+root,
			)
			var stderr bytes.Buffer
			cmd.Stdout = io.Discard
			cmd.Stderr = &stderr
			Expect(cmd.Start()).To(Succeed())
			Eventually(ready, time.Second).Should(BeAnExistingFile())
			Expect(cmd.Process.Signal(signal)).To(Succeed())

			err := cmd.Wait()
			if expected == 0 {
				Expect(err).NotTo(HaveOccurred(), stderr.String())
			} else {
				exitErr, ok := errors.AsType[*exec.ExitError](err)
				Expect(ok).To(BeTrue())
				Expect(exitErr.ExitCode()).To(Equal(expected))
			}
			if mode == "capture" {
				Expect(filepath.Join(root, "capture", replay.CommandFileName)).To(BeAnExistingFile())
			}
		},
		Entry("filtered child terminated by SIGTERM", "filtered", syscall.SIGTERM, 143),
		Entry("filtered child terminated by SIGINT", "filtered", syscall.SIGINT, 130),
		Entry("raw child terminated by SIGTERM", "raw", syscall.SIGTERM, 143),
		Entry("child trapping SIGTERM and exiting zero", "trap-zero", syscall.SIGTERM, 0),
		Entry("interrupted capture", "capture", syscall.SIGTERM, 143),
	)
})

func init() {
	mode := os.Getenv("CMDSHAPE_SIGNAL_HELPER")
	if mode == "" {
		return
	}
	ready := os.Getenv("CMDSHAPE_SIGNAL_READY")
	script := `echo ready > "$1"; exec sleep 30`
	if mode == "trap-zero" {
		script = `trap 'exit 0' TERM; echo ready > "$1"; while :; do sleep .05; done`
	}
	args := []string{"cmdshape"}
	switch mode {
	case "raw":
		args = append(args, "--raw")
	case "capture":
		args = append(args, "capture", "--dir", os.Getenv("CMDSHAPE_SIGNAL_CAPTURE_DIR"), "--")
	}
	os.Args = append(args, "sh", "-c", script, "sh", ready)
	main()
}
