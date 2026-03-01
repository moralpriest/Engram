## Highlights

- Includes the large UI/UX integration from MemeTactician via PR #5.
- TELA cold-start scan performance improved significantly (from ~72s baseline down to ~25s in the optimized path).
- TELA now auto-switches to Search and starts scan immediately on page open.
- Dashboard status indicators now render as true circular dots.

## Major Merge: MemeTactician UI/UX Integration (PR #5)

- PR: #5 - `feat(ui): comprehensive UI overhaul, mobile optimizations, and TELA enhancements`
- Merge commit: `61abd7f`
- Integrated commit snapshot: `2adde2b` (squashed/consolidated history)
- Scope: `32` files changed, `+12,958 / -7,988`

### What this merge delivered

- **TELA Browser**
  - Restored header visibility
  - Wider controls for Start Port Range and Search Min Likes %
  - Favorites and scan resiliency improvements, including force-rescan tooling

- **Settings and Configuration**
  - Centralized TELA + Cyberdeck settings into one settings experience
  - Added advanced settings support
  - Improved settings persistence behavior

- **Navigation and Modules**
  - Replaced multiple hyperlink navigations with icon-based button navigation
  - Standardized back button behavior/sizing
  - Improved module access patterns and dashboard navigation consistency

- **Mobile + Desktop UX**
  - Mobile responsiveness improvements (scaling, rounded controls, single-wallet quick login flow)
  - Desktop scaling behavior corrected via desktop-aware sizing logic
  - Reduced flicker and refresh/layout instability

- **Privacy / Wallet UX**
  - Persistent visibility toggles for balance and address
  - Wallet recovery/create success flow improvements

- **Branding / Assets**
  - Restored custom icons (connect, favorites, TELA)
  - Added/updated bundled UI resources and related theme/resource files

- **Stability**
  - Nil-pointer hardening and database stability improvements
  - Additional cleanup and consistency fixes across UI/layout flows

## TELA Performance Improvements (post-PR #5)

- Added batch RPC prefiltering for `telaVersion` detection.
- Fixed false-positive prefilter behavior caused by `NOT AVAILABLE` key responses.
- Added persistent encrypted negative cache with height-delta invalidation.
- Added delta-only scanning and parallel stored-SCID revalidation.
- Replaced per-call websocket INDEX fetch path with batched INDEX fetches over existing RPC.
- Added INDEX result caching and detailed scan phase metrics.
- Added multi-connection RPC pool and fixed websocket/jrpc2 cleanup ordering to prevent hangs.

## TELA UX Improvements (post-PR #5)

- Auto-trigger TELA search on page load.
- Removed redundant magnifying-glass search button from TELA tab controls.

## UI Fixes (post-PR #5)

- Fixed stretched GNOMON/TELA/CYBERDECK indicator pills by enforcing fixed-size circular dot containers.
- Split shared animation canvas into dedicated indicator canvases to avoid rendering artifacts.

## Notes

- No intended breaking wallet/account workflow changes in this release.
