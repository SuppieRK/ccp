package operations_test

import (
	"github.com/SuppieRK/cmdshape/internal/contracts"
	"github.com/SuppieRK/cmdshape/internal/filters/operations"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ScopeForStream", func() {
	It("returns the direct stream scope when present", func() {
		scope, ok := operations.ScopeForStream(contracts.StreamStdout, new("combined"), new("stdout"), nil)
		Expect(ok).To(BeTrue())
		Expect(*scope).To(Equal("stdout"))
	})

	It("returns the stderr scope when present", func() {
		scope, ok := operations.ScopeForStream(contracts.StreamStderr, new("combined"), nil, new("stderr"))
		Expect(ok).To(BeTrue())
		Expect(*scope).To(Equal("stderr"))
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
		Entry("MatchesFirstIs with empty matcher", "MatchesFirstIs", operations.MatchesFirstIs([]string{"test", "--watch"}, "")),
		Entry("MatchesFirstIn with empty options", "MatchesFirstIn", operations.MatchesFirstIn([]string{"build"}, nil)),
		Entry("MatchesHaveAny with empty wants", "MatchesHaveAny", operations.MatchesHaveAny([]string{"--watch", "--run"}, nil)),
		Entry("MatchesLackAny with empty disallowed", "MatchesLackAny", operations.MatchesLackAny([]string{"--watch"}, nil)),
		Entry("MatchesHaveSequence with empty sequence", "MatchesHaveSequence", operations.MatchesHaveSequence([]string{"-m", "pytest", "-q"}, nil)),
		Entry("MatchesHaveShortFlag with empty flags", "MatchesHaveShortFlag", operations.MatchesHaveShortFlag([]string{"-it"}, nil)),
		Entry("MatchesNotHaveShortFlag with empty flags", "MatchesNotHaveShortFlag", operations.MatchesNotHaveShortFlag([]string{"-it"}, nil)),
		Entry("MatchesHaveAllShortFlags with empty flags", "MatchesHaveAllShortFlags", operations.MatchesHaveAllShortFlags([]string{"-lR"}, nil)),
		Entry("MatchesNotHaveAllShortFlags with empty flags", "MatchesNotHaveAllShortFlags", operations.MatchesNotHaveAllShortFlags([]string{"-l"}, nil)),
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

	DescribeTable("rejects focused helper edges deterministically",
		func(actual bool) {
			Expect(actual).To(BeFalse())
		},
		Entry("MatchesFirstIs when args are empty", operations.MatchesFirstIs(nil, "test")),
		Entry("MatchesFirstIn when args are empty", operations.MatchesFirstIn(nil, []string{"test", "build"})),
		Entry("MatchesLackAny when a disallowed arg is present", operations.MatchesLackAny([]string{"--watch"}, []string{"--watch"})),
		Entry("MatchesHaveSequence when the sequence is longer than argv", operations.MatchesHaveSequence([]string{"-m"}, []string{"-m", "pytest"})),
		Entry("MatchesHaveAllShortFlags when one requested flag is not a short flag", operations.MatchesHaveAllShortFlags([]string{"-lR"}, []string{"-l", "--recursive"})),
		Entry("HasExplicitPositionals stays false when only value-flag arguments are present", operations.HasExplicitPositionals([]string{"-C", "repo"}, []string{"-C"})),
	)

	It("rejects disallowed positional values", func() {
		Expect(operations.MatchesPositionalsLackAny([]string{"test", "--watch", "e2e"}, []string{"e2e"}, nil)).To(BeFalse())
	})

	It("rejects missing required sequence", func() {
		Expect(operations.MatchesHaveSequence([]string{"-m", "pytest"}, []string{"-q"})).To(BeFalse())
	})

	It("matches required sequences at exact-length and final-window boundaries", func() {
		Expect(operations.MatchesHaveSequence([]string{"-m", "pytest"}, []string{"-m", "pytest"})).To(BeTrue())
		Expect(operations.MatchesHaveSequence([]string{"go", "test", "./..."}, []string{"test", "./..."})).To(BeTrue())
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

	It("ignores empty, bare-dash, and long-flag argv segments when scanning short flags", func() {
		Expect(operations.MatchesHaveShortFlag([]string{"", "-", "--all", "-xz"}, []string{"-z"})).To(BeTrue())
		Expect(operations.MatchesHaveShortFlag([]string{"", "-", "--all"}, []string{"-z"})).To(BeFalse())
	})

	It("does not treat values for other value-taking flags as positionals", func() {
		Expect(operations.MatchesPositionalsLackAny([]string{"test", "-run", "TestSmoke"}, []string{"TestSmoke"}, []string{"-run"})).To(BeTrue())
		Expect(operations.MatchesNoPositionals([]string{"test", "-run", "TestSmoke"}, []string{"-run"}, true, true)).To(BeTrue())
	})

	It("treats empty argv elements as explicit positionals once option handling is exhausted", func() {
		Expect(operations.HasExplicitPositionals([]string{""}, nil)).To(BeTrue())
		Expect(operations.MatchesNoPositionals([]string{""}, nil, true, false)).To(BeFalse())
	})

	It("detects explicit positionals after stripping value-flag arguments", func() {
		Expect(operations.HasExplicitPositionals([]string{"status"}, nil)).To(BeTrue())
		Expect(operations.HasExplicitPositionals([]string{"-C", "repo"}, []string{"-C"})).To(BeFalse())
	})

	It("treats arguments after -- as explicit positionals even when they look like flags", func() {
		Expect(operations.HasExplicitPositionals([]string{"--", "-n"}, nil)).To(BeTrue())
		Expect(operations.MatchesNoPositionals([]string{"--", "-n"}, nil, true, false)).To(BeFalse())
	})
})

var _ = Describe("ArgumentView", func() {
	DescribeTable("matches equivalent attached and split long-option values",
		func(args []string, matcher []string) {
			view := operations.ParseArguments(args, []string{"--format"})
			Expect(view.MatchesHaveAny(matcher)).To(BeTrue())
			Expect(view.MatchesHaveSequence(matcher)).To(BeTrue())
		},
		Entry("attached argv and attached matcher", []string{"--format=json"}, []string{"--format=json"}),
		Entry("split argv and attached matcher", []string{"--format", "json"}, []string{"--format=json"}),
		Entry("attached argv and split matcher", []string{"--format=json"}, []string{"--format", "json"}),
		Entry("split argv and split matcher", []string{"--format", "json"}, []string{"--format", "json"}),
	)

	It("preserves values containing equals signs and empty values", func() {
		withEquals := operations.ParseArguments([]string{"--format=a=b"}, []string{"--format"})
		Expect(withEquals.HasLongOptionValue("--format", "a=b")).To(BeTrue())
		Expect(withEquals.MatchesHaveSequence([]string{"--format", "a=b"})).To(BeTrue())

		empty := operations.ParseArguments([]string{"--format="}, []string{"--format"})
		Expect(empty.HasLongOptionValue("--format", "")).To(BeTrue())
		Expect(empty.MatchesHaveAny([]string{"--format="})).To(BeTrue())
	})

	It("uses the final repeated option value without losing earlier presence", func() {
		view := operations.ParseArguments(
			[]string{"--format=text", "--format", "json"},
			[]string{"--format"},
		)

		value, ok := view.LastLongOptionValue("--format")
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal("json"))
		Expect(view.HasLongOptionValue("--format", "text")).To(BeTrue())
		Expect(view.HasLongOptionValue("--format", "json")).To(BeTrue())
	})

	It("stops option interpretation at the delimiter", func() {
		view := operations.ParseArguments(
			[]string{"--format=json", "--", "--format=xml", "-v"},
			[]string{"--format"},
		)

		Expect(view.HasLongOptionValue("--format", "json")).To(BeTrue())
		Expect(view.HasLongOptionValue("--format", "xml")).To(BeFalse())
		Expect(view.MatchesHaveAny([]string{"--format=xml"})).To(BeFalse())
		Expect(view.Positionals()).To(Equal([]string{"--format=xml", "-v"}))
	})

	It("does not consume unsupported values or split short-option clusters", func() {
		view := operations.ParseArguments(
			[]string{"--unknown", "value", "-ovalue", "tail"},
			[]string{"--format", "-o"},
		)

		Expect(view.Positionals()).To(Equal([]string{"value", "tail"}))
		Expect(view.HasLongOptionValue("--unknown", "value")).To(BeFalse())
		Expect(view.BeforeSeparator()).To(Equal([]string{"--unknown", "value", "-ovalue", "tail"}))
	})

	It("returns cloned positional and pre-delimiter views", func() {
		args := []string{"--format", "json", "file", "--", "--literal"}
		view := operations.ParseArguments(args, []string{"--format"})
		args[0] = "mutated"

		Expect(view.Positionals()).To(Equal([]string{"file", "--literal"}))
		Expect(view.BeforeSeparator()).To(Equal([]string{"--format", "json", "file"}))
	})
})
