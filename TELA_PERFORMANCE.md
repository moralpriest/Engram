# TELA Performance

This document describes the TELA app discovery architecture and performance characteristics in Engram.

## Architecture Overview

TELA app discovery uses a **three-layer cache architecture** to achieve ~2s first click on both fresh installs and repeat visits.

### Three-Layer Cache

| Layer | Source | Persistence | When Used |
|-------|--------|-------------|-----------|
| **Layer 1: Embedded** | `tela_embedded.go` (compiled into binary) | Survives everything | First click when DB is empty |
| **Layer 2: Gnomon DB** | `telacandidates` bucket in BBolt/Graviton | Survives app restart | After Gnomon fastsync or backfill completes |
| **Layer 3: JSON File** | `datashards/tela_scid_cache.json` | Survives app restart, not datashards deletion | Fallback when embedded + DB are empty |

### How ~2s First Click Works

1. User clicks TELA button
2. `gnomonReadyForTela()` checks `len(embeddedTelaSCIDs) > 0` → returns `true` **immediately**
3. `GetTelaCandidates()` returns the embedded 88 SCIDs (fallback when DB bucket is empty)
4. Skip the 49K-SCID prefilter entirely
5. Batch fetch INDEX data for 88 SCIDs only
6. **Result: ~2s on fresh install, ~2s on repeat visits**

## Performance Characteristics

| Scenario | Time | Description |
|----------|------|-------------|
| First click (fresh install) | **~2s** | Embedded SCIDs skip prefilter; background backfill starts |
| Second click (same session) | **~2s** | DB candidates from backfill or embedded list still available |
| Click after restart (datashards intact) | **~2s** | Gnomon DB `telacandidates` bucket has persisted candidates |
| Click after datashards deleted | **~2s** | Embedded list in binary still available |
| Mobile first click | **~2-4s** | Smaller worker pool (4 vs 8), same embedded list |

## Embedded SCID List

The binary includes `embeddedTelaSCIDs` in `tela_embedded.go` — a hardcoded list of known TELA app SCIDs. This list is the **fastest possible cache** because it requires zero I/O.

### Why Embedded?

- **No DB needed**: Works on completely fresh installs before Gnomon even finishes syncing
- **No network needed**: Zero RPC calls to discover candidates
- **Instant**: No file reads, no decryption, no Graviton queries

### Updating the List

The list was extracted from a live prefilter run (`datashards/tela_scid_cache.json`). To regenerate:

```bash
# Run Engram, click TELA once to populate the JSON cache
cat datashards/tela_scid_cache.json | jq -r '.scids[]'

# Paste the SCIDs into tela_embedded.go
```

> **Note**: New TELA apps published after the list was compiled are discovered by the **background backfill** running on first click. They appear on the next TELA visit (same session or after restart).

## Background Backfill

`BackfillTelaCandidates()` runs in a **non-blocking goroutine** on every first click, regardless of whether the embedded list was used.

**Purpose:**
- Discover NEW TELA apps published since the embedded list was compiled
- Populate Gnomon DB `telacandidates` bucket for future sessions

**Behavior:**
- Scans all 49K indexed SCIDs via lightweight `KeysString: ["telaVersion"]` RPC calls
- Takes ~30-60 seconds on remote daemon, ~10-20s on local
- Does NOT block the UI — user already sees results from embedded list
- Results available on next click (or after restart if DB persists)

## Gnomon Sync Wait Bypass

`gnomonReadyForTela()` now returns `true` immediately when `embeddedTelaSCIDs` is non-empty. This eliminates the **3-9s Gnomon sync wait** that previously blocked TELA from opening.

**Before:** Wait for `LastIndexedHeight >= DaemonHeight` → 3-9s delay  
**After:** Skip sync wait entirely → instant TELA open

Gnomon continues syncing in the background; TELA doesn't need it to be fully caught up because the embedded list already tells us which SCIDs to query.

## Gnomon Startup Tuning

Reduced fixed `time.Sleep` delays in `vendor/github.com/civilware/Gnomon/indexer/indexer.go`:

| Sleep | Before | After | Savings |
|-------|--------|-------|---------|
| After RPC connect | `1s` | **Removed** | ~1s |
| Before getInfo loop | `1s` | **Removed** | ~1s |
| Waiting for chain height | `1s` | `200ms` | ~0.8s/iteration |

**Total startup savings:** ~2-3 seconds

## Gnomon Bug Fixes (Vendored)

### 1. Backfill `ValuesString` Bug

`BackfillTelaCandidates()` sent `KeysString: ["telaVersion"]` but checked `VariableStringKeys` for the result. The DERO daemon returns `KeysString` results in `ValuesString[0]`, not `VariableStringKeys`.

**Fix:** Check `ValuesString[0]` instead.

### 2. NoCode Fastsync Classification

During fastsync with `NoCode: true`, `AddSCIDToIndex()` skipped TELA classification entirely because no variables were fetched. The `telacandidates` bucket stayed empty after every restart.

**Fix:** Added `HasSCIDVariable()` to BBolt + Graviton backends, wired into the NoCode path to classify candidates from existing stored variables without RPC.

## Build Notes

- Use `-tags migrated_fynedo` for all builds
- Gnomon dependency: `replace github.com/civilware/Gnomon => github.com/moralpriest/Gnomon v0.0.0-20260429054005-02f3d30e2477`
- After modifying Gnomon code in vendor: `go mod vendor` to sync (but note: vendored Gnomon changes are git-ignored; push fixes to fork first)

## New TELA App Discovery

| When | How | Appears In |
|------|-----|------------|
| App startup (fastsync) | Gnomon NoCode classification scans stored variables | Next click after fastsync completes |
| First TELA click | Background backfill scans all 49K SCIDs | Next click (same session or restart) |
| Embedded list update | Recompile with new `tela_embedded.go` | Immediate after app update |

> **Important**: New TELA apps require either (a) Gnomon fastsync to complete, or (b) the background backfill to finish, before they appear. The embedded list only contains apps known at compile time.

## Dead Code Explanation

The following code still exists but is **unreachable on first click** when the embedded list is present:

- `batchPrefilterTelaVersions()` — 49K-SCID prefilter (fallback if embedded list empty)
- `BackfillTelaCandidates()` goroutine inside `else` branch — backfill now runs unconditionally
- Encrypted cache (`StoreEncryptedValue` / `GetEncryptedValue` for `ValidatedSCIDs`) — JSON cache supersedes it
- `hasValidTelaJSONCache()` — shadowed by embedded list check in `gnomonReadyForTela()`

**Why keep it?** Fallback safety. If `embeddedTelaSCIDs` is ever empty or stale, the prefilter/backfill path ensures TELA still works.

## Known Limitations

- **NoCode fastsync stores minimal data**: With `NoCode: true`, only owner + install height are stored; variables and interaction heights are skipped
- **Embedded list is static**: New apps require backfill or fastsync to discover; list needs periodic manual updates

## Changelog

### 2026-04-29

- Added embedded TELA SCID list (`tela_embedded.go`) for instant first click
- Added `GetTelaCandidates()` fallback to embedded list when DB is empty
- Bypassed Gnomon sync wait when embedded list exists
- Moved background backfill to run unconditionally on first click (fixes new app discovery)
- Reduced Gnomon startup `time.Sleep` delays (~2-3s faster)
- Added JSON file cache as cross-session fallback
- Fixed backfill `ValuesString` bug in vendored Gnomon
- Added NoCode fastsync classification with `HasSCIDVariable()` in vendored Gnomon
- Added remote daemon prefilter tuning (reduced workers/batch size)
