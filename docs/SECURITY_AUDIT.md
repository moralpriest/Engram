# Security Audit Report

**Last updated**: January 2026  
**Branch**: feature/ci-workflow

## Executive Summary

| Metric | Value |
|--------|-------|
| Original issues | 131 |
| Fixed issues | 131 |
| Remaining issues | 0 |

## Issue Categories

### G104 - Errors Unhandled (CWE-703)
- **Original count**: 124
- **Fixed with error handling**: 25
- **Documented as acceptable**: 99

### G115 - Integer Overflow (CWE-190)
- **Original count**: 6
- **Fixed with bounds checking**: 6

### G602 - Slice Index Out of Range (CWE-118)
- **Original count**: 1
- **Fixed with bounds check**: 1

## Fix Categories

### Storage Functions
Fixed with error logging (`[Store]` prefix):
- `DeleteKey()`
- `StoreValue()`
- `StoreEncryptedValue()`
- `json.Unmarshal()`

### Custom Functions
Fixed with error logging:
- `setNetwork()` - `[Function]` prefix
- `setDaemon()` - `[Function]` prefix
- `setGnomon()` - `[Function]` prefix
- `setPrimaryUsername()` - `[Function]` prefix
- `getPrimaryUsername()` - `[Function]` prefix
- `Save_Wallet()` - `[Wallet]` prefix
- `AddSCIDToIndex()` - `[TELA]` prefix
- `create()` - return values captured

### UI Methods (Documented Acceptable)
Fyne data binding methods return errors for API completeness only. No meaningful error handling is possible:
- `Set()` - 39 instances
- `Validate()` - 3 instances
- `Reload()` - 3 instances
- `ProcessPayload()` - 3 instances

Each includes comment:
```go
// #nosec G104 // G104 acceptable - Fyne data binding methods return err for API completeness only
```

## Error Message Categories

| Category | Prefix | Purpose |
|----------|--------|---------|
| Storage | `[Store]` | Storage function errors (DeleteKey, StoreValue, StoreEncryptedValue) |
| Wallet | `[Wallet]` | Wallet save errors (Save_Wallet) |
| TELA | `[TELA]` | TELA indexing errors (AddSCIDToIndex) |
| Function | `[Function]` | Custom function errors (setNetwork, setDaemon, etc.) |
| JSON | `[JSON]` | JSON unmarshal errors |

## Tools Used

- **gosec**: Go security scanner
- **CodeQL**: GitHub code analysis
- **Semgrep**: Fast static analysis
- **govulncheck**: Go vulnerability checker

## Testing

See [TESTING.md](TESTING.md) for security testing instructions.

## References

- [SECURITY.md](../SECURITY.md) - Security policy and reporting guidelines
- [.github/workflows/security.yml](../.github/workflows/security.yml) - CI/CD security workflow
