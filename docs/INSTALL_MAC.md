# Engram Installation Guide — macOS

## Prerequisites

- macOS (Apple Silicon or Intel)
- Stable internet connection
- A secure place to store your 25-word recovery seed (offline / paper backup)

Check your chip: **Apple menu → About This Mac**. M-series = Apple Silicon; otherwise Intel.

---

## Steps

### 1. Download

Go to the [releases page](https://github.com/DEROFDN/Engram/releases) and download the correct build:

| Chip | File |
|------|------|
| Apple Silicon (M1–M4) | `engram-*-macos-arm64.tar.gz` |
| Intel | `engram-*-macos-amd64.tar.gz` |

### 2. Install

Double-click the `.tar.gz` to extract it, then drag **Engram.app** into **Applications**.

### 3. Open (bypass Gatekeeper)

Launch Engram from **Applications**. If macOS says the developer can't be verified:

- **Right-click** Engram.app → **Open** → **Open**, or
- Go to **System Settings → Privacy & Security** and click **Open Anyway**

Grant **camera** permission if prompted (used for QR scanning).

### 4. Create an account

- Click **New Account**
- Choose your seed language and enter a wallet name
- Set a strong password and confirm
- **Save your 25-word recovery seed** — this is the only way to recover the wallet if your Mac is lost. Never share it or store it online.
- Click **Close**

### 5. Wait for registration

Engram performs a one-time on-chain proof-of-work registration. Keep the app open and connected to a node (default works). This takes ~10–30 minutes depending on your CPU.

---

## Troubleshooting

| Issue | Fix |
|-------|-----|
| **"Write error" when creating account** | App is running from Downloads, OneDrive, or a restricted folder. Move Engram.app to **/Applications** and retry. |
| **App won't open / "damaged" warning** | Right-click → **Open** (not double-click) on first launch, or allow in **System Settings → Privacy & Security**. |
| **Camera not working** | Grant camera permission in **System Settings → Privacy & Security → Camera**. |
