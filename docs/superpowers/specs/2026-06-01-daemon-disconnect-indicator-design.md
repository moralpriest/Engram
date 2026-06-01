# Daemon Disconnect Visual Indicator

**Date:** 2026-06-01
**Status:** Approved

## Problem

When Engram has a daemon/node selected and that daemon becomes unreachable while the user is on the main wallet page, there is no prominent visual feedback. The existing status indicators (colored dots) are:
- Small (10×10px scaled)
- Located at the bottom of a scrollable dashboard layout (below balance, buttons, account links)
- The daemon address label text never changes to indicate disconnection

## Solution

Two complementary changes on the main wallet dashboard (`layoutDashboard`):

### 1. Warning Banner (Top of Page)

A colored banner bar at the very top of the dashboard content area, visible immediately without scrolling.

- **Normal state:** Transparent background, no text (zero visual footprint)
- **Disconnected state:** Dark red background (`rgba(180, 40, 40, 220)`) with white text: `"⚠ Disconnected from <daemon> — Reconnecting..."`
- **Offline mode:** Not shown (user chose offline deliberately)

### 2. Dynamic Daemon Label

The existing daemon address label (bottom status section) changes dynamically to reflect connection state:

- **Connected:** Truncated daemon address in gray (current behavior)
- **Disconnected:** `"⚠ Disconnected"` in `colors.Red`
- **Offline mode:** `"OFFLINE"` in gray

## Architecture

### Session Struct Additions (`functions.go`)

```go
DaemonBannerBg    *canvas.Rectangle  // banner background rect
DaemonBannerText  *canvas.Text       // banner message text
DaemonLabel       *canvas.Text       // daemon address/status label
```

### Pulse Loop Changes (`functions.go`)

- `setPulseDisconnectedStatus()` — set banner visible + label to red/disconnected
- `updatePulseStatusIndicators()` — revert banner hidden + label to address on reconnect
- All updates wrapped in `uiDo()` for thread-safe UI access

### Layout Changes (`wallet_layouts.go`)

- `layoutDashboard()` — create banner container as first child of `deroForm`
- Store `daemonLabel` reference in `session.DaemonLabel`

## Files Changed

| File | Change |
|------|--------|
| `functions.go` | Add 3 fields to Session struct; update `setPulseDisconnectedStatus` and `updatePulseStatusIndicators` |
| `wallet_layouts.go` | Add banner to `layoutDashboard()`; store daemonLabel in session |

## Edge Cases

- **Offline mode:** Banner hidden, label shows "OFFLINE" — no false alarm
- **Reconnecting:** Banner stays red until connection restores (handled by pulse loop)
- **Daemon switch:** Banner uses current `session.Daemon` value
- **Dashboard re-render:** Layout creates fresh objects; pulse loop references current session pointers
