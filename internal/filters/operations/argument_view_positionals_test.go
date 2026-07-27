package operations_test

import (
	"github.com/SuppieRK/cmdshape/internal/filters/operations"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ArgumentView positional matching", func() {
	DescribeTable("matches without reparsing argv",
		func(args, valueFlags, disallowed []string, noPositionals, allowLeading, lacksAny, hasNoPositionals bool) {
			view := operations.ParseArguments(args, valueFlags)

			Expect(view.MatchesPositionalsLackAny(disallowed)).To(Equal(lacksAny))
			Expect(view.MatchesNoPositionals(noPositionals, allowLeading)).To(Equal(hasNoPositionals))
		},
		Entry("ignores a leading command", []string{"branch", "--all"}, nil, []string{"feature"}, true, true, true, true),
		Entry("detects a trailing positional", []string{"branch", "feature"}, nil, []string{"feature"}, true, true, false, false),
		Entry("ignores standalone flag values", []string{"-C", "repo", "status"}, []string{"-C"}, []string{"repo"}, false, false, true, true),
		Entry("retains positionals after the separator", []string{"run", "--", "-n"}, nil, []string{"-n"}, true, true, false, false),
	)
})
