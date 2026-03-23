//go:build windows

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("windows process control", func() {
	Describe("configureManagedCommand", func() {
		It("configures a new process group and cancel hook", func() {
			cmd := &exec.Cmd{}

			configureManagedCommand(cmd)

			Expect(cmd.SysProcAttr).NotTo(BeNil())
			Expect(cmd.SysProcAttr.CreationFlags).To(Equal(uint32(syscall.CREATE_NEW_PROCESS_GROUP)))
			Expect(cmd.Cancel).NotTo(BeNil())
		})

		It("returns nil when cancellation runs before a process starts", func() {
			cmd := &exec.Cmd{}
			configureManagedCommand(cmd)

			Expect(cmd.Cancel()).To(Succeed())
		})
	})

	Describe("taskkillExecutablePath", func() {
		It("prefers SystemRoot over other fallbacks", func() {
			originalSystemRoot, hadSystemRoot := os.LookupEnv("SystemRoot")
			originalWindir, hadWindir := os.LookupEnv("WINDIR")
			DeferCleanup(func() {
				if hadSystemRoot {
					Expect(os.Setenv("SystemRoot", originalSystemRoot)).To(Succeed())
				} else {
					Expect(os.Unsetenv("SystemRoot")).To(Succeed())
				}
				if hadWindir {
					Expect(os.Setenv("WINDIR", originalWindir)).To(Succeed())
				} else {
					Expect(os.Unsetenv("WINDIR")).To(Succeed())
				}
			})

			Expect(os.Setenv("SystemRoot", `D:\Windows`)).To(Succeed())
			Expect(os.Setenv("WINDIR", `E:\Windows`)).To(Succeed())

			Expect(taskkillExecutablePath()).To(Equal(filepath.Join(`D:\Windows`, "System32", "taskkill.exe")))
		})

		It("falls back to WINDIR and then the canonical default", func() {
			originalSystemRoot, hadSystemRoot := os.LookupEnv("SystemRoot")
			originalWindir, hadWindir := os.LookupEnv("WINDIR")
			DeferCleanup(func() {
				if hadSystemRoot {
					Expect(os.Setenv("SystemRoot", originalSystemRoot)).To(Succeed())
				} else {
					Expect(os.Unsetenv("SystemRoot")).To(Succeed())
				}
				if hadWindir {
					Expect(os.Setenv("WINDIR", originalWindir)).To(Succeed())
				} else {
					Expect(os.Unsetenv("WINDIR")).To(Succeed())
				}
			})

			Expect(os.Unsetenv("SystemRoot")).To(Succeed())
			Expect(os.Setenv("WINDIR", `E:\Windows`)).To(Succeed())
			Expect(taskkillExecutablePath()).To(Equal(filepath.Join(`E:\Windows`, "System32", "taskkill.exe")))

			Expect(os.Unsetenv("WINDIR")).To(Succeed())
			Expect(taskkillExecutablePath()).To(Equal(filepath.Join(`C:\Windows`, "System32", "taskkill.exe")))
		})
	})
})
