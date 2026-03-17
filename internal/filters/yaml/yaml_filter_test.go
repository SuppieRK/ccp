package yaml

import (
	"strings"

	"go-command-compression-proxy/internal/contracts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type yamlFilterContext struct {
	args   []string
	stdout []string
	stderr []string
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
	default:
		return nil
	}
}

var _ = Describe("YamlFilter", func() {
	intPtr := func(v int) *int { return &v }
	stringPtr := func(v string) *string { return &v }

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
			for _, input := range strings.Split("KEEP\nREPLACE\nSKIP\nvalue\n", "\n") {
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
			Output: "pkg/a/\n  a.go\n",
		}))
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
					AddShortFlags:   []string{"-a"},
					AppendIfMissing: []string{"."},
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
			Output: "docs/\nREADME.md  59\n1 dirs, 1 files\n",
		}))
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
			Output: "pkg/a/\n  a.go\n\n2 lines\ndone\n",
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

			for _, input := range strings.Split("./pkg/a/KEEP\n./pkg/a/REPLACE\n./pkg/a/SKIP\n./pkg/a/value\n", "\n") {
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
		Entry("keep, replace, and skip all apply within one grouped stream", true, true, true, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n"}),
		Entry("keep and replace apply while skip is absent in grouped output", true, true, false, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n"}),
		Entry("keep and skip apply while replace is absent in grouped output", true, false, true, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n"}),
		Entry("only keep is configured so unmatched grouped lines are ignored", true, false, false, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n"}),
		Entry("replace and skip apply while unmatched grouped lines still passthrough", false, true, true, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n./pkg/a/value\n"}),
		Entry("only replace is configured so non-target grouped lines passthrough", false, true, false, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n  REWRITTEN\n./pkg/a/SKIP\n./pkg/a/value\n"}),
		Entry("only skip is configured so untargeted grouped lines passthrough", false, false, true, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n./pkg/a/REPLACE\n./pkg/a/value\n"}),
		Entry("no grouped line conditions are present so the stream passthroughs unchanged", false, false, false, contracts.Action{Kind: contracts.ActionReplace, Output: "pkg/a/\n./pkg/a/KEEP\n./pkg/a/REPLACE\n./pkg/a/SKIP\n./pkg/a/value\n"}),
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
})
