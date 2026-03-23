package yaml

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseDefinition", func() {
	It("accepts anchors inside one definition and validates the resolved shape", func() {
		raw := []byte(`
version: 1
filter: pytest
cases:
  - &default_case
    id: default
    compress_output:
      combined:
        lines:
          keep:
            - regex: '^FAILED'
  - <<: *default_case
    id: failed_only
    when_arguments:
      have_any: ['-x']
`)

		spec, err := ParseDefinition(raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Filter).To(Equal("pytest"))
		Expect(spec.Cases).To(HaveLen(2))
		Expect(spec.Cases[1].CompressOutput.Combined.Lines.Keep).To(Equal([]SkipOrKeepRule{{Regex: "^FAILED"}}))
		Expect(spec.Cases[1].WhenArguments.HaveAny).To(Equal([]string{"-x"}))
	})

	It("rejects unknown top-level fields", func() {
		raw := []byte(`
version: 1
filter: pytest
unknown_field: true
cases:
  - id: default
    passthrough: true
`)

		_, err := ParseDefinition(raw)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("field unknown_field not found"))
	})

	It("loads repository filters with strict validation", func() {
		root, err := os.MkdirTemp("", "filter-schema-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(os.RemoveAll(root)).To(Succeed())
		})

		path := filepath.Join(root, "filters", "python", "python.yaml")
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(`
version: 1
filter: python
cases:
  - id: pytest
    when_arguments:
      have_sequence: [-m, pytest]
    compress_output:
      stdout:
        lines:
          keep:
            - regex: 'passed'
`), 0o644)).To(Succeed())

		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		spec, err := ParseDefinition(raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Cases[0].WhenArguments.HaveSequence).To(Equal([]string{"-m", "pytest"}))
	})

	DescribeTable("rejects invalid definitions",
		func(raw string, expected string) {
			_, err := ParseDefinition([]byte(raw))
			if expected == "" {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(MatchError(expected))
		},
		Entry("missing version", `
filter: python
cases:
  - id: default
    passthrough: true
`, "version: version must be exactly 1"),
		Entry("missing filter id", `
version: 1
cases:
  - id: default
    passthrough: true
`, "filter: filter id must not be empty"),
		Entry("missing cases", `
version: 1
filter: python
`, "cases: at least one case is required"),
		Entry("empty case id", `
version: 1
filter: python
cases:
  - passthrough: true
`, "cases[0].id: case id must not be empty"),
		Entry("empty when arguments", `
version: 1
filter: python
cases:
  - id: default
    when_arguments: {}
    passthrough: true
`, "cases[0].when_arguments: when_arguments must set at least one predicate"),
		Entry("valid negative short flag predicates", `
version: 1
filter: grep
cases:
  - id: explicit
    when_arguments:
      not_have_short_flag: ['-o']
      not_have_all_short_flags: ['-H', '-o']
    passthrough: true
`, ""),
		Entry("valid no_positionals predicate", `
version: 1
filter: git
flags_consuming_next_arg: ['-C']
cases:
  - id: branch
    when_arguments:
      no_positionals: true
    passthrough: true
`, ""),
		Entry("invalid top-level flags_consuming_next_arg entry", `
version: 1
filter: git
flags_consuming_next_arg: ['branch']
cases:
  - id: branch
    passthrough: true
`, "flags_consuming_next_arg[0]: flags_consuming_next_arg entries must start with '-'"),
		Entry("invalid case-local value_flags field", `
version: 1
filter: go
cases:
  - id: test
    when_arguments:
      first_is: test
      value_flags: ['-run']
    passthrough: true
`, "decode yaml: yaml: unmarshal errors:\n  line 8: field value_flags not found in type yaml.WhenArguments"),
		Entry("empty command block", `
version: 1
filter: python
cases:
  - id: default
    normalize_command: {}
    passthrough: true
`, "cases[0].normalize_command: command block must define append_if_missing, append_if_no_positionals or add_short_flags"),
		Entry("passthrough output", `
version: 1
filter: python
cases:
  - id: default
    passthrough: true
    compress_output:
      combined:
        lines:
          keep:
            - regex: 'ok'
`, "cases[0].compress_output: passthrough case must not define compress_output"),
		Entry("empty output", `
version: 1
filter: python
cases:
  - id: default
    compress_output: {}
`, "cases[0].compress_output: output must define at least one scope"),
		Entry("mixed combined and stream-specific output", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      combined:
        lines:
          keep:
            - regex: 'ok'
      stdout:
        lines:
          keep:
            - regex: 'ok'
`, "cases[0].compress_output: output.combined must not be mixed with stream-specific scopes"),
		Entry("empty output scope", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout: {}
`, "cases[0].compress_output.stdout: output scope must define lines or groups"),
		Entry("non-positive max count", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout:
        lines:
          max:
            count: 0
            print: "\n{{value}} lines"
`, "cases[0].compress_output.stdout.lines.max.count: max.count must be positive"),
		Entry("empty max print is allowed", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout:
        lines:
          max:
            count: 1
            print: ''
`, ""),
		Entry("invalid max print variable", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout:
        lines:
          max:
            count: 1
            print: '{{missing}}'
`, "cases[0].compress_output.stdout.lines.max.print: max.print must reference only \"{{value}}\", got \"missing\""),
		Entry("valid max print groups summary on collect scope", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        lines:
          max:
            count: 200
            print: "\n+{{value}} more {{groups_summary}}"
            groups_summary:
              show: 3
              print: "{{key}}/({{count}})"
              delimiter: ", "
              prefix: "across "
              suffix: " and {{remaining}} other dirs"
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
              - name: name
                type: string
                regex_group: name
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
            lines:
              replace:
                - regex: '^.*$'
                  to: '  {{name}}'
`, ""),
		Entry("groups summary requires collect groups in scope", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout:
        lines:
          max:
            count: 1
            print: '{{value}} {{groups_summary}}'
            groups_summary:
              show: 1
              print: '{{key}}/({{count}})'
`, "cases[0].compress_output.stdout.lines.max.groups_summary: max.groups_summary requires collect groups in the same output scope"),
		Entry("invalid max print groups summary variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        lines:
          max:
            count: 200
            print: "{{value}} {{missing}}"
            groups_summary:
              show: 3
              print: "{{key}}/({{count}})"
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
              - name: name
                type: string
                regex_group: name
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
            lines:
              replace:
                - regex: '^.*$'
                  to: '  {{name}}'
`, "cases[0].compress_output.combined.lines.max.print: max.print must reference only \"{{value}}, {{groups_summary}}\", got \"missing\""),
		Entry("invalid groups summary print variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        lines:
          max:
            count: 200
            print: "{{value}} {{groups_summary}}"
            groups_summary:
              show: 3
              print: "{{missing}}/({{count}})"
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
              - name: name
                type: string
                regex_group: name
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
            lines:
              replace:
                - regex: '^.*$'
                  to: '  {{name}}'
`, "cases[0].compress_output.combined.lines.max.groups_summary.print: template references undeclared variable \"missing\""),
		Entry("invalid groups summary suffix variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        lines:
          max:
            count: 200
            print: "{{value}} {{groups_summary}}"
            groups_summary:
              show: 3
              print: "{{key}}/({{count}})"
              suffix: " and {{count}} others"
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
              - name: name
                type: string
                regex_group: name
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
            lines:
              replace:
                - regex: '^.*$'
                  to: '  {{name}}'
`, "cases[0].compress_output.combined.lines.max.groups_summary.suffix: template references undeclared variable \"count\""),
		Entry("replace rule without matcher", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          replace:
            - to: replacement
`, "cases[0].compress_output.combined.lines.replace[0]: replace rule must set exactly one of regex, starts_with, contains, or ends_with"),
		Entry("replace rule with multiple matchers", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          replace:
            - starts_with: total
              contains: total
              to: replacement
`, "cases[0].compress_output.combined.lines.replace[0]: replace rule must set exactly one of regex, starts_with, contains, or ends_with"),
		Entry("skip rule without matcher", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          skip:
            - {}
`, "cases[0].compress_output.combined.lines.skip[0]: skip/keep rule must set exactly one of regex, starts_with, contains, or ends_with"),
		Entry("keep rule with multiple matchers", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          keep:
            - starts_with: total
              contains: total
`, "cases[0].compress_output.combined.lines.keep[0]: skip/keep rule must set exactly one of regex, starts_with, contains, or ends_with"),
		Entry("replace rule without to", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          replace:
            - regex: '^foo'
`, "cases[0].compress_output.combined.lines.replace[0].to: replace rule must define to"),
		Entry("match action references undeclared variable", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          replace:
            - regex: '^foo'
              to: '$0'
              on_match:
                - variable: missing
                  increment: 1
`, "cases[0].compress_output.combined.lines.replace[0].on_match[0].variable: match action variable must reference a declared variable"),
		Entry("overlapping line matchers across categories are allowed", `
version: 1
filter: maven
cases:
  - id: default
    compress_output:
      combined:
        lines:
          keep:
            - regex: '^\[INFO\] BUILD SUCCESS$'
          replace:
            - regex: '^\[INFO\] --- .+:([A-Za-z0-9_-]+) \([^)]+\) @ ([^ ]+) ---$'
              to: '$2 : $1'
          skip:
            - starts_with: '[INFO]'
`, ""),
		Entry("collect group missing match regex", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
`, "cases[0].compress_output.combined.groups[0].matches_regex: collect group must define matches_regex"),
		Entry("collect group mixes boundary and collect fields", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            starts_with_regex: '^foo'
            matches_regex: '^bar'
            variables:
              - name: dir
                type: string
                regex_group: dir
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
`, "cases[0].compress_output.combined.groups[0]: group must not mix boundary grouping with collect grouping"),
		Entry("collect group variable references missing regex capture", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
`, "cases[0].compress_output.combined.groups[0].variables[0].regex_group: regex_group must reference a named capture from matches_regex"),
		Entry("collect group group_by references undeclared variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
            group_by: '{{missing}}'
            initially:
              print: '{{dir}}/'
`, "cases[0].compress_output.combined.groups[0].group_by: template references undeclared variable \"missing\""),
		Entry("collect group initially print references undeclared variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
            group_by: '{{dir}}'
            initially:
              print: '{{missing}}/'
`, "cases[0].compress_output.combined.groups[0].initially.print: template references undeclared variable \"missing\""),
		Entry("collect group line replacement references undeclared variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
              - name: name
                type: string
                regex_group: name
            group_by: '{{dir}}'
            lines:
              replace:
                - regex: '^.*$'
                  to: '  {{missing}}'
`, "cases[0].compress_output.combined.groups[0].lines.replace[0].to: template references undeclared variable \"missing\""),
		Entry("collect group finally print references undeclared variable", `
version: 1
filter: find
cases:
  - id: files
    compress_output:
      combined:
        groups:
          - id: by_parent_dir
            matches_regex: '^\./(?:(?P<dir>.+)/)?(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
                default_value: '.'
            group_by: '{{dir}}'
            finally:
              print: '{{missing}}'
`, "cases[0].compress_output.combined.groups[0].finally.print: template references undeclared variable \"missing\""),
		Entry("boundary group is valid with starts_with and lines", `
version: 1
filter: pytest
cases:
  - id: default
    compress_output:
      combined:
        groups:
          - id: failures
            starts_with: '==== FAILURES ===='
            initially:
              print: 'failure details:'
            lines:
              keep:
                - contains: 'AssertionError'
`, ""),
		Entry("boundary group with regex variables is valid", `
version: 1
filter: maven
cases:
  - id: default
    compress_output:
      combined:
        groups:
          - id: module_goal
            starts_with_regex: '^\[INFO\]\s+---\s+.+:(?P<goal>[A-Za-z0-9_-]+)\s+\([^)]+\)\s+@\s+(?P<module>[^ ]+)\s+---$'
            variables:
              - name: goal
                type: string
                regex_group: goal
              - name: module
                type: string
                regex_group: module
            initially:
              print: '{{module}} : {{goal}}'
`, ""),
		Entry("boundary group regex_group requires starts_with_regex", `
version: 1
filter: gradle
cases:
  - id: default
    compress_output:
      combined:
        groups:
          - id: task
            starts_with: '> Task '
            variables:
              - name: task
                type: string
                regex_group: task
            initially:
              print: '{{task}}'
`, "cases[0].compress_output.combined.groups[0].variables[0].regex_group: boundary groups without starts_with_regex must not define regex_group"),
		Entry("boundary group requires a lifecycle stage", `
version: 1
filter: gradle
cases:
  - id: default
    compress_output:
      combined:
        groups:
          - id: task
            starts_with: '> Task '
`, "cases[0].compress_output.combined.groups[0]: boundary group must define initially, lines, or finally"),
	)
})
