# Gomega Matchers

Gomega provides expressive matchers for assertions:

## Common Matchers

```go
// Equality
Expect(actual).To(Equal(expected))
Expect(actual).To(BeNumerically("==", expected))
Expect(actual).To(BeIdenticalTo(expected)) // pointer equality

// Nil checks
Expect(err).ToNot(HaveOccurred())
Expect(err).To(MatchError("expected error message"))
Expect(value).To(BeNil())
Expect(value).ToNot(BeNil())

// Boolean
Expect(condition).To(BeTrue())
Expect(condition).To(BeFalse())

// Numeric comparisons
Expect(value).To(BeNumerically(">", 10))
Expect(value).To(BeNumerically("<=", 100))
Expect(value).To(BeNumerically("~", 3.14, 0.01)) // within delta

// Strings
Expect(str).To(ContainSubstring("expected"))
Expect(str).To(HavePrefix("prefix"))
Expect(str).To(HaveSuffix("suffix"))
Expect(str).To(MatchRegexp(`\d{3}-\d{4}`))

// Collections
Expect(slice).To(HaveLen(5))
Expect(slice).To(BeEmpty())
Expect(slice).To(ContainElement("item"))
Expect(slice).To(ConsistOf("a", "b", "c")) // order doesn't matter
Expect(map).To(HaveKey("key"))
Expect(map).To(HaveKeyWithValue("key", "value"))

// Types
Expect(value).To(BeAssignableToTypeOf(MyType{}))

// Channels
Expect(ch).To(BeClosed())
Expect(ch).To(Receive())
Expect(ch).To(Receive(&result))

// Eventually and Consistently (async)
Eventually(func() int { return counter.Get() }).Should(Equal(10))
Eventually(fetchStatus).WithTimeout(5*time.Second).Should(Equal("ready"))
Consistently(isHealthy).WithTimeout(10*time.Second).Should(BeTrue())
```

## Custom Matchers

Create reusable custom matchers:

```go
func HaveStatusCode(expected int) types.GomegaMatcher {
    return &statusCodeMatcher{expected: expected}
}

type statusCodeMatcher struct {
    expected int
}

func (m *statusCodeMatcher) Match(actual interface{}) (bool, error) {
    resp, ok := actual.(*http.Response)
    if !ok {
        return false, fmt.Errorf("expected *http.Response, got %T", actual)
    }
    return resp.StatusCode == m.expected, nil
}

func (m *statusCodeMatcher) FailureMessage(actual interface{}) string {
    resp := actual.(*http.Response)
    return fmt.Sprintf("Expected status code %d, got %d", m.expected, resp.StatusCode)
}

func (m *statusCodeMatcher) NegatedFailureMessage(actual interface{}) string {
    resp := actual.(*http.Response)
    return fmt.Sprintf("Expected status code not to be %d", resp.StatusCode)
}

// Usage
Expect(response).To(HaveStatusCode(200))
```
