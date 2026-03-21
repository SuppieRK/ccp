package workspaces

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkspaces(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workspaces Suite")
}
