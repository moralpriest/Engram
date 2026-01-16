<img src="ss1.png" alt="Engram Enigma" title="Powered by DERO">

# *One Wallet. All of DERO.*

The Engram smart wallet empowers users to easily and securely manage their money and assets on the DERO blockchain.

## Included Features

- [x] Privately send and receive money globally
- [x] On-chain encrypted private messaging
- [x] Dynamically interact with smart contracts
- [x] Native asset tracking
- [x] Register and transfer user-friendly addresses (usernames)
- [x] [Gnomon](https://github.com/civilware/Gnomon) integration for blockchain indexing
- [x] Encrypted Notepad
- [x] Websocket support for dApp/web3 connections
- [x] Sign files using your wallet to guarantee authenticity
- [x] Explore [TELA](https://github.com/civilware/tela) dApps and websites
- [x] Supports [EPOCH](https://github.com/civilware/epoch) crowd mining protocol

## Upcoming Features

- [ ] Multi-language support
- [ ] Mobile camera support

## Watch the Beta Release Video

[<img src="https://img.youtube.com/vi/00-gpNbkRW4/hqdefault.jpg" width="100%" alt="Engram Beta Release Video" />](https://www.youtube.com/watch?v=00-gpNbkRW4)

## Releases

We plan to deploy releases on the following platforms:

- [x] Windows
- [x] Linux
- [x] Mac OS
- [ ] iOS
- [x] Android

See [releases](https://github.com/DEROFDN/Engram/releases) for the latest builds.

## Build

### Required Processes

Please see: <https://developer.fyne.io/>

You are required to have all the dependencies for Fyne installed. Specifically (if you are on Windows), **TDM-GCC-64**.

1. Install fyne cmd tools: `go install fyne.io/fyne/v2/cmd/fyne@latest`
2. Add `~/go/bin` to your `$PATH` environment variable if not done already: `export PATH=$PATH:~/go/bin/`
3. Clone Engram repository and navigate to its directory:

```bash
git clone https://github.com/DEROFDN/Engram.git
cd Engram
go mod tidy
```

### Building for Windows

Build from within the repo directory:

```bash
fyne package -name Engram -os windows -icon Icon.png -tags migrated_fynedo
```

### Building for Android APK (Linux)

1. Install android-sdk: `sudo apt install android-sdk`
2. Download Android NDK from the [Android Developer site](https://developer.android.com/ndk/downloads/)
3. Add environment variable for `ANDROID_NDK_HOME` to point at the downloaded and extracted ndk directory
4. Build from within the repo directory:

```bash
fyne package -name Engram -os android/arm64 -appID com.engram.wallet -icon ./Icon.png -tags migrated_fynedo
```

## CI/CD

This project uses GitHub Actions with supply chain security hardening:

**Workflows:**

- **CI** - Build verification, tests, and linting on every push/PR
- **Security** - Static analysis (gosec, govulncheck, CodeQL, Semgrep, Trivy)
- **Release** - Multi-platform builds with cryptographic signing
- **Scorecard** - Weekly [OpenSSF Scorecard](https://securityscorecards.dev/) analysis

**Security Features:**

- **SHA-pinned Actions** - All GitHub Actions are pinned to commit SHAs instead of version tags to prevent supply chain attacks via tag hijacking
- **SLSA Level 3 Provenance** - Cryptographic attestations proving where and how binaries were built
- **Sigstore Cosign** - Keyless signing of all release artifacts
- **Harden-Runner** - Network egress monitoring during CI builds
- **Reproducible Builds** - Deterministic builds with `-trimpath` flag
- **SBOM** - Software Bill of Materials included with releases

**Verify Downloads:**

```bash
# Verify with Cosign
cosign verify-blob engram-*.tar.xz \
  --bundle engram-*.tar.xz.bundle \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com

# Verify with GitHub CLI
gh attestation verify engram-*.tar.xz --owner DEROFDN
```

## Contributing

Issues and pull requests are welcome, but will need to be reviewed by DERO Foundation developers.

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.
