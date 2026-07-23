package filtertrust

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFilterTrust(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filter Trust Suite")
}
