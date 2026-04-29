# TELA Performance

This document describes the current TELA app discovery architecture and performance characteristics in Engram.

## Architecture Overview

TELA app discovery uses a **hybrid approach**: a fast prefilter for immediate results on the first visit, plus a background backfill that populates Gnomon's candidate cache for instant future visits.

### Phase 1: Fast Path (Cached Candidates)

When the user opens the TELA browser, Engram queries Gnomon for pre-computed TELA candidates via `GetTelaCandidates()`. This returns a small subset of smart contracts (~60-100) that are known to be TELA apps, bypassing the need to scan all 49,000+ indexed SCIDs.

**Flow:**
1. Call `GetTelaCandidates()` on local Gnomon instance
2. If candidates exist, skip the 49K prefilter entirely
3. Proceed directly to batch fetching INDEX data (4 parallel RPC workers)
4. Return results in ~2 seconds

### Phase 2: Hybrid First Visit (Fast Prefilter + Background Backfill)

When `GetTelaCandidates()` returns empty (fresh Gnomon database), Engram runs **two things in parallel**:

1. **Immediate fast prefilter** — gives the user results within ~5-10 seconds
2. **Background backfill** — scans all 49K SCIDs to populate the candidate cache for next time

**Fast Prefilter:**
- Creates a dedicated RPC pool (8 connections desktop, 4 mobile)
- Runs `batchPrefilterTelaVersions()` across all 49K SCIDs
- Progress bar updates in real time (15% → 60%)
- Returns results immediately for this visit

**Background Backfill:**
- Runs `BackfillTelaCandidates(8)` in a goroutine (4 on mobile)
- Uses its own dedicated RPC pool via `DialRPCPool()` (no conflict with indexing)
- Scans all 49K SCIDs with lightweight `KeysString: ["telaVersion"]` calls
- Stores found candidates in Gnomon's `telacandidates` bucket
- Takes ~30-60 seconds but doesn't block the UI

**Result:** User gets apps in ~5-10s on first visit, then ~2s on every subsequent visit.

## Performance Characteristics

| Scenario | Time | Description |
|----------|------|-------------|
| First visit (candidates ready) | ~2 seconds | Fast path: skip prefilter, parallel INDEX fetch |
| First visit (fresh database) | ~5-10 seconds | Fast prefilter returns results; background backfill runs in parallel |
| Second+ visit | ~2 seconds | Cached candidates from backfill; skip prefilter entirely |
| After Gnomon restart | ~2 seconds | Candidates persist in Gnomon DB across restarts |

## Optimizations Applied

### 1. Dedicated RPC Pool for Backfill

The previous backfill shared Gnomon's main RPC connection, causing every batch to fail with "use of closed network connection" due to conflicts with the indexing loop. Fixed by adding `DialRPCPool()` which creates fresh WebSocket connections exclusively for the backfill workers.

### 2. Parallel Backfill Workers

`BackfillTelaCandidates()` now accepts a `workers` parameter and launches parallel goroutines, each with its own RPC connection. A single collector goroutine stores candidates to avoid Graviton race conditions.

### 3. Inline TELA Classification

During `AddSCIDToIndex()`, Gnomon checks `telaVersion` metadata inline and stores candidates in a dedicated `telacandidates` bucket. This happens during normal sync, not as a post-processing step.

### 4. Parallel INDEX Fetch

`batchFetchINDEXes()` uses 4 parallel RPC workers instead of 1 sequential connection. Each worker fetches INDEX data for a batch of SCIDs concurrently, then aggregates results.

### 5. Likes/Dislikes Caching

`batchFetchINDEXes()` pre-fetches likes/dislikes ratings during the INDEX fetch phase. `getLikesRatioCached()` uses these cached values instead of querying the Graviton database per-app, eliminating ~60 DB lookups per TELA visit.

### 6. Batch Storage Fix

Fixed a Graviton snapshot mismatch bug where `StoreOwner()` and `StoreInstallHeight()` each loaded a new snapshot per call. When processing 500 SCIDs in a loop, only the last snapshot was committed. Replaced with `BatchStoreOwners()` and `BatchStoreInstallHeights()` that use a single snapshot for the entire batch.

### 7. Graviton Commit Guard

Fixed a bug where 5 Store methods called `StoreSCIDChange()` even when `nocommit=true`, causing snapshot mismatches on `CommitTrees()`. Added `if !nocommit` guards.

## Build Notes

- Gnomon dependency points to remote fork: `replace github.com/civilware/Gnomon => github.com/moralpriest/Gnomon v0.0.0-20260429054005-02f3d30e2477`
- Use `-tags migrated_fynedo` for all builds
- Run `go mod vendor` to sync vendor directory after dependency changes

## Known Limitations

- **No persistent candidate cache across sessions**: Candidates are stored in Gnomon's tempDB (in-memory), so they are lost when Gnomon restarts
- **NoCode fastsync stores minimal data**: With `NoCode: true`, only owner + install height are stored; variables and interaction heights are skipped

## Removed/Dead Code

The following were removed during optimization and are no longer needed:

- `indexer/rpc_pool.go` — Replaced with direct `indexer.RPC.RPC.Batch()` calls
- `startTelaPrewarm()` — Superseded by inline classification
- `BatchGetSCData()` / `BatchGetSCDataKeys()` — Unused helpers
- `IsFullyQueryable()` — Too strict; blocked on `InteractionIndexReady`
- Cache migration to Gnomon DB — Reverted; kept encrypted storage
- Dual-path prefilter complexity — Reverted to simple fast-path/fallback model
- Broken backfill goroutine in `startGnomon()` — Replaced with on-demand backfill triggered from TELA tab
