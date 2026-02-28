# Engram Wallet Settings Consolidation Plan

## Overview
This plan outlines the phased implementation of a centralized settings page accessible via a cogwheel icon from all screens. The goal is to consolidate TELA and Cyberdeck settings that are currently scattered across different module tabs into a single, unified settings interface with tabbed navigation.

**Important Note:** Connection settings will remain in the login page (`layoutSettings()`) and are NOT being migrated as part of this plan. Only Cyberdeck, TELA, and Advanced settings will be consolidated into the new centralized settings page.

## Current State Analysis

### Existing Settings Locations:

1. **Connection Settings (layoutSettings)** - `/home/matdero/Projects/Engram/layouts.go:5530`
   - Network tabs (Mainnet/Testnet/Simulator)
   - Node management (add/remove/test nodes)
   - ~~Gnomon toggle~~ (WILL BE MOVED to Advanced tab)
   - ~~Clear Local Data~~ (WILL BE MOVED to Advanced tab)
   - ~~Restore Defaults~~ (WILL BE MOVED to Advanced tab)
   - Accessed from Login page via "Connection Settings" button
   - **REMAINING AS-IS - NOT PART OF THIS MIGRATION**

2. **Cyberdeck Module (layoutCyberdeck)** - `/home/matdero/Projects/Engram/layouts.go:6929`
   - RPC Server configuration (port, toggle on/off)
   - WebSocket (XSWD) configuration (port, toggle on/off)
   - Connection list and permissions
   - Full server management UI

3. **TELA Module Settings (layoutTELA)** - `/home/matdero/Projects/Engram/layouts.go:14542-15650`
   - Restrictive Mode checkbox
   - Allow Content Updates dropdown
   - Rescan Recheck dropdown
   - Sort By dropdown
   - Port Start entry
   - Min Likes % entry
   - dURL Exclusions entry
   - Reset Default Settings button
   - Delete Search Data button
   - Accessed via dropdown: "History", "Active", "Search", **"Settings"**

### Settings Storage Architecture:
- **Graviton "settings" tree**: Network, endpoint, nodes, rpc_user, rpc_pass, gnomon, auth_mode
- **Encrypted "TELA Settings"**: Mode, Min Likes, Exclusions, Rescan Recheck, Sort By
- **Encrypted "TELA Search"**: Search history and SCIDs

---

## Target Architecture

### New Centralized Settings Page Structure:

```
┌─────────────────────────────────────────────────────────┐
│  ⚙️ Settings                                    [Close] │
├─────────────────────────────────────────────────────────┤
│  [ Cyberdeck ] [ TELA ] [ Advanced ]                   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  [Tab Content - RPC/WS Server config for Cyberdeck]    │
│                                                         │
│  - OR -                                                 │
│                                                         │
│  [Tab Content - Search/Mode settings for TELA]         │
│                                                         │
│  - OR -                                                 │
│                                                         │
│  [Tab Content - Gnomon/Maintenance for Advanced]       │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Proposed Tabs (3 Total):

1. **Cyberdeck** - RPC/WS server configuration (moved from Cyberdeck module)
   - RPC server settings (port, username, password, toggle)
   - WebSocket (XSWD) settings (port, toggle)
   - Connection status display
   - Copy credentials button

2. **TELA** - All TELA search and display settings (moved from TELA module)
   - Restrictive Mode checkbox
   - Allow Content Updates dropdown (Allow/Deny)
   - Rescan Recheck dropdown (Yes/No)
   - Sort By dropdown (Ratings, A-Z, Z-A)
   - Start Port Range entry
   - Search Min Likes % entry (0-100)
   - Search Exclusions entry
   - Reset Default Settings button
   - Delete Search Data button

3. **Advanced** - System-wide settings and maintenance options
   - **Gnomon** toggle (moved from Connection settings)
     - Enable/disable Gnomon indexer
     - Description text explaining Gnomon functionality
   - **Track Recent Blocks** entry
     - Scan only last N blocks for faster wallet sync
     - Placeholder: "# of Latest Blocks (Optional)"
   - **Clear Local Data** button (moved from Connection settings)
     - Deletes Gnomon data for current network
   - **Restore Defaults** button (moved from Connection settings)
     - Resets all settings to defaults

---

## Implementation Phases

---

### **Phase 1: Foundation & Infrastructure**
**Objective**: Create the new settings page structure with 3 tabs

#### Tasks:
1. Create `layoutAppSettings()` function in `layouts.go`
2. Set up the tab container with 3 tabs (Cyberdeck, TELA, Advanced)
3. Create placeholder content for each tab (simple labels indicating "Coming in Phase X")
4. Add settings button with cogwheel icon (`theme.SettingsIcon()`) to dashboard
   - Position at top-right or bottom of dashboard screen
   - Accessible via both tap (Android) and click (PC)
5. Create navigation function to open settings from dashboard
6. Add close/back button to return to dashboard
7. Test navigation and tab switching

#### Files to Modify:
- `layouts.go`: Add new `layoutAppSettings()` function
- `layouts.go`: Add settings cogwheel button to `layoutDashboard()`

#### Code Structure:
```go
func layoutAppSettings() fyne.CanvasObject {
    // Create tabs
    tabs := container.NewAppTabs(
        container.NewTabItem("Cyberdeck", container.NewVBox(/* placeholder */)),
        container.NewTabItem("TELA", container.NewVBox(/* placeholder */)),
        container.NewTabItem("Advanced", container.NewVBox(/* placeholder */)),
    )
    // ... rest of layout
}
```

#### Git Commit Message:
```
feat(settings): create centralized settings page infrastructure

- Add layoutAppSettings() with 3-tab interface
- Create tabs: Cyberdeck, TELA, Advanced
- Add settings cogwheel button to dashboard
- Set up navigation from dashboard with back button
- Add placeholder content for future phases

Refs: settings-consolidation-phase-1
```

#### Testing Checklist:
- [ ] Settings page opens from dashboard cogwheel button
- [ ] All 3 tabs are visible and clickable
- [ ] Tab switching works correctly
- [ ] Tab content area displays placeholder text
- [ ] Settings button icon renders properly on Android and PC
- [ ] Back/close button returns to dashboard
- [ ] Settings page layout is responsive

---

### **Phase 2: Cyberdeck Tab Migration**
**Objective**: Extract Cyberdeck configuration UI and move to settings page

#### Tasks:
1. Extract RPC server configuration UI from `layoutCyberdeck()`:
   - Port entry field with validation
   - Username entry field
   - Password entry field
   - Toggle button (Turn On/Off)
   - Status text (Blocked/Allowed)
   - Copy credentials hyperlink

2. Extract WebSocket (XSWD) configuration UI from `layoutCyberdeck()`:
   - Port entry field with validation
   - Toggle button (Turn On/Off)
   - Status text (Blocked/Allowed)

3. Create Cyberdeck tab content in `layoutAppSettings()`:
   - Section header for RPC Configuration
   - RPC port, user, pass fields
   - RPC toggle button
   - Section header for XSWD Configuration
   - XSWD port field
   - XSWD toggle button
   - Status indicators for both

4. Ensure settings sync with Cyberdeck module:
   - Use same `cyberdeck.RPC.*` and `cyberdeck.WS.*` variables
   - Settings changes should reflect in Cyberdeck module immediately
   - Cyberdeck module status changes should reflect in settings

5. Keep existing server management in Cyberdeck module:
   - Connection list remains in `layoutCyberdeck()`
   - Permissions management remains in Cyberdeck module
   - Settings page only handles configuration, not connections

#### Files to Modify:
- `layouts.go`: Extract RPC/WS configuration logic from `layoutCyberdeck()`
- `layouts.go`: Add Cyberdeck tab content to `layoutAppSettings()`

#### Code Considerations:
- Use shared state: `cyberdeck.RPC.port`, `cyberdeck.RPC.user`, `cyberdeck.RPC.pass`
- Use shared state: `cyberdeck.WS.port`
- Toggle functions should call existing `toggleRPCServer()` and `toggleXSWD()`
- Validation regex for ports: `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`

#### Git Commit Message:
```
feat(settings): migrate Cyberdeck configuration to settings page

- Extract RPC server configuration to Cyberdeck tab
- Extract WebSocket (XSWD) configuration to Cyberdeck tab
- Add port validation and toggle functionality
- Maintain synchronization with Cyberdeck module state
- Preserve server toggle functionality

Refs: settings-consolidation-phase-2
```

#### Testing Checklist:
- [ ] RPC port configuration displays current value
- [ ] RPC port validation works (validates hostname:port format)
- [ ] RPC username field displays current value
- [ ] RPC password field displays current value (masked)
- [ ] RPC toggle button works (Turn On/Off)
- [ ] WS port configuration displays current value
- [ ] WS port validation works
- [ ] WS toggle button works
- [ ] Settings sync with Cyberdeck module status
- [ ] Toggle states persist after app restart
- [ ] Copy credentials button works
- [ ] Server status reflects actual state (Blocked/Allowed)

---

### **Phase 3: TELA Tab Migration**
**Objective**: Move TELA settings from module dropdown to centralized settings

#### Tasks:
1. Extract TELA settings UI from `layoutTELA()` (lines ~15350-15650):
   - Restrictive Mode checkbox
   - Allow Content Updates dropdown
   - Rescan Recheck dropdown
   - Sort By dropdown
   - Port Start entry
   - Min Likes % entry
   - dURL Exclusions entry
   - Reset Default Settings button
   - Delete Search Data button

2. Create TELA tab content in `layoutAppSettings()`:
   - Organize settings with proper labels and spacing
   - Use `widget.NewCheck()` for Restrictive Mode
   - Use `widget.NewSelect()` for dropdowns
   - Use `widget.NewEntry()` for text fields
   - Add validation for Min Likes % (0-100)
   - Add validation for Port Start (numeric)

3. Update `layoutTELA()` to remove Settings dropdown option:
   - Change dropdown from `["History", "Active", "Search", "Settings"]`
   - To: `["History", "Active", "Search"]`
   - Remove Settings-related UI code

4. Ensure encrypted storage still works correctly:
   - Use existing keys: "Mode", "Min Likes", "Exclusions", "Rescan Recheck", "Sort By"
   - Use `GetEncryptedValue()` and `StoreEncryptedValue()` functions
   - Maintain compatibility with existing TELA functionality

#### Files to Modify:
- `layouts.go`: Extract TELA settings from `layoutTELA()`
- `layouts.go`: Add TELA tab content to `layoutAppSettings()`
- `layouts.go`: Remove "Settings" option from TELA dropdown in `layoutTELA()`

#### Code Considerations:
- Restrictive Mode: Stored as "Restrictive" (default) or "Unrestrictive"
- Allow Updates: Use `xswd.Allow.String()` and `xswd.Deny.String()`
- Rescan Recheck: "Yes" or "No"
- Sort By: "Ratings", "A-Z", "Z-A"
- Port Start: Integer, default `tela.DEFAULT_PORT_START` (54320)
- Min Likes: Float 0-100, default 30
- Exclusions: Comma-separated string

#### Git Commit Message:
```
feat(settings): migrate TELA settings to centralized page

- Move all TELA configuration to TELA tab
- Add Restrictive Mode, Content Updates, Rescan Recheck controls
- Add Sort By, Port Start, Min Likes, Exclusions fields
- Remove Settings dropdown from TELA module
- Preserve encrypted storage for TELA settings
- Maintain search functionality in TELA module

Refs: settings-consolidation-phase-3
```

#### Testing Checklist:
- [ ] Restrictive Mode toggle loads saved value
- [ ] Restrictive Mode toggle saves correctly
- [ ] Allow Content Updates dropdown loads saved value
- [ ] Allow Content Updates setting saves correctly
- [ ] Rescan Recheck dropdown loads saved value
- [ ] Rescan Recheck setting saves correctly
- [ ] Sort By dropdown loads saved value
- [ ] Sort By preference saves correctly
- [ ] Port Start field loads saved value
- [ ] Port Start validates as numeric
- [ ] Min Likes field loads saved value
- [ ] Min Likes validates (0-100 range)
- [ ] Exclusions field loads saved value
- [ ] Exclusions save correctly
- [ ] Reset Defaults button works
- [ ] Delete Search Data button works
- [ ] TELA dropdown no longer shows "Settings" option
- [ ] TELA search still functions normally
- [ ] Settings persist after app restart

---

### **Phase 4: Advanced Tab & Final Cleanup**
**Objective**: Create Advanced tab with Gnomon and maintenance options

#### Tasks:
1. Create Advanced tab content in `layoutAppSettings()`:
   - **GNOMON Section:**
     - Header: "GNOMON"
     - Description text: "Gnomon scans and indexes blockchain data in order to unlock more features, like native asset tracking."
     - Checkbox: "Enable Gnomon"
   
   - **SCANNING Section:**
     - Header: "SCANNING"
     - Description: "Enter the number of past blocks that the wallet should scan:"
     - Entry field: "# of Latest Blocks (Optional)"
     - Validator: Must be a valid integer
   
   - **MAINTENANCE Section:**
     - Header: "MAINTENANCE"
     - Button: "Clear Local Data"
       - Shows confirmation dialog
       - Clears Gnomon data for current network
     - Button: "Restore Defaults"
       - Shows confirmation dialog
       - Resets all settings to defaults

2. Remove Gnomon and maintenance options from `layoutSettings()` (Connection settings):
   - Remove Gnomon checkbox
   - Remove Gnomon description text
   - Remove Clear Local Data button
   - Remove Restore Defaults button
   - Remove Track Recent Blocks entry
   - Keep only network tabs and node management in Connection settings

3. Final cleanup:
   - Update any remaining navigation references
   - Remove unused imports if any
   - Verify all settings still persist correctly
   - Check UI consistency across all tabs
   - Ensure no duplicate code remains

#### Files to Modify:
- `layouts.go`: Add Advanced tab content to `layoutAppSettings()`
- `layouts.go`: Remove Gnomon/maintenance from `layoutSettings()` (optional - can keep for now)

#### Code Considerations:
- Gnomon storage: `gnomon` key in settings tree ("1" or "0")
- Track Recent Blocks: `session.TrackRecentBlocks` (int64)
- Clear Local Data: Calls `cleanGnomonData()` function
- Restore Defaults: Resets network, daemon, auth_mode, gnomon, nodes, RPC credentials

#### Git Commit Message:
```
feat(settings): add advanced settings tab and finalize migration

- Create Advanced tab with Gnomon toggle
- Add Track Recent Blocks scanning option
- Add Clear Local Data and Restore Defaults buttons
- Remove maintenance options from Connection settings
- Final UI consistency improvements
- Clean up deprecated code

Refs: settings-consolidation-phase-4
Closes: settings-consolidation
```

#### Testing Checklist:
- [ ] Gnomon checkbox loads saved value
- [ ] Gnomon toggle enables/disables correctly
- [ ] Gnomon setting persists after restart
- [ ] Track Recent Blocks field loads saved value
- [ ] Track Recent Blocks validates as integer
- [ ] Clear Local Data shows confirmation dialog
- [ ] Clear Local Data works for each network
- [ ] Restore Defaults shows confirmation dialog
- [ ] Restore Defaults resets all settings correctly
- [ ] Connection settings still work (login page)
- [ ] No broken navigation links
- [ ] Settings accessible from dashboard via cogwheel
- [ ] No duplicate settings UI remains
- [ ] All 3 tabs function correctly
- [ ] Settings persist across app restarts

---

## Cross-Cutting Concerns

### Settings Persistence
All settings must continue to use the existing storage mechanisms:
- Graviton settings tree for general settings (network, endpoint, nodes, rpc_user, rpc_pass, gnomon, auth_mode)
- Encrypted stores for sensitive TELA settings (Mode, Min Likes, Exclusions, Rescan Recheck, Sort By)
- Session variables for runtime state (session.TrackRecentBlocks)
- No migration needed - use existing keys

### Mobile/PC Compatibility
- Use Fyne's responsive design
- Ensure touch targets are adequate for Android (minimum 44x44dp)
- Test both mouse click and touch interactions
- Verify settings button placement works on different screen sizes
- Use `fyne.Do()` for UI updates from goroutines

### Accessibility
- Maintain keyboard navigation support
- Ensure proper focus management
- Keep text labels clear and descriptive
- Use appropriate contrast ratios for colors

### Backward Compatibility
- Existing settings should load correctly
- No user data loss during migration
- Connection settings remain accessible from login page
- All existing functionality preserved

---

## Git Workflow

### Branch Strategy:
```bash
# Create feature branch from main
git checkout -b feature/settings-consolidation

# After each phase:
git add <relevant-files>
git commit -m "feat(settings): [phase description]"

# At project completion:
git push origin feature/settings-consolidation
# Create Pull Request for review
# After approval, merge to main
```

### Commit Message Format:
- Use conventional commits: `feat(settings): description`
- Reference phase number in commit body: "Refs: settings-consolidation-phase-N"
- Keep commits focused on single phase
- Do NOT include plan document in commits

---

## Risk Assessment & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|---------|------------|
| Settings data loss | Low | High | Don't change storage keys; use existing Get/Store functions; test thoroughly |
| Cyberdeck server state sync issues | Medium | High | Keep shared state variables (cyberdeck.RPC.*, cyberdeck.WS.*); test toggles thoroughly |
| TELA search functionality broken | Low | High | Ensure settings still affect TELA module behavior; test search after migration |
| UI layout issues on mobile | Medium | Medium | Test on actual Android device; use Fyne responsive containers; verify touch targets |
| Broken navigation | Low | Medium | Comprehensive testing after each phase; test all navigation paths |
| Connection settings broken | Low | High | Leave Connection settings untouched; only remove from new centralized page |

---

## Testing Strategy

### Unit Testing (Per Phase):
- Test each settings tab independently
- Verify settings persistence (save/load)
- Test navigation paths
- Test validation rules

### Integration Testing:
- Test full user workflows
- Verify cross-module communication:
  - Cyberdeck settings → Cyberdeck module state
  - TELA settings → TELA search behavior
  - Gnomon toggle → Gnomon indexer
- Test settings across app restarts
- Test on both PC and Android

### Platform Testing:
- PC (Linux/Windows/Mac) - mouse interactions
- Android device - touch interactions
- Different screen sizes and orientations

---

## Success Criteria

1. ✅ Single settings page accessible via cogwheel from dashboard
2. ✅ 3 functional tabs: Cyberdeck, TELA, Advanced
3. ✅ Connection settings remain accessible from login page
4. ✅ All Cyberdeck settings migrated without data loss
5. ✅ All TELA settings migrated without data loss
6. ✅ Gnomon and maintenance options moved to Advanced tab
7. ✅ No duplicate settings UI remaining
8. ✅ Mobile and PC compatible
9. ✅ No regression in Cyberdeck or TELA functionality
10. ✅ Clean git history with 4 commits (one per phase)

---

## Notes & Future Considerations

- Connection settings may be migrated to centralized page in future iteration (Phase 5+), but this is out of scope for now
- Consider adding a "What's New" notification for users after update to inform them of settings location change
- Document any breaking changes for users accustomed to old settings locations
- The plan document should remain in `.opencode/plans/` and NOT be committed to git
- If issues arise during implementation, document them in the plan and adjust phases as needed

---

## Plan Document Location

**File:** `/home/matdero/Projects/Engram/.opencode/plans/settings_consolidation_plan.md`

This document is for planning purposes only and should NOT be committed to the repository.
