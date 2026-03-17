# Makefile Targets

```makefile
# Install Ginkgo CLI
install-ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Run unit tests with Ginkgo
test:
	ginkgo -r --race --randomize-all --randomize-suites \
	       --skip-package=test-integration internal/

# Run unit tests with verbose output
test-verbose:
	ginkgo -r -v --race --randomize-all --randomize-suites \
	       --skip-package=test-integration internal/

# Run unit tests with coverage
test-coverage:
	ginkgo -r --race --randomize-all --randomize-suites \
	       --cover --coverprofile=coverage.out \
	       --skip-package=test-integration internal/
	go tool cover -func=coverage.out

# Run integration tests
test-integration:
	ginkgo -r --race --randomize-all --randomize-suites \
	       ./test-integration/...

# Run all tests
test-all:
	ginkgo -r --race --randomize-all --randomize-suites

# Run tests in parallel
test-parallel:
	ginkgo -r -p --race --randomize-all --randomize-suites

# Watch mode for development
test-watch:
	ginkgo watch -r --skip-package=test-integration internal/

# Run focused specs only (local dev only)
test-focus:
	ginkgo -r --focus="$(FOCUS)" internal/

# Generate test coverage HTML report
coverage-html: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
```
