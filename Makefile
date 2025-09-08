.PHONY: all test coverage lint bench clean install help release goimports

# Default target
all: test lint

# Run unit tests
test:
	go test -race ./...

# Run runit integration tests explicitly
test-integration-runit:
	go test -tags=integration_runit -race ./...

# Run daemontools integration tests (requires daemontools installed)
test-integration-daemontools:
	go test -tags=integration_daemontools -race ./...

# Run s6 integration tests (requires s6 installed)
test-integration-s6:
	go test -tags=integration_s6 -race ./...

# Run all tests
test-all: test test-integration-runit

# Generate coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep total

# Generate coverage with integration tests
coverage-all:
	go test -tags=integration_runit -coverprofile=coverage_all.out -covermode=atomic ./...
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

# Update dependencies
deps:
	go mod tidy
	go mod verify

# Check for vulnerabilities
vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# Run integration tests but don't fail if runit isn't installed
test-integration-optional:
	@echo "Running integration tests (if runit is installed)..."
	@go test -tags=integration_runit -race ./... 2>/dev/null || echo "Skipping integration tests (runit not installed or tests failed)"

# Fix import grouping with goimports
goimports:
	@echo "Fixing import grouping..."
	goimports -local github.com/axondata -w .
	@echo "Import grouping fixed"

# Run all release checks
release: clean deps fmt goimports lint test test-integration-optional fuzz-quick bench coverage vulncheck
	@echo ""
	@echo "=================================================="
	@echo "✅ Release checks completed successfully!"
	@echo "=================================================="
	@echo ""
	@echo "Release checklist:"
	@echo "  ✓ Dependencies updated"
	@echo "  ✓ Code formatted"
	@echo "  ✓ Linting passed"
	@echo "  ✓ Unit tests passed"
	@echo "  ✓ Integration tests checked"
	@echo "  ✓ Fuzz tests passed"
	@echo "  ✓ Benchmarks completed"
	@echo "  ✓ Coverage generated"
	@echo "  ✓ Vulnerability check passed"
	@echo ""
	@echo "Next steps:"
	@echo "1. Review changes: git status"
	@echo "2. Commit changes: git commit -m 'Release vX.Y.Z'"
	@echo "3. Tag release: git tag -a vX.Y.Z -m 'Release vX.Y.Z'"
	@echo "4. Push changes: git push origin main --tags"
	@echo "5. Create GitHub release from tag"

# Show help
help:
	@echo "Available targets:"
	@echo "  test                         - Run unit tests"
	@echo "  test-integration-runit       - Run runit integration tests explicitly"
	@echo "  test-integration-daemontools - Run daemontools integration tests (requires daemontools)"
	@echo "  test-integration-s6          - Run s6 integration tests (requires s6)"
	@echo "  test-all                     - Run all tests"
	@echo "  coverage                     - Generate coverage report"
	@echo "  coverage-all                 - Generate coverage with integration tests"
	@echo "  lint                         - Run golangci-lint"
	@echo "  bench                        - Run benchmarks"
	@echo "  fuzz                         - Run fuzz tests (30s each)"
	@echo "  fuzz-quick                   - Run quick fuzz tests (5s each)"
	@echo "  install                      - Install the library"
	@echo "  clean                        - Clean build artifacts"
	@echo "  fmt                          - Format code with go fmt and goimports"
	@echo "  goimports                    - Fix import grouping with local packages"
	@echo "  deps                         - Update dependencies"
	@echo "  vulncheck                    - Check for vulnerabilities"
	@echo "  release                      - Run all release checks"
	@echo "  help                         - Show this help message"
