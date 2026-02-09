# Testing Guide

This document outlines testing procedures for the Engram wallet application.

## Security Testing

### Running Security Scans

The project uses multiple security scanning tools that can be run locally:

```bash
# Run gosec security scanner
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec -exclude=G104,G115,G602 -exclude-dir=vendor -tags migrated_fynedo ./...

# Run govulncheck for vulnerability scanning
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck -tags migrated_fynedo ./...

# Run golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run --timeout=5m
```

### CI/CD Security Checks

All security checks are automated in the CI/CD pipeline via GitHub Actions:

- **Gitleaks**: Secret scanning on every push
- **govulncheck**: Go vulnerability checking
- **gosec**: Static security analysis
- **CodeQL**: Deep semantic code analysis
- **Semgrep**: Fast static analysis with custom rules
- **Trivy**: Container and filesystem vulnerability scanning

See [`.github/workflows/security.yml`](../.github/workflows/security.yml) for the complete security workflow configuration.

## Unit Testing

```bash
# Run all tests
go test -v -race -tags migrated_fynedo ./...

# Run tests with coverage
go test -v -race -coverprofile=coverage.out -tags migrated_fynedo ./...
```

## Build Testing

```bash
# Build for current platform
go build -v -trimpath -tags migrated_fynedo .

# Build for Linux (requires Fyne dependencies)
fyne package -os linux -name Engram -icon Icon.png -tags migrated_fynedo

# Build for other platforms
fyne package -os windows -name Engram -icon Icon.png -tags migrated_fynedo
fyne package -os darwin -name Engram -icon Icon.png -tags migrated_fynedo
```

## Fuzz Testing

The project includes fuzz testing that runs weekly:

```bash
# Run fuzz tests locally
go test -fuzz=FuzzTarget -fuzztime=60s -tags migrated_fynedo ./...
```

See [`.github/workflows/fuzz.yml`](../.github/workflows/fuzz.yml) for automated fuzz testing configuration.
