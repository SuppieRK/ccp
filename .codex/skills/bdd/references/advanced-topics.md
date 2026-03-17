# Advanced Topics

## Contents

- [Focused and Pending Specs](#focused-and-pending-specs)
- [Shared Examples](#shared-examples)
- [Asynchronous Testing](#asynchronous-testing)
- [Testing HTTP Handlers](#testing-http-handlers)
- [Parallel Specs](#parallel-specs)
- [Golden File Testing](#golden-file-testing)

## Focused and Pending Specs

Control which specs run during development:

```go
// FDescribe, FContext, FIt - Only these run when present
var _ = FDescribe("UserService", func() {
    FIt("only runs this test", func() {
        // This runs, others are skipped
    })
})

// PDescribe, PContext, PIt - Marked as pending, skipped
var _ = PDescribe("Future feature", func() {
    PIt("not implemented yet", func() {
        // This is skipped
    })
})

// XDescribe, XContext, XIt - Explicitly disabled
var _ = XDescribe("Broken tests", func() {
    XIt("temporarily disabled", func() {
        // This is skipped
    })
})
```

**Important**: Never commit focused specs (F-prefixed blocks). They cause all other tests to be skipped. Use them only during local development.

## Shared Examples

Extract common test patterns:

```go
// Define shared behavior
var ItBehavesLikeARepository = func() {
    It("saves and retrieves entities", func() {
        // common test
    })

    It("handles non-existent entities", func() {
        // common test
    })
}

// Use in multiple suites
var _ = Describe("UserRepository", func() {
    ItBehavesLikeARepository()

    It("has user-specific behavior", func() {
        // user-specific test
    })
})

var _ = Describe("ProductRepository", func() {
    ItBehavesLikeARepository()

    It("has product-specific behavior", func() {
        // product-specific test
    })
})
```

## Asynchronous Testing

Use Eventually and Consistently for async operations:

```go
var _ = Describe("AsyncProcessor", func() {
    It("processes messages asynchronously", func() {
        processor := NewProcessor()
        processor.Start()
        defer processor.Stop()

        processor.Submit("message1")

        // Poll until condition is true
        Eventually(func() int {
            return processor.ProcessedCount()
        }).WithTimeout(5 * time.Second).
          WithPolling(100 * time.Millisecond).
          Should(Equal(1))

        // Verify condition stays true
        Consistently(func() bool {
            return processor.IsHealthy()
        }).WithTimeout(2 * time.Second).Should(BeTrue())
    })
})
```

## Testing HTTP Handlers

```go
var _ = Describe("HTTP Handlers", func() {
    var (
        router *http.ServeMux
        rec    *httptest.ResponseRecorder
    )

    BeforeEach(func() {
        router = http.NewServeMux()
        router.HandleFunc("/users", handlers.CreateUser)
        rec = httptest.NewRecorder()
    })

    It("creates a user", func() {
        body := `{"name":"Alice","email":"alice@example.com"}`
        req := httptest.NewRequest("POST", "/users", strings.NewReader(body))
        req.Header.Set("Content-Type", "application/json")

        router.ServeHTTP(rec, req)

        Expect(rec.Code).To(Equal(http.StatusCreated))

        var response map[string]string
        err := json.Unmarshal(rec.Body.Bytes(), &response)
        Expect(err).ToNot(HaveOccurred())
        Expect(response["id"]).ToNot(BeEmpty())
    })
})
```

## Parallel Specs

Enable parallel execution for independent tests:

```go
var _ = Describe("ParallelTests", func() {
    It("runs in parallel", func(ctx SpecContext) {
        // Tests with SpecContext automatically run in parallel
        // when using ginkgo -p
    })
})
```

Or use explicit labels:

```bash
ginkgo -r -p --label-filter="parallel"
```

```go
var _ = Describe("Tests", Label("parallel"), func() {
    It("test 1", func() { /* ... */ })
    It("test 2", func() { /* ... */ })
})
```

## Golden File Testing

```go
var _ = Describe("Render", func() {
    It("generates expected JSON output", func() {
        result := Render(testInput)
        golden := filepath.Join("testdata", "golden", "expected.json")

        if *updateGolden {
            err := os.WriteFile(golden, result, 0644)
            Expect(err).ToNot(HaveOccurred())
            Skip("golden file updated")
        }

        expected, err := os.ReadFile(golden)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).To(MatchJSON(expected))
    })
})

var updateGolden = flag.Bool("update-golden", false, "update golden files")
```
