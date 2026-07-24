package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCmdshape(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cmdshape Suite")
}
