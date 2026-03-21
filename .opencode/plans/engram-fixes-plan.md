# Engram Wallet Issues Fix Plan

## Executive Summary

Two critical issues identified in the Engram wallet:
1. **EPOCH methods missing** from CYBERDECK Advanced Settings → Methods section
2. **WebSocket settings persistence failing** across wallet restarts (only works in-session)

## Issue Analysis

### Issue 1: EPOCH Methods Completely Missing
- **Symptom**: EPOCH methods not visible in Advanced Settings → Methods list
- **Expected**: EPOCH methods should be visible at all times (worked before)
- **Root Cause**: Recent changes broke EPOCH method registration or display
- **Impact**: Users cannot configure EPOCH permissions

### Issue 2: WebSocket Settings Partial Persistence
- **Symptom**: WebSocket settings only persist when wallet stays open
- **Works**: Sign-out/sign-in scenario (in-session)
- **Fails**: Full wallet restart (settings lost)
- **Root Cause**: Storage path inconsistency or timing issue
- **Impact**: Users must reconfigure WebSocket settings on every restart

## Detailed Investigation Plan

### Phase 1: EPOCH Methods Restoration (Priority: Critical)

#### 1.1 Identify What Broke EPOCH Methods
**Files to Examine:**
- `functions.go` lines 3725-3764 (`SetDefaultPermissions()`)
- `functions.go` lines 3774-3820 (`getPermissions()`)
- `layouts.go` lines 9250-9268 (Methods UI generation)

**Key Questions:**
- Is `epoch.GetHandler()` being called correctly?
- Are EPOCH methods being added to the `defaults` map?
- Is `getPermissions()` returning EPOCH methods in the methods slice?
- Did recent commits remove EPOCH method registration?

**Expected EPOCH Methods:**
```
AttemptEPOCH, AttemptEPOCHWithAddr, CheckSignature, Echo, GetAddress, 
GetAddressEPOCH, GetBalance, GetDaemon, GetHeight, GetMaxHashesEPOCH, 
GetPrimaryUsername, GetSessionEPOCH, GetTransferbyTXID, GetTransfers, 
HandleTELALinks, HasMethod, MakelntegratedAddress, QueryKey, SignData, 
SplitlntegratedAddress, SubmitEPOCH, Subscribe, Transfer, get-transfer_by_txid, 
get_transfers, getaddress, getbalance, getheight, make_integrated_address, 
query_key, scinvoke, split_integrated_address, transfer, transfer_split
```

#### 1.2 Restore EPOCH Methods Display
**Approach Options:**
1. **Static Registration** (Recommended): Hard-code all EPOCH methods in `SetDefaultPermissions()`
2. **Dynamic Loading Fix**: Fix `epoch.GetHandler()` integration
3. **Hybrid**: Static registration with dynamic enhancement

**Implementation Steps:**
1. Verify current EPOCH method registration in `SetDefaultPermissions()`
2. Add explicit static registration for all missing EPOCH methods
3. Ensure `getPermissions()` includes all EPOCH methods in returned slice
4. Verify `layoutXSWDPermissions()` displays all methods correctly
5. Add logging to track method registration and display

### Phase 2: WebSocket Persistence Fix (Priority: Critical)

#### 2.1 Identify Storage Inconsistency
**Files to Examine:**
- `functions.go` lines 325-360 (`initWebSocketState()`)
- `functions.go` lines 3681-3704 (`setPermissions()`)
- `functions.go` lines 3774-3820 (`getPermissions()`)
- `store.go` (Storage functions)

**Key Questions:**
- Are settings saved to different paths based on wallet state?
- Do `setRemoteAccess()` and `getRemoteAccess()` use consistent storage paths?
- Is there a timing issue with wallet availability during load/save?
- Are there network-specific storage conflicts?

**Current Implementation Analysis:**
- Save: `setRemoteAccess(port, "WS")` in `OnChanged` callback (line 8304)
- Save: `setRemoteAccess(remoteAccess.WS.port, "WS")` on server start (line 83028)
- Load: `getRemoteAccess("WS")` in `initWebSocketState()` (line 8331)
- Issue: Possible storage path mismatch or timing dependency

#### 2.2 Fix Persistence Across Restarts
**Potential Solutions:**
1. **Unified Storage Path**: Ensure consistent storage location regardless of wallet state
2. **Timing Fix**: Add retry mechanism for wallet availability
3. **Multiple Save Points**: Add explicit save on wallet close/shutdown
4. **Storage Validation**: Add verification that saved settings are correctly loaded

**Implementation Steps:**
1. Examine current storage path resolution in `setRemoteAccess()`/`getRemoteAccess()`
2. Identify if wallet state affects storage path selection
3. Implement unified storage approach using `datashards/settings/` path
4. Add explicit save on wallet shutdown/close
5. Implement retry mechanism for loading settings when wallet isn't immediately available
6. Add comprehensive logging for storage operations

### Phase 3: Robustness & Prevention (Priority: Medium)

#### 3.1 Add Diagnostic Logging
**Logging Points:**
- EPOCH method registration at startup
- WebSocket settings save/load operations with paths
- Storage path resolution for debugging
- Method count verification in UI
- Wallet state timing dependencies

#### 3.2 Implement Fallback Mechanisms
**EPOCH Methods:**
- Static method registration if dynamic loading fails
- Graceful degradation if `epoch.GetHandler()` is unavailable
- Method availability indicators in UI

**WebSocket Settings:**
- Multiple save points for critical settings
- Default value fallback if loading fails
- User notification of settings reset

#### 3.3 Validation & Testing
**Test Scenarios:**
1. EPOCH methods visible immediately on startup
2. WebSocket persistence across full restart cycles
3. Different wallet states (loaded/unloaded) during settings operations
4. Network switching scenarios
5. Gnomon sync timing dependencies

## Implementation Strategy

### Recommended Approach
1. **EPOCH Methods**: Use static registration approach (most reliable, eliminates timing dependencies)
2. **WebSocket Persistence**: Unify storage paths using global `datashards/settings/` location
3. **Rollback Preparation**: Identify specific commits that introduced issues for potential cherry-pick

### Risk Assessment
- **Low Risk**: Static EPOCH method registration (adds methods, doesn't remove existing functionality)
- **Medium Risk**: WebSocket storage path changes (need to ensure backward compatibility)
- **High Risk**: Major refactoring (avoid, stick to targeted fixes)

## Success Criteria

### EPOCH Methods Fix
- [ ] All expected EPOCH methods visible in Advanced Settings → Methods
- [ ] Methods visible immediately on wallet startup
- [ ] Methods persist across wallet restarts
- [ ] Permission changes work correctly

### WebSocket Persistence Fix
- [ ] WebSocket settings persist across full wallet restarts
- [ ] Settings work correctly regardless of wallet state during save/load
- [ ] No regression in in-session persistence (sign-out/sign-in)
- [ ] Settings work across network switches

## Development Timeline

### Phase 1: Investigation (1-2 hours)
- Examine current EPOCH method registration
- Identify WebSocket storage path issues
- Add diagnostic logging

### Phase 2: Implementation (2-4 hours)
- Implement static EPOCH method registration
- Fix WebSocket storage path consistency
- Add fallback mechanisms

### Phase 3: Testing & Validation (1-2 hours)
- Test all scenarios
- Verify no regressions
- Final validation

## Rollback Plan

If fixes introduce new issues:
1. **Identify breaking commit** through git bisect
2. **Cherry-pick working implementation** from previous commits
3. **Revert to known-good state** and reapply fixes selectively
4. **Fallback to minimal fixes** (static EPOCH methods only, WebSocket persistence only)

## Dependencies

- **None**: All fixes are self-contained within existing codebase
- **Testing**: Need wallet with different states for comprehensive testing
- **Validation**: Need to test across restart cycles and network switches

## Complete Implementation Ready

### Investigation Results - COMPLETED

#### EPOCH Methods Issue Root Cause
- `epoch.GetHandler()` in `SetDefaultPermissions()` (functions.go:3732) may return nil/empty
- Missing comprehensive fallback registration for all expected EPOCH methods
- Some EPOCH methods are hardcoded but many from the expected list are missing

#### WebSocket Persistence Issue Root Cause  
- `setRemoteAccess()`/`getRemoteAccess()` use `StoreEncryptedValue()`/`GetEncryptedValue()` requiring `engram.Disk != nil`
- `initWebSocketState()` only loads settings if wallet available (line 3729)
- Settings save when wallet open but can't load when wallet closed on restart
- Recent commit unified storage paths but didn't address wallet dependency

## Implementation Code Changes Ready

### Fix 1: Comprehensive EPOCH Methods Registration
**File:** `functions.go` lines 3725-3734

**Replace existing code with:**
```go
	// EPOCH methods - Comprehensive registration with fallback
	logger.Printf("[Engram] EPOCH: Starting comprehensive method registration")
	
	// Register all known EPOCH methods statically as fallback
	epochMethods := []string{
		"AttemptEPOCH", "AttemptEPOCHWithAddr", "CheckSignature", "Echo", "GetAddress", 
		"GetAddressEPOCH", "GetBalance", "GetDaemon", "GetHeight", "GetMaxHashesEPOCH", 
		"GetPrimaryUsername", "GetSessionEPOCH", "GetTransferbyTXID", "GetTransfers", 
		"HandleTELALinks", "HasMethod", "MakeIntegratedAddress", "QueryKey", "SignData", 
		"SplitIntegratedAddress", "SubmitEPOCH", "Subscribe", "Transfer", "Unsubscribe",
		"get-transfer_by_txid", "get_transfers", "getaddress", "getbalance", "getheight", 
		"make_integrated_address", "query_key", "scinvoke", "split_integrated_address", 
		"transfer_split",
	}
	
	// First, try dynamic registration from epoch.GetHandler()
	dynamicMethods := make(map[string]bool)
	if epochHandler := epoch.GetHandler(); epochHandler != nil {
		logger.Printf("[Engram] EPOCH: epoch.GetHandler() returned handler with %d methods", len(epochHandler))
		for method := range epochHandler {
			defaults[method] = xswd.Ask
			dynamicMethods[method] = true
			logger.Printf("[Engram] EPOCH: Dynamically registered method: %s", method)
		}
	} else {
		logger.Printf("[Engram] EPOCH: epoch.GetHandler() returned nil - no dynamic methods available")
	}
	
	// Then, ensure all expected EPOCH methods are registered (fallback)
	registeredCount := 0
	for _, method := range epochMethods {
		if !dynamicMethods[method] {
			defaults[method] = xswd.Ask
			logger.Printf("[Engram] EPOCH: Statically registered fallback method: %s", method)
			registeredCount++
		}
	}
	
	logger.Printf("[Engram] EPOCH: Registration complete. %d static fallback methods registered, %d total EPOCH methods available", 
		registeredCount, len(epochMethods))
```

### Fix 2: WebSocket Settings Persistence - Dual Storage Approach
**File:** `functions.go` - Add new functions after line 723

**Add new functions:**
```go
// Set RemoteAccess endpoint setting with dual storage (encrypted + unencrypted fallback)
func setRemoteAccessDual(port, key string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		logger.Debugf("[Engram] setRemoteAccessDual: invalid key\n")
		return
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err := StoreEncryptedValue("RemoteAccess", []byte(key), []byte(port))
		if err != nil {
			logger.Debugf("[Engram] setRemoteAccessDual encrypted storage failed: %s\n", err)
		} else {
			logger.Printf("[Engram] setRemoteAccessDual: Successfully saved %s to encrypted storage", key)
		}
	}
	
	// Always save to unencrypted storage as fallback
	err := StoreValue("RemoteAccessUnencrypted", []byte(key), []byte(port))
	if err != nil {
		logger.Debugf("[Engram] setRemoteAccessDual unencrypted storage failed: %s\n", err)
	} else {
		logger.Printf("[Engram] setRemoteAccessDual: Successfully saved %s to fallback storage", key)
	}
}

// Get RemoteAccess endpoint setting with dual storage (try encrypted first, then fallback)
func getRemoteAccessDual(key string) (r string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		return
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		stored, err := GetEncryptedValue("RemoteAccess", []byte(key))
		if err == nil && stored != nil {
			logger.Printf("[Engram] getRemoteAccessDual: Successfully loaded %s from encrypted storage", key)
			return string(stored)
		} else if err != nil {
			logger.Debugf("[Engram] getRemoteAccessDual encrypted storage failed: %s\n", err)
		}
	}
	
	// Fallback to unencrypted storage
	stored, err := GetValue("RemoteAccessUnencrypted", []byte(key))
	if err == nil && stored != nil {
		logger.Printf("[Engram] getRemoteAccessDual: Successfully loaded %s from fallback storage", key)
		return string(stored)
	} else if err != nil {
		logger.Debugf("[Engram] getRemoteAccessDual fallback storage failed: %s\n", err)
	}
	
	logger.Printf("[Engram] getRemoteAccessDual: No stored value found for %s", key)
	return ""
}
```

### Fix 3: Update WebSocket Initialization to Use Dual Storage
**File:** `functions.go` line 8304 - Replace the OnChanged callback

**Replace existing `OnChanged` function:**
```go
remoteAccess.WS.portText.OnChanged = func(port string) {
	if remoteAccess.WS.portText.Validate() == nil {
		remoteAccess.WS.port = port
		setRemoteAccessDual(port, "WS") // Use dual storage instead of setRemoteAccess()
		
		// CRITICAL FIX: Save WebSocket enabled state to storage
		remoteAccess.WS.global.enabled = true
		setPermissions()
	}
}
```

**File:** `functions.go` line 83028 - Replace server start save

**Replace server start save code:**
```go
if remoteAccess.WS.portText != nil && remoteAccess.WS.portText.Validate() == nil {
	// Use dual storage for consistent persistence
	setRemoteAccessDual(remoteAccess.WS.portText.Text, "WS")
}
```

### Fix 4: Update WebSocket Initialization to Load Without Wallet Dependency
**File:** `functions.go` line 8331 - Replace initWebSocketState()

**Replace the entire `initWebSocketState()` function:**
```go
// Initialize WebSocket state from dual storage (works with or without wallet)
func initWebSocketState() {
	logger.Printf("[Engram] initWebSocketState() called - wallet available: %v", engram.Disk != nil)
	
	// Load stored WebSocket port using dual storage (works without wallet)
	if wsPort := getRemoteAccessDual("WS"); wsPort != "" {
		remoteAccess.WS.port = wsPort
		if remoteAccess.WS.portText != nil {
			remoteAccess.WS.portText.SetText(wsPort)
		}
		logger.Printf("[Engram] WebSocket port loaded from dual storage: %s", wsPort)
	} else {
		logger.Printf("[Engram] No WebSocket port found in storage")
	}

	// Load global permissions (includes WebSocket enabled state)
	getPermissions()
```

### Fix 5: Update Server Start to Use Dual Storage  
**File:** `functions.go` line 3915 - No changes needed here since it uses epoch.GetHandler()

The existing epoch.GetHandler() call at line 3915 should now work correctly since all methods are properly registered.

## Implementation Steps

1. **Apply Fix 1**: Replace EPOCH methods registration in `SetDefaultPermissions()`
2. **Apply Fix 2**: Add dual storage functions after `setRemoteAccess()` 
3. **Apply Fix 3**: Update OnChanged callback and server start to use `setRemoteAccessDual()`
4. **Apply Fix 4**: Replace `initWebSocketState()` to use `getRemoteAccessDual()` and remove wallet dependency
5. **Test**: Restart wallet and verify both issues are resolved

## Expected Results After Fixes

### EPOCH Methods Fix
- ✅ All 34 expected EPOCH methods visible in Advanced Settings → Methods
- ✅ Methods visible immediately on wallet startup  
- ✅ Fallback registration ensures no methods are missing even if epoch.GetHandler() fails

### WebSocket Persistence Fix  
- ✅ WebSocket settings persist across full wallet restarts
- ✅ Settings work correctly regardless of wallet state during save/load
- ✅ Dual storage ensures backward compatibility and robustness
- ✅ No regression in in-session persistence (sign-out/sign-in)

## Next Steps

1. **Apply code changes** using the exact implementations above
2. **Test the fixes** by restarting wallet and verifying both issues are resolved
3. **Monitor logs** to confirm EPOCH registration and WebSocket storage operations work correctly

---

*Created: 2026-02-11*
*Status: Planning*
*Priority: Critical*