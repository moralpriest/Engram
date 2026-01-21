# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.6.x   | :white_check_mark: |
| < 0.6   | :x:                |

## Reporting a Vulnerability

Engram is a cryptocurrency wallet that handles sensitive user data and funds.
We take security vulnerabilities extremely seriously.

### How to Report

**Please DO NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via one of the following methods:

1. **GitHub Security Advisories** (Preferred)
   - Go to the [Security tab](https://github.com/DEROFDN/Engram/security/advisories)
   - Click "Report a vulnerability"
   - Provide detailed information about the vulnerability

2. **Email**
   - Contact the maintainers directly (see repository for contact info)
   - Use encryption if possible (PGP key available upon request)

### What to Include

Please include as much of the following information as possible:

- Type of vulnerability (e.g., key exposure, injection, authentication bypass)
- Full paths of source files related to the vulnerability
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it
- Any potential mitigations you've identified

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 7 days
- **Status Update**: Every 7 days until resolution
- **Resolution Target**: 90 days for critical vulnerabilities

### Disclosure Policy

- We follow coordinated disclosure practices
- We will work with you to understand and resolve the issue
- We will credit reporters (unless they prefer to remain anonymous)
- We ask that you give us reasonable time to address the issue before public disclosure

### Scope

The following are in scope for security reports:

- Private key handling and storage
- Wallet encryption/decryption
- Transaction signing
- Network communication security
- Authentication and authorization
- Input validation vulnerabilities
- Dependency vulnerabilities

### Out of Scope

- Social engineering attacks
- Physical attacks
- Denial of service attacks
- Issues in third-party dependencies (report to upstream)
- Issues that require physical access to a user's device

## Security Best Practices for Users

1. **Always verify downloads**
   - Check SHA256 checksums
   - Verify Cosign signatures (see release notes)

2. **Backup your wallet**
   - Store seed phrases securely offline
   - Never share your seed phrase or private keys

3. **Keep software updated**
   - Always use the latest release
   - Subscribe to release notifications

4. **Use strong passwords**
   - Use unique, strong passwords for wallet encryption
   - Consider using a password manager

## Security Measures in Engram

- All releases are signed with Sigstore Cosign
- SBOM (Software Bill of Materials) provided for each release
- Automated vulnerability scanning in CI/CD
- Dependency updates monitored via Dependabot
- Code analysis via CodeQL, Gosec, and Semgrep

## Code Security Analysis

Automated static analysis is performed on all code changes to identify potential security issues.

### Tools Used

| Tool | Purpose |
|------|---------|
| gosec | Go security scanner for common vulnerabilities |
| CodeQL | GitHub's code analysis engine |
| Semgrep | Fast static analysis tool |
| govulncheck | Go vulnerability checker |

### Scan Results

| Tool | Status | Issues |
|------|--------|--------|
| gosec | Pass | 0 |
| CodeQL | Pass | 0 |
| Semgrep | Pass | 0 |

### Accepted False Positives

gosec may report G104 (unhandled errors) for Fyne UI methods. These are acceptable because:

- Fyne data binding methods (`Set`, `Reload`) return errors for API completeness only
- There is no meaningful error handling possible for UI state changes
- See [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) for detailed audit information

## Acknowledgments

We thank the following individuals for responsibly disclosing security issues:

*No reports yet - be the first!*
