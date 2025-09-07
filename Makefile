.PHONY: all test coverage lint bench clean install help

# Default target
all: test lint

# Run unit tests
test:
	go test -v -race ./...

# Run integration tests (requires runit installed)
test-integration:
	go test -tags=integration -v -race ./...

# Run all tests
test-all: test test-integration

# Generate coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep total

# Generate coverage with integration tests
coverage-all:
	go test -tags=integration -coverprofile=coverage_all.out -covermode=atomic ./...
	go tool cover -html=coverage_all.out -o coverage_all.html
	@echo "Full coverage report generated: coverage_all.html"
	@go tool cover -func=coverage_all.out | grep total

# Run linter
lint:
	golangci-lint run ./...

# Run benchmarks
bench:
	go test -bench=. -benchmem ./...

# Run fuzz tests
fuzz:
	@echo "Running fuzz tests (30s each)..."
	go test -fuzz=FuzzDecodeStatus -fuzztime=30s . || true
	go test -fuzz=FuzzMakeStatusData -fuzztime=30s . || true
	go test -fuzz=FuzzClientOperations -fuzztime=30s . || true
	go test -fuzz=FuzzStatusParsing -fuzztime=30s . || true

# Run quick fuzz tests (5s each)
fuzz-quick:
	@echo "Running quick fuzz tests (5s each)..."
	go test -fuzz=FuzzDecodeStatus -fuzztime=5s . || true
	go test -fuzz=FuzzMakeStatusData -fuzztime=5s . || true
	go test -fuzz=FuzzClientOperations -fuzztime=5s . || true
	go test -fuzz=FuzzStatusParsing -fuzztime=5s . || true

# Install the library
install:
	go install ./...

# Clean build artifacts
clean:
	rm -f coverage*.out coverage*.html
	rm -rf testdata/fuzz
	go clean

# Format code
fmt:
	go fmt ./...
	goimports -w .

# Update dependencies
deps:
	go mod tidy
	go mod verify

# Check for vulnerabilities
vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  test            - Run unit tests"
	@echo "  test-integration- Run integration tests (requires runit)"
	@echo "  test-all        - Run all tests"
	@echo "  coverage        - Generate coverage report"
	@echo "  coverage-all    - Generate coverage with integration tests"
	@echo "  lint            - Run golangci-lint"
	@echo "  bench           - Run benchmarks"
	@echo "  fuzz            - Run fuzz tests (30s each)"
	@echo "  fuzz-quick      - Run quick fuzz tests (5s each)"
	@echo "  install         - Install the library"
	@echo "  clean           - Clean build artifacts"
	@echo "  fmt             - Format code"
	@echo "  deps            - Update dependencies"
	@echo "  vulncheck       - Check for vulnerabilities"
	@echo "  help            - Show this help message"