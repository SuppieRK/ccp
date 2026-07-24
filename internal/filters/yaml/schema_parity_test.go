package yaml

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("JSON schema parity", func() {
	var schemaDoc map[string]any

	BeforeEach(func() {
		path := filepath.Join("..", "..", "..", "schemas", "cmdshape-filter.schema.json")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(raw, &schemaDoc)).To(Succeed())
	})

	type parityExpectation struct {
		schemaValid bool
		parseValid  bool
	}

	DescribeTable("keeps JSON schema and Go parsing aligned where expected",
		func(raw string, expected parityExpectation) {
			document, err := decodeYAMLDocument(raw)
			Expect(err).NotTo(HaveOccurred())

			schemaErrs := validateSchemaNode(schemaDoc, document, schemaDoc)
			if expected.schemaValid {
				Expect(schemaErrs).To(BeEmpty())
			} else {
				Expect(schemaErrs).NotTo(BeEmpty())
			}

			_, parseErr := ParseDefinition([]byte(raw))
			if expected.parseValid {
				Expect(parseErr).NotTo(HaveOccurred())
			} else {
				Expect(parseErr).To(HaveOccurred())
			}
		},
		Entry("valid passthrough case", `
version: 1
filter: python
cases:
  - id: default
    passthrough: true
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("valid have_all_short_flags predicate", `
version: 1
filter: ls
cases:
  - id: long_recursive
    when_arguments:
      have_all_short_flags: ['-l', '-R']
    passthrough: true
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("valid negative short flag predicates", `
version: 1
filter: grep
cases:
  - id: explicit
    when_arguments:
      not_have_short_flag: ['-o']
      not_have_all_short_flags: ['-H', '-o']
    passthrough: true
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("valid no_positionals predicate", `
version: 1
filter: git
flags_consuming_next_arg: ['-C']
cases:
  - id: branch
    when_arguments:
      no_positionals: true
    passthrough: true
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("valid append_if_no_positionals command mutation", `
version: 1
filter: ls
flags_consuming_next_arg: ['--color']
cases:
  - id: long
    normalize_command:
      append_if_no_positionals: ['.']
    passthrough: true
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("invalid top-level flags_consuming_next_arg entry", `
version: 1
filter: ls
flags_consuming_next_arg: ['color']
cases:
  - id: long
    passthrough: true
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("invalid case-local value_flags field", `
version: 1
filter: go
cases:
  - id: test
    when_arguments:
      first_is: test
      value_flags: ['-run']
    passthrough: true
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("valid combined output case", `
version: 1
filter: ls
cases:
  - id: long
    compress_output:
      combined:
        lines:
          keep:
            - starts_with: total
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("invalid missing version", `
filter: python
cases:
  - id: default
    passthrough: true
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("invalid empty cases", `
version: 1
filter: python
cases: []
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("invalid mixed combined and stdout", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      combined:
        lines:
          keep:
            - starts_with: ok
      stdout:
        lines:
          keep:
            - starts_with: ok
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("invalid empty output scope remains a structural gap in JSON schema", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout: {}
`, parityExpectation{schemaValid: true, parseValid: false}),
		Entry("invalid removed tail field", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout:
        lines:
          tail: 10
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("invalid removed truncate field", `
version: 1
filter: python
cases:
  - id: default
    compress_output:
      stdout:
        lines:
          truncate: 80
`, parityExpectation{schemaValid: false, parseValid: false}),
		Entry("invalid replace matcher multiplicity remains a runtime-only gap", `
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
`, parityExpectation{schemaValid: true, parseValid: false}),
		Entry("invalid regex_group reference remains a runtime-only gap", `
version: 1
filter: find
cases:
  - id: grouped
    compress_output:
      combined:
        groups:
          - id: dir
            matches_regex: '^\./(?P<dir>.+)/(?P<name>[^/]+)$'
            variables:
              - name: missing
                type: string
                regex_group: missing
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
`, parityExpectation{schemaValid: true, parseValid: false}),
		Entry("invalid template variable reference remains a runtime-only gap", `
version: 1
filter: find
cases:
  - id: grouped
    compress_output:
      combined:
        groups:
          - id: dir
            matches_regex: '^\./(?P<dir>.+)/(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
            group_by: '{{missing}}'
            initially:
              print: '{{dir}}/'
`, parityExpectation{schemaValid: true, parseValid: false}),
		Entry("valid groups summary on grouped scope", `
version: 1
filter: find
cases:
  - id: grouped
    compress_output:
      combined:
        lines:
          max:
            count: 200
            print: "\n+{{value}} more {{groups_summary}}"
            groups_summary:
              show: 3
              print: "{{key}}/({{count}})"
        groups:
          - id: dir
            matches_regex: '^\./(?P<dir>.+)/(?P<name>[^/]+)$'
            variables:
              - name: dir
                type: string
                regex_group: dir
              - name: name
                type: string
                regex_group: name
            group_by: '{{dir}}'
            initially:
              print: '{{dir}}/'
`, parityExpectation{schemaValid: true, parseValid: true}),
		Entry("groups summary without collect groups remains a runtime-only gap", `
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
`, parityExpectation{schemaValid: true, parseValid: false}),
	)
})

func decodeYAMLDocument(raw string) (any, error) {
	var document any
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return nil, err
	}
	return document, nil
}

func validateSchemaNode(schema any, value any, root map[string]any) []string {
	current, ok := schema.(map[string]any)
	if !ok {
		return nil
	}

	if ref, ok := current["$ref"].(string); ok {
		resolved, err := resolveSchemaRef(ref, root)
		if err != nil {
			return []string{err.Error()}
		}
		return validateSchemaNode(resolved, value, root)
	}

	errs := validateSchemaCombinators(current, value, root)
	if len(errs) > 0 {
		return errs
	}
	errs = append(errs, validateSchemaScalarConstraints(current, value)...)
	errs = append(errs, validateSchemaTypedNode(current, value, root)...)
	return errs
}

func validateSchemaCombinators(current map[string]any, value any, root map[string]any) []string {
	oneOf, ok := current["oneOf"].([]any)
	if !ok {
		return nil
	}
	matches := 0
	for _, candidate := range oneOf {
		if len(validateSchemaNode(candidate, value, root)) == 0 {
			matches++
		}
	}
	if matches == 1 {
		return nil
	}
	return []string{fmt.Sprintf("expected exactly one schema match, got %d", matches)}
}

func validateSchemaScalarConstraints(current map[string]any, value any) []string {
	var errs []string
	if constValue, ok := current["const"]; ok && !valuesEqual(constValue, value) {
		errs = append(errs, fmt.Sprintf("expected const %v", constValue))
	}
	if enumValues, ok := current["enum"].([]any); ok && !matchesEnum(enumValues, value) {
		errs = append(errs, "value not in enum")
	}
	return errs
}

func validateSchemaTypedNode(current map[string]any, value any, root map[string]any) []string {
	switch current["type"] {
	case "object":
		return validateSchemaObject(current, value, root)
	case "array":
		return validateSchemaArray(current, value, root)
	case "string":
		return validateSchemaString(current, value)
	case "integer":
		return validateSchemaInteger(current, value)
	case "boolean":
		return validateSchemaBoolean(value)
	default:
		return nil
	}
}

func validateSchemaObject(current map[string]any, value any, root map[string]any) []string {
	obj, ok := value.(map[string]any)
	if !ok {
		return []string{"expected object"}
	}

	var errs []string
	properties := schemaProperties(current)
	errs = append(errs, validateRequiredFields(current, obj)...)
	errs = append(errs, validateAdditionalProperties(current, obj, properties)...)
	for key, propertyValue := range properties {
		child, exists := obj[key]
		if !exists {
			continue
		}
		errs = append(errs, validateSchemaNode(propertyValue, child, root)...)
	}
	return errs
}

func validateSchemaArray(current map[string]any, value any, root map[string]any) []string {
	items, ok := value.([]any)
	if !ok {
		return []string{"expected array"}
	}

	var errs []string
	if minItems, ok := numberAsInt(current["minItems"]); ok && len(items) < minItems {
		errs = append(errs, fmt.Sprintf("expected at least %d items", minItems))
	}
	if itemSchema, ok := current["items"]; ok {
		for _, item := range items {
			errs = append(errs, validateSchemaNode(itemSchema, item, root)...)
		}
	}
	return errs
}

func validateSchemaString(current map[string]any, value any) []string {
	text, ok := value.(string)
	if !ok {
		return []string{"expected string"}
	}
	if pattern, ok := current["pattern"].(string); ok {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return []string{fmt.Sprintf("invalid schema pattern %q", pattern)}
		}
		if !re.MatchString(text) {
			return []string{fmt.Sprintf("expected string matching %q", pattern)}
		}
	}
	if minLength, ok := numberAsInt(current["minLength"]); ok && len(text) < minLength {
		return []string{fmt.Sprintf("expected min length %d", minLength)}
	}
	return nil
}

func validateSchemaInteger(current map[string]any, value any) []string {
	currentValue, ok := asInt(value)
	if !ok {
		return []string{"expected integer"}
	}
	if minimum, ok := numberAsInt(current["minimum"]); ok && currentValue < minimum {
		return []string{fmt.Sprintf("expected integer >= %d", minimum)}
	}
	return nil
}

func validateSchemaBoolean(value any) []string {
	if _, ok := value.(bool); ok {
		return nil
	}
	return []string{"expected boolean"}
}

func validateRequiredFields(current map[string]any, obj map[string]any) []string {
	required, ok := current["required"].([]any)
	if !ok {
		return nil
	}

	var errs []string
	for _, field := range required {
		name := field.(string)
		if _, exists := obj[name]; !exists {
			errs = append(errs, fmt.Sprintf("missing required field %q", name))
		}
	}
	return errs
}

func validateAdditionalProperties(current map[string]any, obj map[string]any, properties map[string]any) []string {
	additional, ok := current["additionalProperties"].(bool)
	if !ok || additional {
		return nil
	}

	var errs []string
	for key := range obj {
		if _, exists := properties[key]; !exists {
			errs = append(errs, fmt.Sprintf("unexpected field %q", key))
		}
	}
	return errs
}

func schemaProperties(current map[string]any) map[string]any {
	rawProps, ok := current["properties"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return rawProps
}

func matchesEnum(enumValues []any, value any) bool {
	for _, candidate := range enumValues {
		if valuesEqual(candidate, value) {
			return true
		}
	}
	return false
}

func resolveSchemaRef(ref string, root map[string]any) (any, error) {
	if ref == "#" {
		return root, nil
	}
	if len(ref) == 0 || ref[:2] != "#/" {
		return nil, fmt.Errorf("unsupported schema ref %q", ref)
	}
	current := any(root)
	for _, part := range splitRef(ref[2:]) {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid schema ref %q", ref)
		}
		next, ok := object[part]
		if !ok {
			return nil, fmt.Errorf("unknown schema ref %q", ref)
		}
		current = next
	}
	return current, nil
}

func splitRef(path string) []string {
	if path == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i != len(path) && path[i] != '/' {
			continue
		}
		parts = append(parts, path[start:i])
		start = i + 1
	}
	return parts
}

func valuesEqual(expected, actual any) bool {
	if value, ok := expected.(float64); ok {
		if actualInt, ok := asInt(actual); ok {
			return int(value) == actualInt
		}
	}
	return fmt.Sprint(expected) == fmt.Sprint(actual)
}

func numberAsInt(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	return asInt(value)
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}
