package audit

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAudit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Audit Suite")
}
