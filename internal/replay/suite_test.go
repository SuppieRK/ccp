package replay

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReplay(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Replay Suite")
}
