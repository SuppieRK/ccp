---
name: bdd
description: >
  Go BDD testing with Ginkgo and Gomega.
  Covers BDD-style tests, table-driven tests with DescribeTable, matchers,
  async testing with Eventually/Consistently, test organization, and
  integration testing. Use when writing tests in projects that use
  Ginkgo/Gomega instead of stdlib testing.
---

# BDD

Behavior-Driven Development (BDD) testing in Go using [Ginkgo](https://onsi.github.io/ginkgo/) as the testing framework and [Gomega](https://onsi.github.io/gomega/) as the matcher/assertion library.

## Core Principles

1. **BDD-style organization** — Use Describe/Context/It for hierarchical test organization
2. **Expressive matchers** — Gomega provides readable assertions
3. **Table-driven tests** — DescribeTable for multiple test cases
4. **Test behavior, not implementation** — Focus on the public API
5. **Keep specs focused** — Each It block tests one behavior

## Setup and Installation

```bash
# Install Ginkgo CLI
go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Install dependencies
go get github.com/onsi/ginkgo/v2
go get github.com/onsi/gomega

# Bootstrap a test suite
cd mypackage
ginkgo bootstrap
```

This creates a `mypackage_suite_test.go` file:

```go
package mypackage_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestMypackage(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Mypackage Suite")
}
```

## Basic Test Structure

Ginkgo organizes tests using Describe, Context, and It blocks:

```go
package parser_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "myservice/internal/parser"
)

var _ = Describe("ParseAmount", func() {
    Context("when given valid input", func() {
        It("parses whole dollars", func() {
            result, err := parser.ParseAmount("42")
            Expect(err).ToNot(HaveOccurred())
            Expect(result).To(Equal(int64(4200)))
        })

        It("parses dollars with cents", func() {
            result, err := parser.ParseAmount("42.50")
            Expect(err).ToNot(HaveOccurred())
            Expect(result).To(Equal(int64(4250)))
        })
    })

    Context("when given invalid input", func() {
        It("returns error for negative amounts", func() {
            _, err := parser.ParseAmount("-10")
            Expect(err).To(HaveOccurred())
        })

        It("returns error for empty string", func() {
            _, err := parser.ParseAmount("")
            Expect(err).To(HaveOccurred())
        })
    })
})
```

**Structure guidelines:**
- **Describe** — Describes a component, function, or feature
- **Context** — Describes a specific scenario or condition
- **It** — Describes expected behavior in that context
- Use descriptive strings that read like sentences
- Nest contexts to organize related scenarios

## Table-Driven Tests with DescribeTable

DescribeTable is the Ginkgo equivalent of table-driven tests:

```go
var _ = Describe("ParseAmount", func() {
    DescribeTable("parsing different inputs",
        func(input string, expected int64, shouldError bool) {
            result, err := parser.ParseAmount(input)

            if shouldError {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).ToNot(HaveOccurred())
                Expect(result).To(Equal(expected))
            }
        },
        Entry("whole dollars", "42", int64(4200), false),
        Entry("with cents", "42.50", int64(4250), false),
        Entry("zero amount", "0", int64(0), false),
        Entry("large amount", "9999.99", int64(999999), false),
        Entry("negative amount", "-10", int64(0), true),
        Entry("empty string", "", int64(0), true),
        Entry("invalid format", "abc", int64(0), true),
    )
})
```

**DescribeTable guidelines:**
- First parameter is the table description
- Second parameter is the test function
- Each Entry is a test case with a descriptive name
- Parameters match the test function signature
- Use zero values for unused parameters in error cases

### Advanced Table Testing

For complex scenarios, use PEntry (pending) and FEntry (focused) to control execution:

```go
DescribeTable("complex scenarios",
    func(input string, expected Result) {
        // test implementation
    },
    Entry("working case", "foo", expectedFoo),
    PEntry("not implemented yet", "bar", expectedBar), // Skipped
    FEntry("debug this case", "baz", expectedBaz),     // Only this runs when present
)
```

## Setup and Teardown

Ginkgo provides several hooks for setup and teardown:

```go
var _ = Describe("UserService", func() {
    var (
        service *UserService
        repo    *mockUserRepo
        ctx     context.Context
    )

    BeforeEach(func() {
        // Runs before each It block
        repo = &mockUserRepo{users: make(map[string]*User)}
        service = NewUserService(repo)
        ctx = context.Background()
    })

    AfterEach(func() {
        // Runs after each It block
        // Cleanup resources
    })

    BeforeSuite(func() {
        // Runs once before the entire suite
        // Setup expensive resources (databases, etc.)
    })

    AfterSuite(func() {
        // Runs once after the entire suite
        // Cleanup expensive resources
    })

    It("creates a user", func() {
        user := &User{ID: "user-1", Name: "Alice"}
        err := service.Create(ctx, user)
        Expect(err).ToNot(HaveOccurred())
    })
})
```

### DeferCleanup

Use DeferCleanup for resource cleanup (similar to t.Cleanup):

```go
var _ = Describe("Database operations", func() {
    var db *sql.DB

    BeforeEach(func() {
        var err error
        db, err = sql.Open("postgres", testDSN)
        Expect(err).ToNot(HaveOccurred())

        DeferCleanup(func() {
            db.Close()
        })
    })

    It("performs query", func() {
        // Use db
    })
})
```

## Test Fixtures and testdata/

Use `testdata/` directories for test fixtures (same as stdlib testing):

```go
var _ = Describe("ParseFile", func() {
    It("parses valid JSON file", func() {
        data, err := os.ReadFile("testdata/valid_input.json")
        Expect(err).ToNot(HaveOccurred())

        result, err := parser.ParseJSON(data)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).ToNot(BeNil())
    })
})
```

## Interface-Based Test Doubles

Use the same function-based mock and struct-based stub patterns from the `go-testing` skill. The only difference is how they're wired into Ginkgo's setup blocks.

### Ginkgo Mock Wiring

```go
var _ = Describe("UserService", func() {
    var (
        service *UserService
        repo    *UserRepositoryFunc
    )

    BeforeEach(func() {
        repo = &UserRepositoryFunc{}
        service = NewUserService(repo)
    })

    Describe("Activate", func() {
        var savedUser *User

        BeforeEach(func() {
            savedUser = nil
            repo.FindByIDFunc = func(_ context.Context, id string) (*User, error) {
                if id == "user-1" {
                    return &User{ID: "user-1", Status: StatusInactive}, nil
                }
                return nil, ErrNotFound
            }
            repo.SaveFunc = func(_ context.Context, user *User) error {
                savedUser = user
                return nil
            }
        })

        It("activates an inactive user", func() {
            err := service.Activate(context.Background(), "user-1")
            Expect(err).ToNot(HaveOccurred())
            Expect(savedUser).ToNot(BeNil())
            Expect(savedUser.Status).To(Equal(StatusActive))
        })

        It("returns error for non-existent user", func() {
            err := service.Activate(context.Background(), "user-999")
            Expect(err).To(MatchError(ErrNotFound))
        })
    })
})
```

## Running Tests

Key `ginkgo` CLI commands:

```bash
ginkgo -r              # Run all tests recursively
ginkgo -v              # Verbose output
ginkgo --focus="..."   # Run matching specs only
ginkgo -p --race       # Parallel with race detector
ginkgo watch -r        # Watch mode
ginkgo -r --cover --coverprofile=coverage.out  # Coverage
```

## Test Organization

- One Describe per file matching the function/type under test
- Nest Context blocks for different scenarios (max 3-4 levels deep)
- Use descriptive strings that read like sentences

## Testing Anti-Patterns

- **Over-nesting contexts** — More than 3-4 levels becomes hard to read
- **Testing private functions** — Test public APIs, not implementation
- **Using Sleep for timing** — Use Eventually/Consistently for async operations
- **Not using DeferCleanup** — Resource leaks from unclosed connections
- **Focused specs in commits** — FIt, FDescribe should never reach main branch
- **Empty It blocks** — Either implement or mark as PIt (pending)
- **Assertions in BeforeEach** — Setup should not contain test assertions
- **Complex matchers for simple checks** — `Expect(x).To(Equal(true))` should be `Expect(x).To(BeTrue())`
- **Mocking everything** — Only mock at system boundaries
- **Not using DescribeTable** — Table tests are clearer than multiple Its
- **Matcher negation confusion** — Use `ToNot` instead of `NotTo` for consistency
- **Testing log output** — Test behavior, not logging side effects
- **Shared mutable state** — Each test must be independent

## Testing Strategy

Follow the same test pyramid and mocking boundaries as the `go-testing` skill. The only difference: use Ginkgo's Describe/Context/It organization instead of table-driven tests.

## Additional Resources

- For the complete Gomega matcher reference, see [matchers-reference.md](references/matchers-reference.md)
- For integration test setup and examples, see [integration-testing.md](references/integration-testing.md)
- For advanced topics (focused/pending specs, shared examples, async testing, HTTP handlers, parallel specs, golden files), see [advanced-topics.md](references/advanced-topics.md)
- For Makefile targets, see [makefile-targets.md](references/makefile-targets.md)
## Migrating from Standard Testing

1. **Install and bootstrap**: Run `ginkgo bootstrap` in each package
2. **Convert test functions**: Change `func TestX(t *testing.T)` to `Describe` blocks
3. **Convert assertions**: Change `if` + `t.Error` to `Expect().To()` matchers
4. **Convert table tests**: Use `DescribeTable` instead of slice + for loop
5. **Convert setup/teardown**: Use `BeforeEach`/`AfterEach` instead of helper functions
6. **Update CI/CD**: Replace `go test` with `ginkgo` commands
