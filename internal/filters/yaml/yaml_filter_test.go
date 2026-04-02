package yaml

import (
	"regexp"
	"slices"
	"strings"

	"go-command-compression-proxy/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type yamlFilterContext struct {
	args     []string
	stdout   []string
	stderr   []string
	combined []string
	exitCode int
}

func (c yamlFilterContext) Args() []string {
	return c.args
}

func (c yamlFilterContext) BufferedLines(stream contracts.Stream) []string {
	switch stream {
	case contracts.StreamStdout:
		return c.stdout
	case contracts.StreamStderr:
		return c.stderr
	case contracts.StreamCombined:
		if c.combined != nil {
			return c.combined
		}
		return append(append([]string(nil), c.stdout...), c.stderr...)
	default:
		return nil
	}
}

func (c yamlFilterContext) ExitCode() int {
	return c.exitCode
}

var _ = Describe("YamlFilter", func() {
	intPtr := func(v int) *int { return &v }
	stringPtr := func(v string) *string { return &v }

	It("returns nil when cloning a nil filter", func() {
		var filter *YamlFilter

		Expect(filter.CloneFilter()).To(BeNil())
	})

	It("builds a runtime filter from validated schema", func() {
		spec := &FilterDefinition{
			Version: 1,
			Filter:  "gradle",
			Cases: []CaseClause{
				{
					ID: "unsafe_modes",
					WhenArguments: &WhenArguments{
						HaveAny: []string{"--scan"},
					},
					Passthrough: true,
				},
				{
					ID: "default",
					NormalizeCommand: &CommandMutation{
						AppendIfMissing: []string{"--console=auto"},
					},
					CompressOutput: &OutputShape{
						Stdout: &OutputScope{
							Lines: &OutputLines{
								Keep: []SkipOrKeepRule{{Regex: "^FAILURE:"}, {Regex: "^BUILD "}},
								Max:  &MaxRule{Count: 80, Print: "\n{{value}} lines"},
							},
						},
						Stderr: &OutputScope{
							Lines: &OutputLines{
								Skip: []SkipOrKeepRule{{Regex: "^debug:"}},
							},
						},
					},
				},
			},
		}

		filter, err := NewFilter(spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(filter.spec).To(BeIdenticalTo(spec))

		action := filter.OnStdout("BUILD SUCCESSFUL\n", yamlFilterContext{
			args: []string{"gradle", "test"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionKeep))

		action = filter.OnStdout("random line\n", yamlFilterContext{
			args: []string{"gradle", "test"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionIgnore))

		action = filter.OnStdout("plain\n", yamlFilterContext{
			args: []string{"gradle", "test", "--scan"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionEmit))

		action = filter.OnStderr("debug: details\n", yamlFilterContext{
			args: []string{"gradle", "test"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionIgnore))
	})

	It("clones compiled configuration without carrying invocation state", func() {
		spec := &FilterDefinition{
			Version:               1,
			Filter:                "go",
			FlagsConsumingNextArg: []string{"-run"},
			Cases: []CaseClause{{
				ID: "test",
				WhenArguments: &WhenArguments{
					FirstIs: "test",
				},
				NormalizeCommand: &CommandMutation{
					AppendIfNoPositionals: []string{"./..."},
				},
				Variables: []Variable{{
					Name: "count",
					Type: "number",
				}},
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Groups: []OutputGroup{{
							ID:           "files",
							MatchesRegex: `^\./(?P<name>[^/]+)$`,
							Variables: []Variable{
								{Name: "name", Type: "string", RegexGroup: "name"},
							},
							GroupBy:   "all",
							Initially: &OnExit{Print: "files:"},
							Lines: &OutputLines{
								Replace: []ReplaceRule{{
									Regex: `^.*$`,
									To:    stringPtr("  {{name}}"),
									OnMatch: []MatchAction{{
										Variable:  "count",
										Increment: intPtr(1),
									}},
								}},
							},
						}},
					},
				},
				Finally: &OnExit{Print: "count={{count}}"},
			}},
		}

		filter, err := NewFilter(spec)
		Expect(err).NotTo(HaveOccurred())

		ctx := yamlFilterContext{args: []string{"go", "test", "-run", "TestSmoke"}}
		Expect(filter.OnStdout("./main.go\n", ctx).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.cases[0].shared.groups[0].items).To(HaveLen(1))

		cloned, ok := filter.CloneFilter().(*YamlFilter)
		Expect(ok).To(BeTrue())
		Expect(cloned).NotTo(BeIdenticalTo(filter))
		Expect(cloned.cases[0].shared.groups[0].items).To(BeEmpty())
		Expect(cloned.cases[0].shared.activeBoundary).To(BeNil())
		Expect(cloned.cases[0].variables["count"]).To(Equal("0"))

		filter.flagsConsumingNextArg[0] = "-changed"
		filter.cases[0].variables["count"] = "99"
		filter.cases[0].shared.groups[0].items["stale"] = []compiledGroupItem{{line: "./stale.go"}}
		Expect(cloned.cases[0].variables["count"]).To(Equal("0"))

		command, err := cloned.PrepareCommand(contracts.Command{
			Tool: "go",
			Args: []string{"go", "test", "-run", "TestSmoke"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Args).To(Equal([]string{"go", "test", "-run", "TestSmoke", "./..."}))

		exit := cloned.OnStdoutExit(yamlFilterContext{
			args: []string{"go", "test", "-run", "Other"},
		})
		Expect(exit).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "count=0\n",
		}))
	})

	DescribeTable("applies top-level lines.* precedence deterministically",
		func(keep, replace, skip bool, expectedOutput string) {
			lines := &OutputLines{}
			if keep {
				lines.Keep = []SkipOrKeepRule{{StartsWith: "KEEP"}}
			}
			if replace {
				lines.Replace = []ReplaceRule{{StartsWith: "REPLACE", To: stringPtr("REWRITTEN")}}
			}
			if skip {
				lines.Skip = []SkipOrKeepRule{{StartsWith: "SKIP"}}
			}
			filter, err := NewFilter(&FilterDefinition{
				Version: 1,
				Filter:  "demo",
				Cases: []CaseClause{{
					ID: "default",
					CompressOutput: &OutputShape{
						Combined: &OutputScope{
							Lines: lines,
						},
					},
				}},
			})
			Expect(err).NotTo(HaveOccurred())

			var rendered strings.Builder
			for input := range strings.SplitSeq("KEEP\nREPLACE\nSKIP\nvalue\n", "\n") {
				if input == "" {
					continue
				}
				action := filter.OnStdout(input+"\n", yamlFilterContext{args: []string{"demo"}})
				switch action.Kind {
				case contracts.ActionKeep, contracts.ActionEmit, contracts.ActionReplace:
					rendered.WriteString(action.Output)
					if action.Kind != contracts.ActionReplace {
						rendered.WriteString(input + "\n")
					}
				}
			}
			Expect(rendered.String()).To(Equal(expectedOutput))
		},
		Entry("keep, replace, and skip all apply within one stream", true, true, true, "KEEP\nREWRITTEN\n"),
		Entry("keep and replace apply while skip is absent", true, true, false, "KEEP\nREWRITTEN\n"),
		Entry("keep and skip apply while replace is absent", true, false, true, "KEEP\n"),
		Entry("only keep is configured so unmatched lines are ignored", true, false, false, "KEEP\n"),
		Entry("replace and skip apply while unmatched lines still passthrough", false, true, true, "KEEP\nREWRITTEN\nvalue\n"),
		Entry("only replace is configured so non-target lines passthrough", false, true, false, "KEEP\nREWRITTEN\nSKIP\nvalue\n"),
		Entry("only skip is configured so untargeted lines passthrough", false, false, true, "KEEP\nREPLACE\nvalue\n"),
		Entry("no line conditions are present so the stream passthroughs unchanged", false, false, false, "KEEP\nREPLACE\nSKIP\nvalue\n"),
	)

	DescribeTable("renders exit actions deterministically",
		func(output string, stream contracts.Stream, expected contracts.Action) {
			Expect(exitActionForOutput(output, stream)).To(Equal(expected))
		},
		Entry("keeps empty exit output", "", contracts.StreamStdout, contracts.Action{Kind: contracts.ActionKeep}),
		Entry("replaces stdout output without setting a combined stream", "summary\n", contracts.StreamStdout, contracts.Action{Kind: contracts.ActionReplace, Output: "summary\n"}),
		Entry("marks combined exit replacements explicitly", "summary\n", contracts.StreamCombined, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "summary\n"}),
	)

	DescribeTable("appends scope max overflow only when needed",
		func(output string, hidden int, expected string) {
			scope := &compiledScope{
				hidden: hidden,
				max: &compiledMax{
					count: 1,
					print: "\n{{value}} lines",
				},
			}

			Expect(appendScopeMaxOverflow(output, scope)).To(Equal(expected))
		},
		Entry("returns the original output when nothing overflowed", "first\n", 0, "first\n"),
		Entry("joins overflow after a newline-terminated output", "first\n", 2, "first\n\n2 lines"),
		Entry("inserts a newline before overflow when needed", "first", 2, "first\n\n2 lines"),
	)

	It("rejects invalid definitions instead of building partial runtime state", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "npm",
			Cases:   []CaseClause{{ID: "default", CompressOutput: &OutputShape{}}},
		})

		Expect(err).To(MatchError("cases[0].compress_output: output must define at least one scope"))
		Expect(filter).To(BeNil())
	})

	It("clones compiled slices so later schema mutation does not rewrite runtime behavior", func() {
		spec := &FilterDefinition{
			Version: 1,
			Filter:  "npm",
			Cases: []CaseClause{{
				ID: "default",
				WhenArguments: &WhenArguments{
					HaveSequence: []string{"run", "test"},
				},
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Keep: []SkipOrKeepRule{{Regex: "^Done"}},
						},
					},
				},
			}},
		}

		filter, err := NewFilter(spec)
		Expect(err).NotTo(HaveOccurred())

		spec.Cases[0].WhenArguments.HaveSequence[0] = "exec"
		spec.Cases[0].CompressOutput.Combined.Lines.Keep[0].Regex = "^Changed"

		action := filter.OnStdout("Done\n", yamlFilterContext{
			args: []string{"npm", "run", "test"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionKeep))

		action = filter.OnStdout("Changed\n", yamlFilterContext{
			args: []string{"npm", "run", "test"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionIgnore))
	})

	It("caps retained lines when max is configured", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "demo",
			Cases: []CaseClause{{
				ID: "default",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Max: &MaxRule{Count: 1, Print: "\n{{value}} lines"},
						},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		action := filter.OnStdout("second\n", yamlFilterContext{
			args:   []string{"demo"},
			stdout: []string{"first\n"},
		})
		Expect(action.Kind).To(Equal(contracts.ActionIgnore))
	})

	It("caps retained lines silently when max.print is omitted", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "find",
			Cases: []CaseClause{{
				ID: "files",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Groups: []OutputGroup{{
							ID:           "by_parent_dir",
							MatchesRegex: `^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$`,
							Variables: []Variable{
								{Name: "dir", Type: "string", RegexGroup: "dir", DefaultValue: "."},
								{Name: "name", Type: "string", RegexGroup: "name"},
							},
							GroupBy: "{{dir}}",
							Initially: &OnExit{
								Print: "{{dir}}/",
							},
							Lines: &OutputLines{
								Max: &MaxRule{Count: 1},
								Replace: []ReplaceRule{{
									Regex: `^.*$`,
									To:    stringPtr("  {{name}}"),
								}},
							},
						}},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.OnStdout("./pkg/a/a.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/b.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/c.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))

		exit := filter.OnStdoutExit(yamlFilterContext{
			args: []string{"find", ".", "-type", "f"},
		})
		Expect(exit).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "pkg/a/\n  a.go\n",
		}))
	})

	It("summarizes omitted grouped items at scope max from YAML", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "find",
			Cases: []CaseClause{{
				ID: "files",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Max: &MaxRule{
								Count: 5,
								Print: "\n+{{value}} more {{groups_summary}}",
								GroupsSummary: &MaxGroupsSummary{
									Show:      2,
									Print:     "{{key}}/({{count}})",
									Delimiter: ", ",
									Prefix:    "across ",
									Suffix:    " and {{remaining}} other dirs",
								},
							},
						},
						Groups: []OutputGroup{{
							ID:           "by_parent_dir",
							MatchesRegex: `^\./(?P<dir>.+)/(?P<name>[^/]+)$`,
							Variables: []Variable{
								{Name: "dir", Type: "string", RegexGroup: "dir"},
								{Name: "name", Type: "string", RegexGroup: "name"},
							},
							GroupBy: "{{dir}}",
							Initially: &OnExit{
								Print: "{{dir}}/",
							},
							Lines: &OutputLines{
								Replace: []ReplaceRule{{
									Regex: `^.*$`,
									To:    stringPtr("  {{name}}"),
								}},
							},
						}},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		args := []string{"find", ".", "-type", "f"}
		for _, line := range []string{
			"./a/1.go\n",
			"./a/2.go\n",
			"./b/1.go\n",
			"./b/2.go\n",
			"./c/1.go\n",
			"./c/2.go\n",
		} {
			Expect(filter.OnStdout(line, yamlFilterContext{args: args}).Kind).To(Equal(contracts.ActionIgnore))
		}

		exit := filter.OnStdoutExit(yamlFilterContext{args: args})
		Expect(exit).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: strings.Join([]string{
				"a/",
				"  1.go",
				"  2.go",
				"b/",
				"  1.go",
				"+4 more across c/(2), b/(1)",
				"",
			}, "\n"),
		}))
	})

	It("trims empty groups summary placeholders from max print", func() {
		Expect(renderMaxPrint("\n+{{value}} more {{groups_summary}}", 5, "")).To(Equal("\n+5 more"))
	})

	It("rewrites matching lines with replace rules", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "ls",
			Cases: []CaseClause{{
				ID: "long",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Replace: []ReplaceRule{
								{
									Regex: `^d\S+\s+\d+\s+\S+\s+\S+\s+\S+\s+\w+\s+\d+\s+\S+\s+(.+)$`,
									To:    stringPtr("$1/"),
								},
								{
									Regex: `^[\-l]\S+\s+\d+\s+\S+\s+\S+\s+(\S+)\s+\w+\s+\d+\s+\S+\s+(.+)$`,
									To:    stringPtr("$2  $1"),
								},
							},
						},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		dirAction := filter.OnStdout("drwxrwxrwx 1 suppie suppie 4.0K Feb 26 11:50 docs\n", yamlFilterContext{
			args: []string{"ls", "-la"},
		})
		Expect(dirAction).To(Equal(contracts.Action{
			Kind:         contracts.ActionReplace,
			Output:       "docs/\n",
			ReplaceCount: 1,
		}))

		fileAction := filter.OnStdout("-rwxrwxrwx 1 suppie suppie 59 Feb 26 11:50 README.md\n", yamlFilterContext{
			args: []string{"ls", "-la"},
		})
		Expect(fileAction).To(Equal(contracts.Action{
			Kind:         contracts.ActionReplace,
			Output:       "README.md  59\n",
			ReplaceCount: 1,
		}))
	})

	It("replaces whole lines for non-regex replace matchers", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "demo",
			Cases: []CaseClause{{
				ID: "default",
				CompressOutput: &OutputShape{
					Stdout: &OutputScope{
						Lines: &OutputLines{
							Replace: []ReplaceRule{
								{StartsWith: "warning:", To: stringPtr("warning")},
								{Contains: "temporary", To: stringPtr("temp-file")},
								{EndsWith: ".tmp", To: stringPtr("temp-path")},
							},
						},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		startsWithAction := filter.OnStdout("warning: noisy\n", yamlFilterContext{args: []string{"demo"}})
		Expect(startsWithAction).To(Equal(contracts.Action{
			Kind:         contracts.ActionReplace,
			Output:       "warning\n",
			ReplaceCount: 1,
		}))

		containsAction := filter.OnStdout("created temporary directory\n", yamlFilterContext{args: []string{"demo"}})
		Expect(containsAction).To(Equal(contracts.Action{
			Kind:         contracts.ActionReplace,
			Output:       "temp-file\n",
			ReplaceCount: 1,
		}))

		endsWithAction := filter.OnStdout("/tmp/build.tmp\n", yamlFilterContext{args: []string{"demo"}})
		Expect(endsWithAction).To(Equal(contracts.Action{
			Kind:         contracts.ActionReplace,
			Output:       "temp-path\n",
			ReplaceCount: 1,
		}))
	})

	It("rejects malformed replace rules explicitly", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "demo",
			Cases: []CaseClause{{
				ID: "default",
				CompressOutput: &OutputShape{
					Stdout: &OutputScope{
						Lines: &OutputLines{
							Replace: []ReplaceRule{{Regex: "^foo"}},
						},
					},
				},
			}},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("replace rule must define to"))
		Expect(filter).To(BeNil())
	})

	It("applies command mutations for the first matching case", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "ls",
			Cases: []CaseClause{{
				ID: "long",
				WhenArguments: &WhenArguments{
					HaveShortFlag: []string{"-l"},
				},
				NormalizeCommand: &CommandMutation{
					AddShortFlags:         []string{"-a"},
					AppendIfNoPositionals: []string{"."},
				},
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Keep: []SkipOrKeepRule{{Regex: "^"}},
						},
					},
				},
			}},
		})

		Expect(err).NotTo(HaveOccurred())

		command, err := filter.PrepareCommand(contracts.Command{
			Tool: "ls",
			Args: []string{"ls", "-l"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Args).To(Equal([]string{"ls", "-l", "-a", "."}))
		Expect(filter.Dispatch(contracts.Command{
			Tool: "ls",
			Args: []string{"ls", "-l"},
		})).To(Equal("ls|long"))
	})

	It("does not append default positionals when explicit paths are present", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "ls",
			Cases: []CaseClause{{
				ID: "long",
				WhenArguments: &WhenArguments{
					HaveShortFlag: []string{"-l"},
				},
				NormalizeCommand: &CommandMutation{
					AddShortFlags:         []string{"-a"},
					AppendIfNoPositionals: []string{"."},
				},
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Keep: []SkipOrKeepRule{{Regex: "^"}},
						},
					},
				},
			}},
		})

		Expect(err).NotTo(HaveOccurred())

		command, err := filter.PrepareCommand(contracts.Command{
			Tool: "ls",
			Args: []string{"ls", "-l", "filters", "docs"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Args).To(Equal([]string{"ls", "-l", "filters", "docs", "-a"}))
	})

	It("tracks declared variables on match and prints them at exit", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "ls",
			Cases: []CaseClause{{
				ID: "long",
				Variables: []Variable{
					{Name: "dirs", Type: "number"},
					{Name: "files", Type: "number"},
				},
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Replace: []ReplaceRule{
								{
									Regex: `^d\S*(?:\s+\S+){7}\s+(.+)$`,
									To:    stringPtr("$1/"),
									OnMatch: []MatchAction{{
										Variable:  "dirs",
										Increment: intPtr(1),
									}},
								},
								{
									Regex: `^[\-l]\S*(?:\s+\S+){3}\s+(\S+)(?:\s+\S+){3}\s+(.+)$`,
									To:    stringPtr("$2  $1"),
									OnMatch: []MatchAction{{
										Variable:  "files",
										Increment: intPtr(1),
									}},
								},
							},
						},
					},
				},
				Finally: &OnExit{Print: "{{dirs}} dirs, {{files}} files"},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		dirAction := filter.OnStdout("drwxrwxrwx 1 suppie suppie 4.0K Feb 26 11:50 docs\n", yamlFilterContext{
			args: []string{"ls", "-la"},
		})
		Expect(dirAction.Output).To(Equal("docs/\n"))

		fileAction := filter.OnStdout("-rwxrwxrwx 1 suppie suppie 59 Feb 26 11:50 README.md\n", yamlFilterContext{
			args: []string{"ls", "-la"},
		})
		Expect(fileAction.Output).To(Equal("README.md  59\n"))

		exitAction := filter.OnStdoutExit(yamlFilterContext{
			args:   []string{"ls", "-la"},
			stdout: []string{"docs/\n", "README.md  59\n"},
		})
		Expect(exitAction).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "docs/\nREADME.md  59\n1 dirs, 1 files\n",
		}))
	})

	It("restores explicit case variable initial values on invocation reset", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "demo",
			Cases: []CaseClause{{
				ID: "default",
				Variables: []Variable{{
					Name:         "count",
					Type:         "number",
					InitialValue: stringPtr("7"),
				}},
				CompressOutput: &OutputShape{
					Stdout: &OutputScope{
						Lines: &OutputLines{
							Replace: []ReplaceRule{{
								Regex: `^bump$`,
								To:    stringPtr("bump"),
								OnMatch: []MatchAction{{
									Variable:  "count",
									Increment: intPtr(1),
								}},
							}},
						},
					},
				},
				Finally: &OnExit{Print: "count={{count}}"},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.OnStdout("bump\n", yamlFilterContext{args: []string{"demo", "one"}}).Output).To(Equal("bump\n"))
		firstExit := filter.OnStdoutExit(yamlFilterContext{args: []string{"demo", "one"}, stdout: []string{"bump"}})
		Expect(firstExit.Output).To(Equal("bump\ncount=8\n"))

		secondExit := filter.OnStdoutExit(yamlFilterContext{args: []string{"demo", "two"}, stdout: nil})
		Expect(secondExit.Output).To(Equal("count=7\n"))
	})

	It("renders sequential boundary groups in encounter order", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "pytest",
			Cases: []CaseClause{{
				ID: "default",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Groups: []OutputGroup{
							{
								ID:         "failures",
								StartsWith: "=================================== FAILURES",
								Initially:  &OnExit{Print: "failure details:"},
								Lines: &OutputLines{
									Keep: []SkipOrKeepRule{
										{Contains: "AssertionError"},
										{Contains: "Captured stdout call"},
										{StartsWith: "captured stdout call"},
									},
									Replace: []ReplaceRule{
										{Regex: `^E\s+AssertionError:\s+(.+)$`, To: stringPtr("  AssertionError: $1")},
										{Regex: `^(tests/.+)$`, To: stringPtr("  $1")},
										{Regex: `^captured stdout call$`, To: stringPtr("  captured stdout call")},
									},
								},
							},
							{
								ID:         "summary",
								StartsWith: "=========================== short test summary info",
								Initially:  &OnExit{Print: "failed tests:"},
								Lines: &OutputLines{
									Replace: []ReplaceRule{
										{Regex: `^FAILED\s+(.+)$`, To: stringPtr("- $1")},
									},
								},
							},
						},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		ctx := yamlFilterContext{args: []string{"pytest", "-q"}}
		for _, line := range []string{
			"=================================== FAILURES ===================================\n",
			"E       AssertionError: assert {'ok': False} == {'ok': True}\n",
			"tests/test_app.py:10: AssertionError\n",
			"----------------------------- Captured stdout call -----------------------------\n",
			"captured stdout call\n",
			"=========================== short test summary info ============================\n",
			"FAILED tests/test_app.py::test_fail - AssertionError: assert {'ok': False} ==...\n",
		} {
			action := filter.OnStdout(line, ctx)
			Expect(action.Kind).To(Equal(contracts.ActionIgnore))
		}

		exitAction := filter.OnStdoutExit(yamlFilterContext{
			args:   []string{"pytest", "-q"},
			stdout: nil,
		})
		Expect(exitAction.Kind).To(Equal(contracts.ActionReplace))
		Expect(exitAction.Output).To(ContainSubstring("failure details:\n"))
		Expect(exitAction.Output).To(ContainSubstring("AssertionError: assert {'ok': False} == {'ok': True}\n"))
		Expect(exitAction.Output).To(ContainSubstring("tests/test_app.py:10: AssertionError\n"))
		Expect(exitAction.Output).To(ContainSubstring("Captured stdout call"))
		Expect(exitAction.Output).To(ContainSubstring("captured stdout call\n"))
		Expect(exitAction.Output).To(ContainSubstring("failed tests:\n"))
		Expect(exitAction.Output).To(ContainSubstring("- tests/test_app.py::test_fail - AssertionError: assert {'ok': False} ==...\n"))
		Expect(strings.Index(exitAction.Output, "failure details:\n")).To(BeNumerically("<", strings.Index(exitAction.Output, "failed tests:\n")))
	})

	It("uses combined buffered lines for combined exit output", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "demo",
			Cases: []CaseClause{{
				ID: "default",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{Lines: &OutputLines{}},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		exitAction := filter.OnStdoutExit(yamlFilterContext{
			args:     []string{"demo"},
			combined: []string{"out-1\n", "err-1\n"},
		})
		Expect(exitAction).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "out-1\nerr-1\n",
		}))
	})

	It("groups matched find file paths by parent directory at exit", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "find",
			Cases: []CaseClause{{
				ID: "files",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Groups: []OutputGroup{{
							ID:           "by_parent_dir",
							MatchesRegex: `^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$`,
							Variables: []Variable{
								{Name: "dir", Type: "string", RegexGroup: "dir", DefaultValue: "."},
								{Name: "name", Type: "string", RegexGroup: "name"},
							},
							GroupBy: "{{dir}}",
							Initially: &OnExit{
								Print: "{{dir}}/",
							},
							Lines: &OutputLines{
								Skip: []SkipOrKeepRule{{Regex: `^\./\.ccp(?:/|$)`}},
								Replace: []ReplaceRule{{
									Regex: `^.*$`,
									To:    stringPtr("  {{name}}"),
								}},
							},
						}},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.OnStdout("./pkg/b/b.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./main.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/a.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))

		exitAction := filter.OnStdoutExit(yamlFilterContext{
			args: []string{"find", ".", "-type", "f"},
		})
		Expect(exitAction).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "./\n  main.go\npkg/a/\n  a.go\npkg/b/\n  b.go\n",
		}))
	})

	It("prints grouped max overflow before finally output", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "find",
			Cases: []CaseClause{{
				ID: "files",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Groups: []OutputGroup{{
							ID:           "by_parent_dir",
							MatchesRegex: `^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$`,
							Variables: []Variable{
								{Name: "dir", Type: "string", RegexGroup: "dir", DefaultValue: "."},
								{Name: "name", Type: "string", RegexGroup: "name"},
							},
							GroupBy: "{{dir}}",
							Initially: &OnExit{
								Print: "{{dir}}/",
							},
							Lines: &OutputLines{
								Max: &MaxRule{Count: 1, Print: "\n{{value}} lines"},
								Replace: []ReplaceRule{{
									Regex: `^.*$`,
									To:    stringPtr("  {{name}}"),
								}},
							},
							Finally: &OnExit{
								Print: "done",
							},
						}},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.OnStdout("./pkg/a/a.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/b.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/c.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))

		exitAction := filter.OnStdoutExit(yamlFilterContext{
			args: []string{"find", ".", "-type", "f"},
		})
		Expect(exitAction).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "pkg/a/\n  a.go\n\n2 lines\ndone\n",
		}))
	})

	It("suppresses finally output on non-zero exit codes", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "git",
			Cases: []CaseClause{{
				ID: "show",
				Variables: []Variable{
					{Name: "files", Type: "number", InitialValue: stringPtr("0")},
				},
				CompressOutput: &OutputShape{
					Stdout: &OutputScope{
						Lines: &OutputLines{
							Replace: []ReplaceRule{{
								Regex: `^diff --git a/(.+) b/.+$`,
								To:    stringPtr("$1"),
								OnMatch: []MatchAction{{
									Variable:  "files",
									Increment: intPtr(1),
								}},
							}},
						},
					},
				},
				Finally: &OnExit{Print: "summary: {{files}} files changed"},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.OnStdout("diff --git a/tracked.txt b/tracked.txt\n", yamlFilterContext{
			args: []string{"git", "show"},
		}).Output).To(Equal("tracked.txt\n"))

		successExit := filter.OnStdoutExit(yamlFilterContext{
			args:     []string{"git", "show"},
			stdout:   []string{"tracked.txt\n"},
			exitCode: 0,
		})
		Expect(successExit).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Output: "tracked.txt\nsummary: 1 files changed\n",
		}))

		failureExit := filter.OnStdoutExit(yamlFilterContext{
			args:     []string{"git", "show"},
			stdout:   []string{"fatal: not a git repository: '.git'\n"},
			exitCode: 128,
		})
		Expect(failureExit).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Output: "fatal: not a git repository: '.git'\n",
		}))
	})

	It("keeps grouped initial headers on non-zero exit codes", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "pytest",
			Cases: []CaseClause{{
				ID: "default",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Groups: []OutputGroup{{
							ID:         "failures",
							StartsWith: "=================================== FAILURES",
							Initially:  &OnExit{Print: "failure details:"},
							Lines: &OutputLines{
								Keep:    []SkipOrKeepRule{{Contains: "AssertionError:"}},
								Replace: []ReplaceRule{{Regex: `^E\s+(.+)$`, To: stringPtr("AssertionError: $1")}},
							},
						}},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		ctx := yamlFilterContext{args: []string{"pytest", "-q"}}
		for _, line := range []string{
			"=================================== FAILURES ===================================\n",
			"E       AssertionError: assert {'ok': False} == {'ok': True}\n",
		} {
			action := filter.OnStdout(line, ctx)
			Expect(action.Kind).To(Equal(contracts.ActionIgnore))
		}

		failureExit := filter.OnStdoutExit(yamlFilterContext{
			args:     []string{"pytest", "-q"},
			exitCode: 1,
		})
		Expect(failureExit).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "failure details:\nE       AssertionError: assert {'ok': False} == {'ok': True}\n",
		}))
	})

	DescribeTable("applies grouped lines.* precedence deterministically",
		func(keep, replace, skip bool, expected contracts.Action) {
			lines := &OutputLines{}
			if keep {
				lines.Keep = []SkipOrKeepRule{{StartsWith: "./pkg/a/KEEP"}}
			}
			if replace {
				lines.Replace = []ReplaceRule{{StartsWith: "./pkg/a/REPLACE", To: stringPtr("  REWRITTEN")}}
			}
			if skip {
				lines.Skip = []SkipOrKeepRule{{StartsWith: "./pkg/a/SKIP"}}
			}
			filter, err := NewFilter(&FilterDefinition{
				Version: 1,
				Filter:  "find",
				Cases: []CaseClause{{
					ID: "files",
					CompressOutput: &OutputShape{
						Combined: &OutputScope{
							Groups: []OutputGroup{{
								ID:           "by_parent_dir",
								MatchesRegex: `^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$`,
								Variables: []Variable{
									{Name: "dir", Type: "string", RegexGroup: "dir", DefaultValue: "."},
									{Name: "name", Type: "string", RegexGroup: "name"},
								},
								GroupBy:   "{{dir}}",
								Initially: &OnExit{Print: "{{dir}}/"},
								Lines:     lines,
							}},
						},
					},
				}},
			})
			Expect(err).NotTo(HaveOccurred())

			for input := range strings.SplitSeq("./pkg/a/KEEP\n./pkg/a/REPLACE\n./pkg/a/SKIP\n./pkg/a/value\n", "\n") {
				if input == "" {
					continue
				}
				action := filter.OnStdout(input, yamlFilterContext{args: []string{"find", ".", "-type", "f"}})
				Expect(action.Kind).To(Equal(contracts.ActionIgnore))
			}

			exitAction := filter.OnStdoutExit(yamlFilterContext{
				args: []string{"find", ".", "-type", "f"},
			})
			Expect(exitAction).To(Equal(expected))
		},
		Entry("keep, replace, and skip all apply within one grouped stream", true, true, true, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n"}),
		Entry("keep and replace apply while skip is absent in grouped output", true, true, false, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n"}),
		Entry("keep and skip apply while replace is absent in grouped output", true, false, true, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n"}),
		Entry("only keep is configured so unmatched grouped lines are ignored", true, false, false, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n"}),
		Entry("replace and skip apply while unmatched grouped lines still passthrough", false, true, true, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n./pkg/a/value\n"}),
		Entry("only replace is configured so non-target grouped lines passthrough", false, true, false, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n./pkg/a/SKIP\n./pkg/a/value\n"}),
		Entry("only skip is configured so untargeted grouped lines passthrough", false, false, true, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n./pkg/a/REPLACE\n./pkg/a/value\n"}),
		Entry("no grouped line conditions are present so the stream passthroughs unchanged", false, false, false, contracts.Action{Kind: contracts.ActionReplace, Stream: contracts.StreamCombined, Output: "pkg/a/\n./pkg/a/KEEP\n./pkg/a/REPLACE\n./pkg/a/SKIP\n./pkg/a/value\n"}),
	)

	It("applies parent scope max after grouped output is rendered", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "find",
			Cases: []CaseClause{{
				ID: "files",
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Max: &MaxRule{Count: 2, Print: "\n{{value}} lines"},
						},
						Groups: []OutputGroup{{
							ID:           "by_parent_dir",
							MatchesRegex: `^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$`,
							Variables: []Variable{
								{Name: "dir", Type: "string", RegexGroup: "dir", DefaultValue: "."},
								{Name: "name", Type: "string", RegexGroup: "name"},
							},
							GroupBy: "{{dir}}",
							Initially: &OnExit{
								Print: "{{dir}}/",
							},
							Lines: &OutputLines{
								Replace: []ReplaceRule{{
									Regex: `^.*$`,
									To:    stringPtr("  {{name}}"),
								}},
							},
						}},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.OnStdout("./pkg/a/a.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/b.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))
		Expect(filter.OnStdout("./pkg/a/c.go\n", yamlFilterContext{args: []string{"find", ".", "-type", "f"}}).Kind).To(Equal(contracts.ActionIgnore))

		exitAction := filter.OnStdoutExit(yamlFilterContext{
			args: []string{"find", ".", "-type", "f"},
		})
		Expect(exitAction).To(Equal(contracts.Action{
			Kind:   contracts.ActionReplace,
			Stream: contracts.StreamCombined,
			Output: "pkg/a/\n  a.go\n2 lines\n",
		}))
	})

	It("does not mutate commands for unmatched cases", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "ls",
			Cases: []CaseClause{{
				ID: "long",
				WhenArguments: &WhenArguments{
					HaveShortFlag: []string{"-l"},
				},
				NormalizeCommand: &CommandMutation{
					AddShortFlags: []string{"-a"},
				},
				CompressOutput: &OutputShape{
					Combined: &OutputScope{
						Lines: &OutputLines{
							Keep: []SkipOrKeepRule{{Regex: "^"}},
						},
					},
				},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		command, err := filter.PrepareCommand(contracts.Command{
			Tool: "ls",
			Args: []string{"ls"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Args).To(Equal([]string{"ls"}))
		Expect(filter.Dispatch(contracts.Command{
			Tool: "ls",
			Args: []string{"ls"},
		})).To(Equal("ls"))
	})

	It("matches no_positionals cases only when no explicit positionals are present", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version: 1,
			Filter:  "git",
			Cases: []CaseClause{
				{
					ID: "branch-list",
					WhenArguments: &WhenArguments{
						FirstIs:       "branch",
						NoPositionals: true,
					},
					CompressOutput: &OutputShape{
						Combined: &OutputScope{
							Lines: &OutputLines{
								Replace: []ReplaceRule{{
									Regex: `^  (.+)$`,
									To:    stringPtr("$1"),
								}},
							},
						},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.Dispatch(contracts.Command{
			Tool: "git",
			Args: []string{"git", "branch"},
		})).To(Equal("git|branch-list"))
		Expect(filter.Dispatch(contracts.Command{
			Tool: "git",
			Args: []string{"git", "branch", "--all"},
		})).To(Equal("git|branch-list"))
		Expect(filter.Dispatch(contracts.Command{
			Tool: "git",
			Args: []string{"git", "branch", "feature"},
		})).To(Equal("git"))
	})

	It("uses filter-level flags_consuming_next_arg when checking positionals", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version:               1,
			Filter:                "go",
			FlagsConsumingNextArg: []string{"-run"},
			Cases: []CaseClause{
				{
					ID: "test-run",
					WhenArguments: &WhenArguments{
						FirstIs:       "test",
						NoPositionals: true,
					},
					CompressOutput: &OutputShape{
						Combined: &OutputScope{Lines: &OutputLines{Keep: []SkipOrKeepRule{{Regex: "^"}}}},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(filter.Dispatch(contracts.Command{
			Tool: "go",
			Args: []string{"go", "test", "-run", "TestSmoke"},
		})).To(Equal("go|test-run"))
	})

	It("uses filter-level flags_consuming_next_arg for append_if_no_positionals mutations", func() {
		filter, err := NewFilter(&FilterDefinition{
			Version:               1,
			Filter:                "go",
			FlagsConsumingNextArg: []string{"-run"},
			Cases: []CaseClause{
				{
					ID: "test-run",
					WhenArguments: &WhenArguments{
						FirstIs: "test",
					},
					NormalizeCommand: &CommandMutation{
						AppendIfNoPositionals: []string{"./..."},
					},
					CompressOutput: &OutputShape{
						Combined: &OutputScope{Lines: &OutputLines{Keep: []SkipOrKeepRule{{Regex: "^"}}}},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		command, err := filter.PrepareCommand(contracts.Command{
			Tool: "go",
			Args: []string{"go", "test", "-run", "TestSmoke"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Args).To(Equal([]string{"go", "test", "-run", "TestSmoke", "./..."}))
	})

	DescribeTable("applies command mutations directly",
		func(args []string, when compiledWhen, flagsWithValues []string, command *compiledCommand, expected []string) {
			Expect(applyCommandMutations(args, when, flagsWithValues, command)).To(Equal(expected))
		},
		Entry("returns the original args when no command mutation is defined", []string{"go", "test"}, compiledWhen{}, nil, nil, []string{"go", "test"}),
		Entry("skips append_if_missing when the arg already exists", []string{"go", "test", "./..."}, compiledWhen{}, nil, &compiledCommand{appendIfMissing: []string{"./..."}}, []string{"go", "test", "./..."}),
		Entry("does not re-add an already grouped short flag", []string{"ls", "-la"}, compiledWhen{}, nil, &compiledCommand{addShortFlags: []string{"-a"}}, []string{"ls", "-la"}),
		Entry("ignores non-short flags in add_short_flags", []string{"ls"}, compiledWhen{}, nil, &compiledCommand{addShortFlags: []string{"--all"}}, []string{"ls"}),
		Entry("honors first_is when checking for explicit positionals", []string{"go", "test", "-run", "TestSmoke"}, compiledWhen{firstIs: "test"}, []string{"-run"}, &compiledCommand{appendIfNoPositionals: []string{"./..."}}, []string{"go", "test", "-run", "TestSmoke", "./..."}),
		Entry("honors first_in when checking for explicit positionals", []string{"go", "list", "-run", "TestSmoke"}, compiledWhen{firstIn: []string{"test", "list"}}, []string{"-run"}, &compiledCommand{appendIfNoPositionals: []string{"./..."}}, []string{"go", "list", "-run", "TestSmoke", "./..."}),
		Entry("does not append positionals when one already exists after first_is", []string{"go", "test", "./pkg"}, compiledWhen{firstIs: "test"}, []string{"-run"}, &compiledCommand{appendIfNoPositionals: []string{"./..."}}, []string{"go", "test", "./pkg"}),
		Entry("does not append positionals when one already exists without leading command context", []string{"go", "./pkg"}, compiledWhen{}, nil, &compiledCommand{appendIfNoPositionals: []string{"./..."}}, []string{"go", "./pkg"}),
	)

	Context("mutation hardening helpers", func() {
		DescribeTable("applies scope action precedence deterministically",
			func(scope *compiledScope, line string, expected contracts.Action) {
				Expect(scope.baseActionForLine(line, trimLineEnding(line), map[string]string{"name": "demo"})).To(Equal(expected))
			},
			Entry("keeps lines before considering replacements or skips",
				&compiledScope{
					keep:    []compiledMatcher{{startsWith: "KEEP"}},
					replace: []compiledReplace{{startsWith: "KEEP", replacement: "rewritten"}},
					skip:    []compiledMatcher{{startsWith: "KEEP"}},
				},
				"KEEP this\n",
				contracts.Action{Kind: contracts.ActionKeep},
			),
			Entry("replaces lines before skip rules run",
				&compiledScope{
					replace: []compiledReplace{{contains: "rewrite", replacement: "rewritten"}},
					skip:    []compiledMatcher{{contains: "rewrite"}},
				},
				"please rewrite this\n",
				contracts.Action{Kind: contracts.ActionReplace, Output: "rewritten\n", ReplaceCount: 1},
			),
			Entry("ignores skipped lines when keep rules do not match",
				&compiledScope{
					keep: []compiledMatcher{{startsWith: "PASS"}},
					skip: []compiledMatcher{{startsWith: "DEBUG"}},
				},
				"DEBUG details\n",
				contracts.Action{Kind: contracts.ActionIgnore},
			),
			Entry("ignores unmatched lines when keep rules exist",
				&compiledScope{
					keep: []compiledMatcher{{startsWith: "PASS"}},
				},
				"plain output\n",
				contracts.Action{Kind: contracts.ActionIgnore},
			),
		)

		It("clones scopes without groups while resetting runtime state", func() {
			cloned := cloneCompiledScope(&compiledScope{
				hidden:         3,
				activeBoundary: &activeBoundaryGroup{groupIndex: 1, sectionIndex: 2},
			})

			Expect(cloned).NotTo(BeNil())
			Expect(cloned.hidden).To(BeZero())
			Expect(cloned.activeBoundary).To(BeNil())
			Expect(cloned.groups).To(BeNil())
		})

		DescribeTable("compiles variable defaults deterministically",
			func(variables []Variable, expected map[string]string) {
				compiled, initials := compileVariables(variables)
				Expect(compiled).To(Equal(expected))
				Expect(initials).To(Equal(expected))
			},
			Entry("defaults string variables to empty values", []Variable{{Name: "name", Type: "string"}}, map[string]string{"name": ""}),
			Entry("defaults number variables to zero strings", []Variable{{Name: "count", Type: "number"}}, map[string]string{"count": "0"}),
			Entry("preserves explicit initial values", []Variable{
				{Name: "name", Type: "string", InitialValue: stringPtr("demo")},
				{Name: "count", Type: "number", InitialValue: stringPtr("2")},
			}, map[string]string{"name": "demo", "count": "2"}),
		)

		DescribeTable("compiles boundary groups with strict regex-group requirements",
			func(group OutputGroup, expectedErr string) {
				compiled, err := compileBoundaryGroup("stdout", group)
				if expectedErr == "" {
					Expect(err).NotTo(HaveOccurred())
					Expect(compiled.startsRegex).NotTo(BeNil())
					return
				}
				Expect(err).To(MatchError(ContainSubstring(expectedErr)))
			},
			Entry("accepts regex groups when starts_with_regex exposes the named capture", OutputGroup{
				ID:          "section",
				StartsRegex: `^== (?P<name>[^=]+) ==$`,
				Variables:   []Variable{{Name: "name", Type: "string", RegexGroup: "name"}},
			}, ""),
			Entry("rejects invalid starts_with_regex patterns", OutputGroup{
				ID:          "section",
				StartsRegex: `(`,
			}, `scope "stdout" group "section" starts_with_regex:`),
			Entry("rejects regex groups when starts_with_regex is missing", OutputGroup{
				ID:         "section",
				StartsWith: "== ",
				Variables:  []Variable{{Name: "name", Type: "string", RegexGroup: "name"}},
			}, `regex_group requires starts_with_regex`),
			Entry("rejects regex groups that are not present in starts_with_regex", OutputGroup{
				ID:          "section",
				StartsRegex: `^== (?P<other>[^=]+) ==$`,
				Variables:   []Variable{{Name: "name", Type: "string", RegexGroup: "name"}},
			}, `regex_group must reference a named capture from starts_with_regex`),
		)

		DescribeTable("compiles collected groups with strict regex-group requirements",
			func(group OutputGroup, expectedErr string) {
				compiled, err := compileCollectedGroup("stdout", group)
				if expectedErr == "" {
					Expect(err).NotTo(HaveOccurred())
					Expect(compiled.regex).NotTo(BeNil())
					return
				}
				Expect(err).To(MatchError(ContainSubstring(expectedErr)))
			},
			Entry("accepts regex groups when matches_regex exposes the named capture", OutputGroup{
				ID:           "section",
				MatchesRegex: `^== (?P<name>[^=]+) ==$`,
				Variables:    []Variable{{Name: "name", Type: "string", RegexGroup: "name"}},
			}, ""),
			Entry("accepts variables that do not depend on named captures", OutputGroup{
				ID:           "section",
				MatchesRegex: `^== [^=]+ ==$`,
				Variables:    []Variable{{Name: "name", Type: "string", DefaultValue: "fallback"}},
			}, ""),
			Entry("rejects regex groups that are not present in matches_regex", OutputGroup{
				ID:           "section",
				MatchesRegex: `^== (?P<other>[^=]+) ==$`,
				Variables:    []Variable{{Name: "name", Type: "string", RegexGroup: "name"}},
			}, `regex_group must reference a named capture from matches_regex`),
		)

		DescribeTable("applies rendered max helper deterministically",
			func(rendered []renderedLine, max *compiledMax, expected string) {
				Expect(applyRenderedMax(rendered, max)).To(Equal(expected))
			},
			Entry("renders all lines when no max is configured",
				[]renderedLine{{text: "first"}, {text: "second\n"}},
				nil,
				"first\nsecond\n",
			),
			Entry("appends overflow on a fresh line when the print does not start with one",
				[]renderedLine{{text: "first\n"}, {text: "second"}, {text: "third"}},
				&compiledMax{count: 2, print: "+{{value}} more"},
				"first\nsecond\n+1 more\n",
			),
			Entry("drops hidden rendered lines silently when the max print is empty",
				[]renderedLine{{text: "first"}, {text: "second"}},
				&compiledMax{count: 1},
				"first\n",
			),
			Entry("keeps all rendered lines when the count matches the exact max boundary",
				[]renderedLine{{text: "first"}, {text: "second"}},
				&compiledMax{count: 2, print: "+{{value}} more"},
				"first\nsecond\n",
			),
			Entry("summarizes omitted group items inside the overflow print",
				[]renderedLine{
					{text: "visible", groupKey: "visible", groupItem: true},
					{text: "beta-1", groupKey: "beta", groupItem: true},
					{text: "beta-2", groupKey: "beta", groupItem: true},
					{text: "alpha-1", groupKey: "alpha", groupItem: true},
				},
				&compiledMax{
					count: 1,
					print: "\n+{{value}} more {{groups_summary}}",
					groupsSummary: &compiledMaxGroupsSummary{
						show:      1,
						print:     "{{key}}({{count}})",
						delimiter: ", ",
						prefix:    "across ",
						suffix:    " and {{remaining}} more",
					},
				},
				"visible\n+3 more across beta(2) and 1 more\n",
			),
		)

		DescribeTable("renders omitted group summaries deterministically",
			func(summary *compiledMaxGroupsSummary, hidden []renderedLine, expected string) {
				Expect(renderGroupsSummary(summary, hidden)).To(Equal(expected))
			},
			Entry("returns empty when no summary config is provided",
				nil,
				[]renderedLine{{text: "ignored", groupKey: "pkg", groupItem: true}},
				"",
			),
			Entry("ignores hidden lines that are not grouped items",
				&compiledMaxGroupsSummary{show: 2, print: "{{key}}={{count}}", delimiter: ", "},
				[]renderedLine{{text: "plain"}, {text: "groupless", groupKey: "pkg"}},
				"",
			),
			Entry("orders by descending count then ascending key and appends the remaining suffix",
				&compiledMaxGroupsSummary{
					show:      2,
					print:     "{{key}}={{count}}",
					delimiter: ", ",
					prefix:    "groups: ",
					suffix:    " +{{remaining}}",
				},
				[]renderedLine{
					{text: "b1", groupKey: "beta", groupItem: true},
					{text: "a1", groupKey: "alpha", groupItem: true},
					{text: "g1", groupKey: "gamma", groupItem: true},
					{text: "a2", groupKey: "alpha", groupItem: true},
					{text: "b2", groupKey: "beta", groupItem: true},
				},
				"groups: alpha=2, beta=2 +1",
			),
			Entry("returns empty when all rendered summary parts are empty",
				&compiledMaxGroupsSummary{show: 1, print: "", delimiter: ", ", prefix: "groups: "},
				[]renderedLine{{text: "a1", groupKey: "alpha", groupItem: true}},
				"",
			),
		)

		DescribeTable("renders group items deterministically",
			func(group *compiledGroup, items []compiledGroupItem, groupKey string, countTowardsSummary bool, expected []renderedLine, emitted bool, hidden int) {
				rendered, gotEmitted, gotHidden := group.renderGroupItems(items, groupKey, countTowardsSummary)
				Expect(rendered).To(Equal(expected))
				Expect(gotEmitted).To(Equal(emitted))
				Expect(gotHidden).To(Equal(hidden))
			},
			Entry("passes lines through when no line rules exist",
				&compiledGroup{},
				[]compiledGroupItem{{line: "first"}},
				"pkg",
				true,
				[]renderedLine{{text: "first", groupKey: "pkg", groupItem: true}},
				true,
				0,
			),
			Entry("drops ignored lines before they can count as emitted output",
				&compiledGroup{lines: &compiledScope{skip: []compiledMatcher{{contains: "debug"}}}},
				[]compiledGroupItem{{line: "debug details"}},
				"pkg",
				true,
				[]renderedLine{},
				false,
				0,
			),
			Entry("counts limited lines as hidden once the grouped max is reached",
				&compiledGroup{lines: &compiledScope{max: &compiledMax{count: 1}}},
				[]compiledGroupItem{{line: "first"}, {line: "second"}},
				"pkg",
				false,
				[]renderedLine{{text: "first"}},
				true,
				1,
			),
			Entry("uses replacement output and records summary metadata when requested",
				&compiledGroup{lines: &compiledScope{replace: []compiledReplace{{startsWith: "src", replacement: "dst"}}}},
				[]compiledGroupItem{{line: "src/path"}},
				"pkg",
				true,
				[]renderedLine{{text: "dst", groupKey: "pkg", groupItem: true}},
				true,
				0,
			),
		)

		DescribeTable("renders collected groups only when output survives helper rules",
			func(exitCode int, maxCount int, maxPrint string, expected []renderedLine) {
				group := &compiledGroup{
					initially: &compiledOnExit{print: "header"},
					lines:     &compiledScope{max: &compiledMax{count: maxCount, print: maxPrint}},
					finally:   &compiledOnExit{print: "done"},
				}

				Expect(group.renderGroup([]compiledGroupItem{{line: "value"}}, "pkg", exitCode)).To(Equal(expected))
			},
			Entry("renders overflow and finally on success",
				0,
				0,
				"\n+{{value}} more",
				[]renderedLine{{text: "header"}, {text: "\n+1 more"}, {text: "done"}},
			),
			Entry("suppresses finally on non-zero exit",
				1,
				0,
				"\n+{{value}} more",
				[]renderedLine{{text: "header"}, {text: "\n+1 more"}},
			),
			Entry("keeps the group header and visible items without adding overflow when nothing was hidden",
				0,
				1,
				"\n+{{value}} more",
				[]renderedLine{{text: "header"}, {text: "value", groupKey: "pkg", groupItem: true}, {text: "done"}},
			),
			Entry("drops the group entirely when nothing is emitted and overflow is silent",
				0,
				0,
				"",
				nil,
			),
		)

		DescribeTable("renders boundary sections only when output survives helper rules",
			func(exitCode int, maxCount int, maxPrint string, expected []renderedLine) {
				group := &compiledGroup{
					initially: &compiledOnExit{print: "section {{name}}"},
					lines:     &compiledScope{max: &compiledMax{count: maxCount, print: maxPrint}},
					finally:   &compiledOnExit{print: "done {{name}}"},
				}
				section := compiledBoundarySection{
					vars:  map[string]string{"name": "tests"},
					items: []compiledGroupItem{{line: "value"}},
				}

				Expect(group.renderBoundarySection(section, exitCode)).To(Equal(expected))
			},
			Entry("renders overflow and final section output on success",
				0,
				0,
				"+{{value}} more",
				[]renderedLine{{text: "section tests"}, {text: "+1 more"}, {text: "done tests"}},
			),
			Entry("keeps the boundary section output without overflow when nothing was hidden",
				0,
				1,
				"+{{value}} more",
				[]renderedLine{{text: "section tests"}, {text: "value"}, {text: "done tests"}},
			),
			Entry("returns nothing when a boundary section has no visible or printable output",
				0,
				0,
				"",
				nil,
			),
		)

		DescribeTable("matches boundary starts deterministically",
			func(group compiledGroup, input string, expected map[string]string, matched bool) {
				values, ok := group.matchBoundaryStart(input)
				Expect(ok).To(Equal(matched))
				Expect(values).To(Equal(expected))
			},
			Entry("captures named regex groups from boundary starts",
				compiledGroup{
					startsRegex: regexp.MustCompile(`^== (?P<name>[^=]+) ==$`),
					variables:   []compiledVariable{{name: "name", kind: "string", regexGroup: "name"}},
				},
				"== tests ==",
				map[string]string{"name": "tests"},
				true,
			),
			Entry("returns false when a boundary regex does not match",
				compiledGroup{startsRegex: regexp.MustCompile(`^== (?P<name>[^=]+) ==$`)},
				"tests ==",
				nil,
				false,
			),
			Entry("matches fixed boundary prefixes without captures",
				compiledGroup{startsWith: "FAILURE:"},
				"FAILURE: details",
				map[string]string{},
				true,
			),
			Entry("returns false for fixed boundary prefixes that do not match",
				compiledGroup{startsWith: "FAILURE:"},
				"SUCCESS: details",
				nil,
				false,
			),
			Entry("returns false when no boundary selector is configured",
				compiledGroup{},
				"anything",
				nil,
				false,
			),
		)

		DescribeTable("classifies argument helpers deterministically",
			func(args []string, expected []string) {
				Expect(filterArgs(args)).To(Equal(expected))
			},
			Entry("drops the tool name and returns the remaining args", []string{"go", "test", "./..."}, []string{"test", "./..."}),
			Entry("returns nil when no filtered args remain", []string{"go"}, nil),
		)

		DescribeTable("matches short flags deterministically",
			func(args []string, want rune, expected bool) {
				Expect(containsShortFlag(args, want)).To(Equal(expected))
			},
			Entry("finds grouped short flags", []string{"ls", "-la"}, 'a', true),
			Entry("ignores bare dashes when scanning short flags", []string{"ls", "-", "file"}, 'a', false),
			Entry("ignores long flags", []string{"ls", "--all"}, 'a', false),
			Entry("returns false when no matching flag exists", []string{"ls", "-l"}, 'a', false),
		)

		DescribeTable("classifies atomic argument helpers deterministically",
			func(arg string, want string, shortExpected, argExpected bool) {
				Expect(isShortFlag(arg)).To(Equal(shortExpected))
				Expect(slices.Contains([]string{"go", "test", "./..."}, want)).To(Equal(argExpected))
			},
			Entry("accepts single short flags and present args", "-a", "./...", true, true),
			Entry("rejects long flags while still finding present args", "--all", "test", false, true),
			Entry("rejects plain tokens and missing args", "test", "missing", false, false),
		)

		DescribeTable("matches when-argument helper branches deterministically",
			func(when compiledWhen, flagsWithValues, args []string, expected bool) {
				Expect(matchesWhenArguments(when, flagsWithValues, args)).To(Equal(expected))
			},
			Entry("treats first_is as leading command context for no_positionals checks",
				compiledWhen{firstIs: "test", noPositionals: true},
				[]string{"-run"},
				[]string{"test", "-run", "TestSmoke"},
				true,
			),
			Entry("rejects explicit positionals after the leading command",
				compiledWhen{firstIs: "test", noPositionals: true},
				[]string{"-run"},
				[]string{"test", "./pkg"},
				false,
			),
			Entry("treats first_in as leading command context for no_positionals checks",
				compiledWhen{firstIn: []string{"test", "list"}, noPositionals: true},
				[]string{"-run"},
				[]string{"list", "-run", "TestSmoke"},
				true,
			),
		)

		DescribeTable("matches compiled matchers deterministically",
			func(matcher compiledMatcher, input string, expected bool) {
				Expect(matcher.matches(input)).To(Equal(expected))
			},
			Entry("matches regex rules", compiledMatcher{regex: regexp.MustCompile(`^FAIL`)}, "FAIL: broken", true),
			Entry("matches starts_with rules", compiledMatcher{startsWith: "WARN:"}, "WARN: details", true),
			Entry("matches contains rules", compiledMatcher{contains: "debug"}, "plain debug output", true),
			Entry("matches ends_with rules", compiledMatcher{endsWith: ".go"}, "main.go", true),
			Entry("returns false when regex rules do not match", compiledMatcher{regex: regexp.MustCompile(`^FAIL`)}, "PASS", false),
			Entry("returns false when starts_with rules do not match", compiledMatcher{startsWith: "WARN:"}, "INFO: details", false),
			Entry("returns false when contains rules do not match", compiledMatcher{contains: "debug"}, "plain output", false),
			Entry("returns false when ends_with rules do not match", compiledMatcher{endsWith: ".go"}, "main.txt", false),
			Entry("returns false when no matcher selector exists", compiledMatcher{}, "anything", false),
		)

		DescribeTable("replaces content only when replacement selectors match",
			func(rule compiledReplace, input string, expected string, matched bool) {
				got, ok := rule.replace(input, map[string]string{"name": "demo"})
				Expect(ok).To(Equal(matched))
				Expect(got).To(Equal(expected))
			},
			Entry("uses regex replacements with template rendering",
				compiledReplace{regex: regexp.MustCompile(`^name=(.+)$`), replacement: "service={{name}}"},
				"name=value",
				"service=demo",
				true,
			),
			Entry("returns false when regex replacements do not match",
				compiledReplace{regex: regexp.MustCompile(`^name=`), replacement: "service={{name}}"},
				"value",
				"",
				false,
			),
			Entry("uses starts_with replacements", compiledReplace{startsWith: "WARN:", replacement: "warning"}, "WARN: detail", "warning", true),
			Entry("uses contains replacements", compiledReplace{contains: "debug", replacement: "hidden"}, "plain debug output", "hidden", true),
			Entry("uses ends_with replacements", compiledReplace{endsWith: ".go", replacement: "file"}, "main.go", "file", true),
			Entry("returns false when starts_with replacements do not match", compiledReplace{startsWith: "WARN:", replacement: "warning"}, "INFO", "", false),
			Entry("returns false when contains replacements do not match", compiledReplace{contains: "debug", replacement: "hidden"}, "plain output", "", false),
			Entry("returns false when ends_with replacements do not match", compiledReplace{endsWith: ".go", replacement: "file"}, "main.txt", "", false),
		)

		DescribeTable("applies scope max only once the buffered count reaches the exact limit",
			func(bufferedCount int, expectedKind contracts.ActionKind, expectedHidden int) {
				scope := &compiledScope{
					max: &compiledMax{count: 1},
				}

				action := scope.actionForLine("value\n", bufferedCount, nil)
				Expect(action.Kind).To(Equal(expectedKind))
				Expect(scope.hidden).To(Equal(expectedHidden))
			},
			Entry("keeps lines below the max boundary", 0, contracts.ActionKeep, 0),
			Entry("hides lines once the max boundary is reached", 1, contracts.ActionIgnore, 1),
		)
	})
})
