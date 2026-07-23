//go:build linux

package core

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

var _ = Describe("Linux terminal detection", func() {
	It("recognizes a pseudo-terminal and rejects an ordinary pipe", func() {
		master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
		if err != nil {
			Skip(fmt.Sprintf("pseudo-terminal unavailable: %v", err))
		}
		DeferCleanup(master.Close)

		Expect(unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0)).To(Succeed())
		number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
		Expect(err).NotTo(HaveOccurred())
		slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(slave.Close)

		Expect(fileIsTerminal(master)).To(BeTrue())
		Expect(fileIsTerminal(slave)).To(BeTrue())

		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(reader.Close)
		DeferCleanup(writer.Close)
		Expect(fileIsTerminal(reader)).To(BeFalse())
		Expect(fileIsTerminal(writer)).To(BeFalse())
	})
})
