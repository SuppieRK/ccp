package projectfiles

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProjectfiles(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Projectfiles Suite")
}
