# TELA Performance

This document describes the current TELA app discovery architecture and performance characteristics in Engram.

## Architecture Overview

TELA app discovery uses a two-phase approach to minimize initial sync time:

### Phase 1: Fast Path (Cached Candidates)

When the user opens the TELA browser, Engram queries Gnomon for pre-computed TELA candidates via `GetTelaCandidates()`. This returns a small subset of smart contracts (~60-100) that are known to be TELA apps, bypassing the need to scan all 49,000+ indexed SCIDs.

**Flow:**
1. Call `GetTelaCandidates()` on local Gnomon instance
2. If candidates exist, skip the 49K prefilter entirely
3. Proceed directly to batch fetching INDEX data (4 parallel RPC workers)
4. Return results in ~2 seconds

### Phase 2: Background Backfill

If candidates are not yet available (first sync after Gnomon startup), a background goroutine runs `BackfillTelaCandidates()` while the user is on the dashboard. This:

- Scans only owner and install height (lightweight, no variables)
- Uses 3 retries with exponential backoff
- Batch size of 200 SCIDs per RPC call
- Stores results in Gnomon's tempDB for future fast-path queries

**Note:** This is gated behind `!NoCode` — with NoCode fastsync, backfill is skipped because minimal data is stored.

### Fallback Behavior

If `GetTelaCandidates()` returns empty AND backfill is incomplete, Engram falls back to the legacy behavior:
- Run `batchPrefilterTelaVersions()` across all 49,000+ SCIDs
- This takes ~6-7 seconds instead of ~2 seconds
- Only occurs on first visit after Gnomon cold start

## Performance Characteristics

| Scenario | Time | Description |
|----------|------|-------------|
| First visit (candidates ready) | ~2 seconds | Fast path: skip prefilter, parallel INDEX fetch |
| First visit (backfill incomplete) | ~6-7 seconds | Fallback: 49K prefilter + sequential scan |
| Second+ visit | ~2 seconds | Cached results from previous visit |
| After Gnomon restart | ~2-6 seconds | Depends on whether backfill completed before visit |

## Optimizations Applied

### 1. Inline TELA Classification

During `AddSCIDToIndex()`, Gnomon checks `telaVersion` metadata inline and stores candidates in a dedicated `telacandidates` bucket. This happens during normal sync, not as a post-processing step.

### 2. Parallel INDEX Fetch

`batchFetchINDEXes()` uses 4 parallel RPC workers instead of 1 sequential connection. Each worker fetches INDEX data for a batch of SCIDs concurrently, then aggregates results.

### 3. Likes/Dislikes Caching

`batchFetchINDEXes()` pre-fetches likes/dislikes ratings during the INDEX fetch phase. `getLikesRatioCached()` uses these cached values instead of querying the Graviton database per-app, eliminating ~60 DB lookups per TELA visit.

### 4. Batch Storage Fix

Fixed a Graviton snapshot mismatch bug where `StoreOwner()` and `StoreInstallHeight()` each loaded a new snapshot per call. When processing 500 SCIDs in a loop, only the last snapshot was committed. Replaced with `BatchStoreOwners()` and `BatchStoreInstallHeights()` that use a single snapshot for the entire batch.

### 5. Graviton Commit Guard

Fixed a bug where 5 Store methods called `StoreSCIDChange()` even when `nocommit=true`, causing snapshot mismatches on `CommitTrees()`. Added `if !nocommit` guards.

## Build Notes

- Gnomon dependency points to local path for development: `replace github.com/civilware/Gnomon => /home/priest/Projects/Gnomon`
- After modifying Gnomon code, run `go mod vendor` to sync Engram's vendor directory
- Use `-tags migrated_fynedo` for all builds

## Known Limitations

- **Intermittent daemon overload**: First visit timing can vary between ~2s and ~6s depending on daemon load and whether the background backfill completed
- **No persistent candidate cache across sessions**: Candidates are stored in Gnomon's tempDB (in-memory), so they are lost when Gnomon restarts
- **NoCode fastsync stores minimal data**: With `NoCode: true`, only owner + install height are stored; variables and interaction heights are skipped

## Future Improvements (Not Yet Implemented)

- **Background INDEX pre-fetch**: Fetch INDEX data for candidates while user is on dashboard (would make first visit consistently <1s)
- **Persistent candidate cache in Engram**: Store `GetTelaCandidates()` results across sessions in encrypted storage
- **Hosted Gnomon endpoints**: Add `gettelacandidates` websocket/REST endpoint for remote Gnomon instances

## Removed/Dead Code

The following were removed during optimization and are no longer needed:

- `indexer/rpc_pool.go` — Replaced with direct `indexer.RPC.RPC.Batch()` calls
- `startTelaPrewarm()` — Superseded by inline classification
- `BatchGetSCData()` / `BatchGetSCDataKeys()` — Unused helpers
- `IsFullyQueryable()` — Too strict; blocked on `InteractionIndexReady`
- Cache migration to Gnomon DB — Reverted; kept encrypted storage
- Dual-path prefilter complexity — Reverted to simple fast-path/fallback model
