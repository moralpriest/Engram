# Engram Installation Guide — Windows

## Prerequisites

- Windows 10 or later
- Stable internet connection
- A secure place to store your 25-word recovery seed (offline / paper backup)

---

## Steps

### 1. Download

Go to the [releases page](https://github.com/DEROFDN/Engram/releases) and download:

| Architecture | File |
|-------------|------|
| 64-bit | `engram-*-windows-amd64.zip` |

### 2. Install

Extract the zip and place `Engram.exe` in a permanent folder (e.g. `C:\Program Files\Engram` or `C:\Users\You\Apps\Engram`).

### 3. Run as Administrator

**Right-click** `Engram.exe` → **Run as administrator**. Some wallet functions (node connection, file access) may not work properly without admin rights.

If Windows SmartScreen blocks the app, click **More info** → **Run anyway**.

### 4. Create an account

- Click **New Account**
- Choose your seed language and enter a wallet name
- Set a strong password and confirm
- **Save your 25-word recovery seed** — this is the only way to recover the wallet. Never share it or store it online.
- Click **Close**

### 5. Wait for registration

Engram performs a one-time on-chain proof-of-work registration. Keep the app open and connected to a node (default works). This takes ~10–30 minutes depending on your CPU.

---

## Troubleshooting

| Issue | Fix |
|-------|-----|
| **App blocked by SmartScreen** | Click **More info** → **Run anyway** |
| **Node connection fails** | Run as Administrator. Check firewall isn't blocking the app. |
| **Camera not working** | Grant camera permission in **Settings → Privacy & Security → Camera** |
