# Contributing to DockRouter

Thank you for your interest in contributing to DockRouter! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Commit Guidelines](#commit-guidelines)
- [Pull Request Process](#pull-request-process)

## Code of Conduct

Be respectful and inclusive. We welcome contributions from everyone.

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/dockrouter.git
   cd dockrouter
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/DockRouter/dockrouter.git
   ```

## Development Setup

### Prerequisites

- Go 1.21 or later
- Docker & Docker Compose
- Make (optional)

### Setup

```bash
# Install dependencies
go mod download

# Build the binary
make build

# Run tests
make test

# Run locally
./bin/dockrouter --log-level=debug
```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feature/add-websocket-support`
- `fix/memory-leak`
- `docs/update-readme`
- `refactor/simplify-router`

### Code Style

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` to format your code
- Run `go vet` before committing
- Ensure all tests pass

### Project Structure

```
cmd/dockrouter/     # Main application
internal/
  ├── admin/         # Admin server
  ├── config/        # Configuration
  ├── discovery/     # Docker discovery
  ├── health/        # Health checking
  ├── log/           # Logging
  ├── metrics/       # Prometheus metrics
  ├── middleware/    # HTTP middleware
  ├── proxy/         # Reverse proxy
  ├── router/        # Route management
  └── tls/           # TLS/ACME
examples/            # Example configurations
```

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package tests
go test -v ./internal/proxy/...

# Run short tests only
make test-short
```

### Writing Tests

- Write unit tests for new functionality
- Maintain or improve code coverage
- Use table-driven tests for multiple test cases
- Add integration tests for complex features

Example:
```go
func TestNewFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"case1", "input1", "output1"},
        {"case2", "input2", "output2"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := MyFunction(tt.input)
            if result != tt.expected {
                t.Errorf("got %q, want %q", result, tt.expected)
            }
        })
    }
}
```

## Commit Guidelines

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New features
- `fix:` Bug fixes
- `docs:` Documentation changes
- `style:` Code style changes (formatting, etc.)
- `refactor:` Code refactoring
- `test:` Adding or updating tests
- `chore:` Maintenance tasks

Examples:
```
feat: add WebSocket support
fix: resolve memory leak in connection pool
docs: update installation instructions
refactor: simplify route matching logic
test: add tests for TLS certificate renewal
```

## Pull Request Process

1. **Create a branch** from `main` or `develop`
2. **Make your changes** following the guidelines above
3. **Test your changes** thoroughly
4. **Commit your changes** with clear commit messages
5. **Push to your fork** and create a pull request
6. **Wait for review** and address any feedback

### PR Checklist

- [ ] Code compiles without errors
- [ ] All tests pass
- [ ] New code has test coverage
- [ ] Documentation updated (if needed)
- [ ] Commit messages follow conventions
- [ ] PR description clearly describes changes

### PR Description Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
How was this tested?

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
```

## Questions?

- Open a [Discussion](https://github.com/DockRouter/dockrouter/discussions)
- Ask in your PR

Thank you for contributing! 🎉
