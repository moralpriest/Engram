# Engram Installation Guide — Linux

## Prerequisites

- A Linux distribution with a desktop environment (GNOME, KDE, etc.)
- Stable internet connection
- A secure place to store your 25-word recovery seed (offline / paper backup)

---

## Steps

### 1. Download

Go to the [releases page](https://github.com/DEROFDN/Engram/releases) and download:

| Architecture | File |
|-------------|------|
| 64-bit | `engram-*-linux-amd64.tar.gz` |

### 2. Install

```bash
tar -xzf engram-*-linux-amd64.tar.gz
sudo mv Engram /usr/local/bin/
```

Or extract anywhere and run `./Engram` from that directory.

### 3. Open

Launch Engram from your terminal or application menu. If it doesn't start, ensure Fyne system dependencies are installed:

```bash
# Debian/Ubuntu
sudo apt install libgl1-mesa-dev xorg-dev

# Fedora
sudo dnf install mesa-libGL-devel libX11-devel libXcursor-devel libXrandr-devel

# Arch
sudo pacman -S libx11 libxcursor libxrandr mesa
```

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
| **App won't launch / missing libraries** | Install Fyne deps for your distro (see Step 3) |
| **No window appears** | Ensure a desktop environment is running and `$DISPLAY` is set |
| **Node connection fails** | Check firewall or try a different node in **Settings → Node Configuration** |
