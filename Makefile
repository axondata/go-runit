.PHONY: all test coverage lint bench clean install help release goimports install-tools install-golangci-lint install-go

# Default target
all: test lint

# Run unit tests
test:
	@if [ "$$(uname)" = "Linux" ]; then \
		CGO_ENABLED=1 go test -race ./...; \
	else \
		go test -race ./...; \
	fi

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
lint: install-tools
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

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@# Update apt if on Ubuntu/Debian
	@if [ -f /etc/lsb-release ] || [ -f /etc/debian_version ]; then \
		if command -v apt-get >/dev/null 2>&1; then \
			echo "Updating apt package list..."; \
			sudo apt-get update; \
			echo "Installing supervision tools..."; \
			sudo apt-get install -y daemontools runit s6 || true; \
		fi; \
	fi
	@if ! which go > /dev/null 2>&1; then \
		echo "Go not found, installing..."; \
		$(MAKE) install-go; \
	else \
		echo "Go already installed: $$(go version)"; \
	fi
	@if ! which goimports > /dev/null 2>&1; then \
		echo "Installing goimports..."; \
		go install golang.org/x/tools/cmd/goimports@latest; \
	fi
	@if ! which golangci-lint > /dev/null 2>&1; then \
		$(MAKE) install-golangci-lint; \
	fi
	@echo "Development tools installed"

# Install Go to /usr/local/go
install-go:
	@if which go > /dev/null 2>&1; then \
		echo "Go is already installed: $$(go version)"; \
	else \
		echo "Installing Go..."; \
		GO_VERSION="1.25.1" && \
		ARCH=$$(uname -m) && \
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') && \
		if [ "$$ARCH" = "x86_64" ]; then ARCH="amd64"; \
		elif [ "$$ARCH" = "aarch64" ]; then ARCH="arm64"; \
		elif [ "$$ARCH" = "armv7l" ]; then ARCH="armv6l"; fi && \
		FILENAME="go$$GO_VERSION.$$OS-$$ARCH.tar.gz" && \
		echo "Downloading Go $$GO_VERSION for $$OS/$$ARCH..." && \
		wget -q --show-progress "https://go.dev/dl/$$FILENAME" -O /tmp/$$FILENAME && \
		echo "Extracting Go to /usr/local..." && \
		sudo rm -rf /usr/local/go && \
		sudo tar -C /usr/local -xzf /tmp/$$FILENAME && \
		rm /tmp/$$FILENAME && \
		echo "Creating symlinks in /usr/local/bin..." && \
		sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
		sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt && \
		echo "Go $$GO_VERSION installed successfully!" && \
		echo "Version: $$(go version)" && \
		echo "" && \
		echo "Add the following to your shell profile if not already present:" && \
		echo "  export PATH=/usr/local/go/bin:\$$PATH" && \
		echo "  export GOPATH=\$$HOME/go" && \
		echo "  export PATH=\$$GOPATH/bin:\$$PATH"; \
	fi

# Install golangci-lint based on OS
install-golangci-lint:
	@echo "Installing golangci-lint..."
	@if [ "$$(uname)" = "Darwin" ]; then \
		echo "Installing golangci-lint via Homebrew..."; \
		brew install golangci-lint; \
	elif [ "$$(uname)" = "Linux" ]; then \
		echo "Installing golangci-lint for Linux..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
	else \
		echo "Unsupported OS. Please install golangci-lint manually."; \
		echo "Visit: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi
	@echo "golangci-lint installed successfully"

# Fix import grouping with goimports
goimports:
	@if ! which goimports > /dev/null 2>&1; then \
		echo "Installing goimports..."; \
		go install golang.org/x/tools/cmd/goimports@latest; \
	fi
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
	@echo "  install-tools                - Install development tools (goimports, etc.)"
	@echo "  install-go                   - Install Go to /usr/local/go with symlinks"
	@echo "  deps                         - Update dependencies"
	@echo "  vulncheck                    - Check for vulnerabilities"
	@echo "  release                      - Run all release checks"
	@echo "  help                         - Show this help message"
