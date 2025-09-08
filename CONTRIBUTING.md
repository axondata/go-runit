# Contributing to go-runit

Thank you for your interest in contributing to go-runit! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please be respectful and constructive in all interactions.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/go-runit.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run tests: `make test-all`
6. Submit a pull request

## Development Setup

### Prerequisites

- Go 1.21 or later
- runit installed (for integration tests)
- golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

### Running Tests

```bash
# Unit tests only
make test

# Integration tests (requires runit)
make test-integration

# All tests
make test-all

# With coverage
make coverage-all
```

### Code Quality

Before submitting a PR, ensure:

1. All tests pass: `make test-all`
2. Code is formatted: `make fmt`
3. Linter passes: `make lint`
4. Benchmarks run: `make bench`

## Pull Request Process

1. Update README.md with details of changes if applicable
2. Add tests for new functionality
3. Ensure all tests pass
4. Update documentation and examples
5. Submit PR with clear description of changes

## Testing Guidelines

- Write unit tests for all new functions
- Integration tests should use real runit processes
- Use table-driven tests where appropriate
- Aim for >80% code coverage

## Code Style

- Follow standard Go conventions
- Use meaningful variable names
- Add comments for exported functions
- Keep functions focused and small
- Use constants instead of magic numbers

## Documentation

- All exported types and functions must have godoc comments
- Include examples in documentation where helpful
- Update README for user-facing changes

## Reporting Issues

When reporting issues, please include:

- Go version (`go version`)
- Operating system and version
- runit version
- Minimal code to reproduce the issue
- Expected vs actual behavior

## Performance Considerations

This library prioritizes:
- Zero allocations on hot paths (status decode achieves 0 allocs/op)
- Direct system calls over process spawning
- Efficient binary parsing
- Concurrent operations where beneficial

When contributing performance improvements:
- Include benchmark comparisons
- Document the optimization approach
- Ensure correctness is maintained

## Questions?

Feel free to open an issue for questions or discussions.
