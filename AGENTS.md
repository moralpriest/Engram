# AGENTS Guide

This file is for coding agents working in `/home/priest/Projects/Engram`.

## Project Snapshot

- Language: Go 1.26
- App type: Fyne desktop/mobile wallet for the DERO ecosystem
- Module path: `github.com/DEROFDN/engram`
- Primary areas: wallet UI, message flows, Gnomon indexing, local encrypted storage
- Build tags commonly required: `migrated_fynedo`

## Rule Files

- No `.cursorrules` file was found.
- No `.cursor/rules/` directory was found.
- No `.github/copilot-instructions.md` file was found.
- If any of those files are added later, treat them as higher-priority repo instructions.

## Core Commands

### Preferred Taskfile commands

- Build app: `task build`
- Run app: `task run`
- Run tests: `task test`
- Run lint: `task lint`
- Auto-fix lint issues: `task lint-fix`
- Format code: `task fmt`
- Tidy modules: `task tidy`
- Run local CI set: `task ci`
- Run all checks: `task check`
- Run security suite: `task security`
- List tasks: `task --list`

### Direct Go commands

- Build repo binary: `go build -tags migrated_fynedo -ldflags "-X main.versionString=$(git describe --tags --always 2>/dev/null | sed 's/^v//')" -o bin/engram .`
- Simple build for quick verification: `go build ./...`
- Run all tests: `go test ./...`
- Run tests with race and coverage like Taskfile: `go test -v -race -tags migrated_fynedo -coverprofile=coverage.out ./...`

### Running a single test

- Single named test in all packages:
  - `go test -tags migrated_fynedo -run '^TestDecodeHex$' ./...`
- Single test in this module/package only:
  - `go test -tags migrated_fynedo -run '^TestDecodeHex$' .`
- Single fuzz target as regression test seed execution:
  - `go test -tags migrated_fynedo -run '^FuzzStoreValueInputs$' .`
- Run a fuzz target actively:
  - `go test -tags migrated_fynedo -fuzz '^FuzzStoreValueInputs$' .`

### Packaging

- Linux package: `task package-linux`
- Windows package: `task package-windows`
- macOS package: `task package-macos`
- Android package: `task package-android`

### Useful generated artifacts

- Main task-built binary: `bin/engram`
- Coverage report: `coverage.html`
- Dist artifacts: `dist/`

## Repository Conventions

### Formatting

- Always run `gofmt -w` on modified Go files.
- Prefer also running `goimports -w` if imports changed.
- `task fmt` is the canonical repo formatter.
- Do not manually align spacing; let Go tooling format it.

### Imports

- Follow normal Go import grouping as produced by `goimports`.
- Standard library imports first.
- Third-party imports second.
- Avoid unused imports; the build should stay clean.

### Naming

- Exported names use Go PascalCase.
- Unexported helpers use camelCase.
- Constants use existing repo style, often `ALL_CAPS` for app-wide constants.
- Keep names descriptive over short, cryptic abbreviations unless matching surrounding code.
- Reuse existing domain names where possible: `gnomon`, `session`, `engram`, `messages`, `status`.

### Types and data flow

- Prefer concrete structs already used by the app over introducing abstractions too early.
- Keep UI state in existing shared structs when following current architecture.
- Avoid introducing generics unless clearly justified; the codebase is conventional Go.
- Preserve existing DERO/Fyne types rather than wrapping them unnecessarily.

### Error handling

- Return errors rather than panic in normal flows.
- Log actionable runtime context with `logger.Printf/Warnf/Errorf` where the code already logs.
- Keep user-facing failures distinct from debug logs.
- When adding fallbacks, preserve the original behavior in logs where useful.
- Prefer explicit error branches over deeply nested logic.

### Control flow

- Favor early returns for validation failures.
- Keep UI event handlers readable; move heavy or blocking work into helpers or goroutines.
- If work touches the UI from a goroutine, use `fyne.Do(...)` for UI updates.

### Concurrency

- Be careful with blocking calls in Fyne button handlers and page construction.
- Avoid blocking the UI thread with network, wallet, or indexing calls.
- If adding goroutines, guard shared mutable state.
- Existing code uses package-level shared state heavily; do not assume thread safety.

## Testing Guidance

- Start with `go test ./...` for quick validation.
- For behavior tied to build tags or race-sensitive flows, use `task test`.
- When touching parsing, storage, or helpers, look at:
  - `functions_test.go`
  - `fuzz_test.go`
- If adding logic to storage or message classification, add focused unit tests when practical.

## Lint and Quality

- Run `task lint` after meaningful Go changes.
- Run `task lint-fix` only if you are prepared to review all auto-edits.
- Run `task tidy` if dependencies or module metadata changed.
- Run `task check` before handing off larger changes.

## Security and Wallet-Safety Notes

- This is wallet software; be conservative with changes touching transaction creation, RPC, signing, payloads, and storage.
- Do not log secrets, seeds, or decrypted sensitive content.
- Be careful when touching local encrypted storage in `store.go` and wallet memory flows.
- Treat daemon RPC issues, wallet history, and username resolution as separate concerns unless proven otherwise.

## UI and Fyne Notes

- Preserve current Fyne patterns already used in `layouts.go`.
- Keep mobile and desktop behavior in mind; this codebase targets both.
- Prefer incremental UI fixes over broad rewrites.
- If adding background work for UI screens, ensure reload/navigation does not race stale updates.

## Messaging and History Notes

- Messaging is encoded as normal DERO transfers with payload on destination port `1337`.
- Username resolution is not the same thing as message history reconstruction.
- Gnomon helps with username lookup but is not the primary source of message history.
- Wallet transfer history and payload decoding are the critical dependencies for viewing sent/received messages.

## Git and Change Hygiene

- Keep changes tightly scoped.
- Do not reformat unrelated files.
- Avoid sweeping renames unless required.
- Respect existing local modifications in the working tree.
- If a file is already dirty, read carefully before editing.

## Recommended Agent Workflow

1. Read the relevant code path completely before editing.
2. Prefer the Taskfile command when one exists.
3. Make the minimal change that matches current architecture.
4. Run `gofmt`/`goimports` on touched Go files.
5. Run at least `go test ./...`.
6. For larger changes, run `task test` and `task lint`.
7. Summarize changed files, commands run, and any remaining risks.

## Known Practical Commands for Agents

- Verify repo health quickly: `git status --short && go test ./...`
- Rebuild Linux binary: `go build -o Engram .`
- Build task-managed binary: `task build`
- Build Android APK: `task package-android`

## TELA Performance Notes

- See `TELA_PERFORMANCE.md` for the current TELA app discovery architecture and timing expectations.
- First TELA browser visit is typically ~5-10 seconds (fast prefilter with dedicated RPC pool); ~2 seconds on subsequent visits once the background backfill populates the candidate cache.
- After modifying Gnomon code, remember to run `go mod vendor` to sync Engram's vendor directory.
- Gnomon dependency points to the remote fork (`replace github.com/civilware/Gnomon => github.com/moralpriest/Gnomon v0.0.0-20260429054005-02f3d30e2477`).

## Final Notes

- Prefer evidence-driven debugging in this repo, especially around messaging, wallet history, and Gnomon.
- If runtime behavior contradicts code inspection, verify the actual binary being run before patching further.
