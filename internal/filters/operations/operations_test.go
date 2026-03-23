package operations_test

import (
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/filters/operations"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ScopeForStream", func() {
	It("returns the direct stream scope when present", func() {
		scope, ok := operations.ScopeForStream(contracts.StreamStdout, new("combined"), new("stdout"), nil)
		Expect(ok).To(BeTrue())
		Expect(*scope).To(Equal("stdout"))
	})

	It("falls back to the combined scope", func() {
		scope, ok := operations.ScopeForStream(contracts.StreamStderr, new("combined"), nil, nil)
		Expect(ok).To(BeTrue())
		Expect(*scope).To(Equal("combined"))
	})

	It("returns false when no scope is available", func() {
		scope, ok := operations.ScopeForStream[string](contracts.StreamStdout, nil, nil, nil)
		Expect(ok).To(BeFalse())
		Expect(scope).To(BeNil())
	})
})

var _ = Describe("Predicate helpers", func() {
	DescribeTable("match argv shapes conjunctively through the focused helpers",
		func(name string, actual bool) {
			Expect(actual).To(BeTrue(), name)
		},
		Entry("MatchesFirstIs", "MatchesFirstIs", operations.MatchesFirstIs([]string{"test", "--watch"}, "test")),
		Entry("MatchesFirstIn", "MatchesFirstIn", operations.MatchesFirstIn([]string{"build"}, []string{"test", "build"})),
		Entry("MatchesHaveAny", "MatchesHaveAny", operations.MatchesHaveAny([]string{"--watch", "--run"}, []string{"--watch"})),
		Entry("MatchesLackAny", "MatchesLackAny", operations.MatchesLackAny([]string{"--watch"}, []string{"--scan"})),
		Entry("MatchesHaveSequence", "MatchesHaveSequence", operations.MatchesHaveSequence([]string{"-m", "pytest", "-q"}, []string{"-m", "pytest"})),
		Entry("MatchesHaveShortFlag", "MatchesHaveShortFlag", operations.MatchesHaveShortFlag([]string{"-it"}, []string{"-t"})),
		Entry("MatchesNotHaveShortFlag", "MatchesNotHaveShortFlag", operations.MatchesNotHaveShortFlag([]string{"-it"}, []string{"-x"})),
		Entry("MatchesHaveAllShortFlags", "MatchesHaveAllShortFlags", operations.MatchesHaveAllShortFlags([]string{"-lR"}, []string{"-l", "-R"})),
		Entry("MatchesNotHaveAllShortFlags", "MatchesNotHaveAllShortFlags", operations.MatchesNotHaveAllShortFlags([]string{"-l"}, []string{"-l", "-R"})),
		Entry("MatchesPositionalsLackAny", "MatchesPositionalsLackAny", operations.MatchesPositionalsLackAny([]string{"test", "--watch", "unit"}, []string{"e2e"}, nil)),
		Entry("MatchesNoPositionals", "MatchesNoPositionals", operations.MatchesNoPositionals([]string{"branch", "--all"}, nil, true, true)),
	)

	It("rejects disallowed positional values", func() {
		Expect(operations.MatchesPositionalsLackAny([]string{"test", "--watch", "e2e"}, []string{"e2e"}, nil)).To(BeFalse())
	})

	It("rejects missing required sequence", func() {
		Expect(operations.MatchesHaveSequence([]string{"-m", "pytest"}, []string{"-q"})).To(BeFalse())
	})

	It("rejects missing required short flags when all are required", func() {
		Expect(operations.MatchesHaveAllShortFlags([]string{"-l"}, []string{"-l", "-R"})).To(BeFalse())
	})

	It("rejects disallowed short flags when any are present", func() {
		Expect(operations.MatchesNotHaveShortFlag([]string{"-it"}, []string{"-t"})).To(BeFalse())
	})

	It("rejects disallowed short flag sets when all are present together", func() {
		Expect(operations.MatchesNotHaveAllShortFlags([]string{"-lR"}, []string{"-l", "-R"})).To(BeFalse())
	})

	It("rejects explicit positionals when none are allowed", func() {
		Expect(operations.MatchesNoPositionals([]string{"branch", "feature"}, nil, true, true)).To(BeFalse())
	})

	It("rejects a single true positional after the executable has already been stripped", func() {
		Expect(operations.MatchesNoPositionals([]string{"feature"}, nil, true, false)).To(BeFalse())
	})

	It("allows a single leading command token when matcher context identifies it", func() {
		Expect(operations.MatchesNoPositionals([]string{"branch"}, nil, true, true)).To(BeTrue())
	})

	It("does not treat common option values as explicit positionals", func() {
		Expect(operations.MatchesPositionalsLackAny([]string{"-C", "repo", "status"}, []string{"repo"}, []string{"-C"})).To(BeTrue())
		Expect(operations.MatchesPositionalsLackAny([]string{"-C", "repo", "status"}, []string{"status"}, []string{"-C"})).To(BeFalse())
		Expect(operations.MatchesNoPositionals([]string{"-C", "repo"}, []string{"-C"}, true, false)).To(BeTrue())
		Expect(operations.MatchesNoPositionals([]string{"-f", "archive.tar"}, []string{"-f"}, true, false)).To(BeTrue())
	})

	It("does not treat values for other value-taking flags as positionals", func() {
		Expect(operations.MatchesPositionalsLackAny([]string{"test", "-run", "TestSmoke"}, []string{"TestSmoke"}, []string{"-run"})).To(BeTrue())
		Expect(operations.MatchesNoPositionals([]string{"test", "-run", "TestSmoke"}, []string{"-run"}, true, true)).To(BeTrue())
	})
})
