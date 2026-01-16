# Contributing to Engram

Thank you for your interest in contributing to Engram! This document provides
guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Commit Guidelines](#commit-guidelines)
- [Pull Request Process](#pull-request-process)
- [Security](#security)

## Code of Conduct

Please be respectful and constructive in all interactions. We're all here to
build great software together.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/Engram.git`
3. Add upstream remote: `git remote add upstream https://github.com/DEROFDN/Engram.git`
4. Create a feature branch: `git checkout -b feature/your-feature-name`

## Development Setup

### Prerequisites

- Go 1.23 or later
- Fyne dependencies for your platform:
  - **Linux**: `sudo apt-get install libgl1-mesa-dev xorg-dev`
  - **macOS**: Xcode command line tools
  - **Windows**: TDM-GCC-64 or MinGW-w64

### Install Development Tools

```bash
# Using Taskfile (recommended)
task setup

# Or manually:
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install fyne.io/fyne/v2/cmd/fyne@latest
```

### Install Pre-commit Hooks

```bash
pip install pre-commit
pre-commit install
pre-commit install --hook-type commit-msg
```

### Common Commands

```bash
# Build
task build

# Run tests
task test

# Run linters
task lint

# Run security checks
task security

# Run all checks
task check

# See all available commands
task
```

## Making Changes

### Branch Naming

Use descriptive branch names:

- `feature/add-dark-mode` - New features
- `fix/wallet-connection-timeout` - Bug fixes
- `security/update-vulnerable-dep` - Security fixes
- `docs/update-readme` - Documentation
- `refactor/simplify-transaction-logic` - Refactoring

### Code Style

- Follow standard Go formatting (`gofmt`)
- Run `task lint` before committing
- Keep functions focused and reasonably sized
- Add comments for non-obvious logic
- Use meaningful variable names

### Testing

- Add tests for new functionality when possible
- Run `task test` to ensure tests pass
- Use the `-race` flag (included in `task test`)

## Commit Guidelines

We use [Conventional Commits](https://www.conventionalcommits.org/). All commits
must follow this format:

```text
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

| Type       | Description                              |
| ---------- | ---------------------------------------- |
| `feat`     | New feature                              |
| `fix`      | Bug fix                                  |
| `security` | Security fix                             |
| `docs`     | Documentation changes                    |
| `style`    | Formatting, no code change               |
| `refactor` | Code change that neither fixes nor adds  |
| `perf`     | Performance improvement                  |
| `test`     | Adding or updating tests                 |
| `build`    | Build system or dependencies             |
| `ci`       | CI configuration                         |
| `chore`    | Maintenance tasks                        |
| `revert`   | Revert a previous commit                 |

### Examples

```bash
feat(wallet): add transaction history export

fix(ui): resolve layout issue on small screens

security: update crypto library to patch CVE-2024-XXXX

docs: add API documentation for transfer functions
```

### Commit Signing

All commits must be signed. We recommend SSH signing:

```bash
# Configure Git for SSH signing
git config --global gpg.format ssh
git config --global user.signingkey ~/.ssh/id_ed25519.pub
git config --global commit.gpgsign true
```

## Pull Request Process

1. **Update your branch**

   ```bash
   git fetch upstream
   git rebase upstream/dev
   ```

2. **Run all checks**

   ```bash
   task check
   ```

3. **Push your changes**

   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create a Pull Request**
   - Target the `dev` branch (not `main`)
   - Fill out the PR template completely
   - Link any related issues

5. **Address feedback**
   - Make requested changes
   - Push additional commits
   - Re-request review when ready

### PR Requirements

- All CI checks must pass
- Commits must be signed
- Code must be formatted (`gofmt`)
- No new security vulnerabilities
- Version updated if required (see PR template)

## Security

- **Never commit secrets** (keys, passwords, tokens)
- **Use `crypto/rand`** for any randomness, never `math/rand`
- **Validate all inputs** especially for amounts and addresses
- **Run `task security`** before submitting PRs
- **Report vulnerabilities** via [SECURITY.md](SECURITY.md)

## Questions?

If you have questions, feel free to:

- Open a GitHub Discussion
- Check existing issues for similar questions
- Review the codebase documentation

Thank you for contributing to Engram!
