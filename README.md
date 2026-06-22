<img src="assets/Icon.png" width="128" height="128" alt="Engram Enigma" title="Powered by DERO">

# *One Wallet. All of DERO.*

### The Engram smart wallet empowers users to easily and securely manage their money and assets on the DERO blockchain.

### Included Features
- [x]  Privately send and receive money globally
- [x]  On-chain encrypted private messaging
- [x]  Dynamically interact with smart contracts
- [x]  Native asset tracking
- [x]  Register and transfer user-friendly addresses (usernames)
- [x]  [Gnomon](https://github.com/civilware/Gnomon) integration for blockchain indexing
- [x]  Encrypted Notepad
- [x]  Websocket support for dApp/web3 connections
- [x]  Sign files using your wallet to guarantee authenticity
- [x]  **Instant TELA discovery (~2s)** using three-layer cache (Embedded, DB, JSON)
- [x]  **QR Code Scanning** (Desktop + Android)
- [x]  Supports [EPOCH](https://github.com/civilware/epoch) crowd mining protocol
- [x]  **Multi-language support** (English, Français, Español, Deutsch, Italiano, Português, Русский, 日本語, 中文, Esperanto)
- [x]  **5 built-in themes** with customizable palette — see [docs/Theme.md](docs/Theme.md)

### Upcoming Features
- [ ]  Mobile QR Scanning (iOS)

### Watch the Beta Release Video
[<img src="https://img.youtube.com/vi/00-gpNbkRW4/hqdefault.jpg" width="100%" />](https://www.youtube.com/watch?v=00-gpNbkRW4)

## Releases
We plan to deploy releases on the following platforms:
- [x]  Windows
- [x]  Linux
- [x]  Mac OS (with Camera/QR support)
- [ ]  iOS
- [x]  Android

See [releases](https://github.com/DEROFDN/Engram/releases) for the latest builds.

Platform installation guides:
- [macOS](docs/INSTALL_MAC.md)
- [Windows](docs/INSTALL_WINDOWS.md)
- [Linux](docs/INSTALL_LINUX.md)

## Development

### Using Taskfile (Recommended)

We provide a [Taskfile](https://taskfile.dev/) to automate development, testing, and packaging:

```bash
# Install task
go install github.com/go-task/task/v3/cmd/task@latest

# List all available tasks
task --list

# Common development tasks
task build          # Build the application (bin/engram)
task run            # Build and run immediately
task test           # Run tests with race detector
task lint           # Run linters (golangci-lint)

# Packaging for different platforms
task package-linux
task package-windows
task package-macos
task package-android
```

### Manual Build Requirements

Please see: https://developer.fyne.io/ for Fyne dependencies.

**Important Build Tags:**
Always use `-tags migrated_fynedo` when building manually.

* Install fyne cmd tools: `go install fyne.io/fyne/v2/cmd/fyne@latest`
* Add `~/go/bin` to your `$PATH` environment variable.

#### Building for MacOS
MacOS builds require specific entitlements for camera and microphone access.
```bash
task package-macos
```
This task automatically runs `./fix_macos_permissions.sh` to apply the necessary entitlements to the `.app` bundle.

#### Building for Windows
```bash
fyne package -name Engram -os windows -appVersion 0.6.9 -icon assets/Icon.png -tags migrated_fynedo
```

#### Building for Android
### Android Packaging (with QR support)
Packaging for Android requires patching the Fyne source and using a custom CLI tool to inject the QR scanner Java logic.
1. Ensure `ANDROID_HOME` is set and you have `javac` and `d8` (Android SDK) in your `$PATH`.
2. Run the automated packaging task:
   ```bash
   task package-android
   ```
   *Note: This task automatically handles source patching (including `vendor/` if present) and builds the necessary custom tooling (`bin/fyne-custom`).*

### Custom Dependencies & Vendoring

Engram Dev relies on optimized forks of Gnomon and TELA. These are managed via `go.mod` replacements and vendored for stability.

If you modify vendored code:
1. Make changes in the respective fork repositories.
2. Update `go.mod` to point to the new commits.
3. Run `go mod vendor` to synchronize the local vendor directory.

## CI/CD & Security

This project implements wallet-grade CI/CD with:
- Multi-platform builds (Linux, Windows, macOS, Android)
- Security scanning (Trivy, Gitleaks, GoSec, Semgrep)
- Conventional commit validation
- Fuzz testing for critical functions
- Supply chain security with SBOM generation

See [Security Audit](docs/SECURITY_AUDIT.md) for details.

## Contributing

Issues and pull requests are welcome. Please follow [Conventional Commits](https://www.conventioncommits.org/) for pull request titles.

### Adding a New Language

Engram supports community-contributed translations. To add your language:

1. Copy `i18n/template.go` to `i18n/xx.go` (replace `xx` with your language code, e.g., `de.go` for German)
2. Translate all string values while keeping the keys unchanged
3. In `i18n/lang.go`:
   - Add a constant: `const LangXX = "xx"`
   - Add to `availableLanguages`: `LangXX: "YourLanguage"`
   - Add to `LanguageOrder()`: append your code to the slice
   - Add a case in `T()`: `case LangXX: translations = stringsXX`
4. Build and test: `go build -tags migrated_fynedo`

**Translation guidelines:**
- Keep technical terms in English: TELA Web, GNOMON, EPOCH, RPC, WS, SCID, dURL, DERO
- Keep network names in English: Mainnet, Testnet, Simulator
- If a translated word is too long for a button, abbreviate naturally
- Preserve format verbs (`%s`, `%d`) and markdown formatting
