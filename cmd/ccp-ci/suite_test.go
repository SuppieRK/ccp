package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCCPCI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CCP CI Suite")
}
