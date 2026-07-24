package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCmdshapeCI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cmdshape CI Suite")
}
