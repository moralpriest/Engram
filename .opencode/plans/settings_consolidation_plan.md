# Engram Wallet Settings Consolidation Plan

## Overview
This plan outlines the phased implementation of a centralized settings page accessible via a cogwheel icon from all screens. The goal is to consolidate TELA and Cyberdeck settings that are currently scattered across different module tabs into a single, unified settings interface with tabbed navigation.

## Current State Analysis

### Existing Settings Locations:

1. **Connection Settings (layoutSettings)** - `/home/matdero/Projects/Engram/layouts.go:5530`
   - Network tabs (Mainnet/Testnet/Simulator)
   - Node management (add/remove/test nodes)
   - Basic Cyberdeck RPC credentials (username/password)
   - Gnomon toggle
   - Clear Local Data / Restore Defaults buttons
   - Accessed from Login page via "Connection Settings" button

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
- **Graviton "settings" tree**: Network, endpoint, nodes, rpc_user, rpc_pass, gnomon
- **Encrypted "TELA Settings"**: Mode, Min Likes, Exclusions, Rescan Recheck, Sort By
- **Encrypted "TELA Search"**: Search history and SCIDs

---

## Target Architecture

### New Centralized Settings Page Structure:

```
┌─────────────────────────────────────────────────────────┐
│  ⚙️ Settings                                    [Close] │
├─────────────────────────────────────────────────────────┤
│  [ Connection ] [ Cyberdeck ] [ TELA ] [ Advanced ]     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  [Tab Content - Network/Nodes/Gnomon for Connection]   │
│                                                         │
│  - OR -                                                 │
│                                                         │
│  [Tab Content - RPC/WS Server toggles for Cyberdeck]   │
│                                                         │
│  - OR -                                                 │
│                                                         │
│  [Tab Content - Search/Mode settings for TELA]         │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Proposed Tabs:
1. **Connection** - Network selection, node management, Gnomon toggle
2. **Cyberdeck** - RPC/WS server configuration (moved from Cyberdeck module)
3. **TELA** - All TELA search and display settings (moved from TELA module)
4. **Advanced** - Clear data, restore defaults, scan options

---

## Implementation Phases

---

### **Phase 1: Foundation & Planning**
**Objective**: Create the new settings page structure and infrastructure

#### Tasks:
1. Create `layoutAppSettings()` function in `layouts.go`
2. Set up the tab container with 4 tabs (Connection, Cyberdeck, TELA, Advanced)
3. Create placeholder content for each tab
4. Add settings button with cogwheel icon to dashboard
5. Test navigation and tab switching

#### Files to Modify:
- `layouts.go`: Add new `layoutAppSettings()` function
- `layouts.go`: Add settings button to `layoutDashboard()`
- `layouts.go`: Update `layoutMain()` to reference new settings

#### Git Commit Message:
```
feat(settings): create centralized settings page infrastructure

- Add layoutAppSettings() with tabbed interface
- Create 4 settings tabs: Connection, Cyberdeck, TELA, Advanced
- Add settings cogwheel button to dashboard
- Set up navigation from login page and dashboard

Refs: settings-consolidation-phase-1
```

#### Testing Checklist:
- [ ] Settings page opens from dashboard
- [ ] Settings page opens from login page
- [ ] All 4 tabs are visible and clickable
- [ ] Tab content area displays correctly
- [ ] Settings button icon renders properly on Android and PC
- [ ] Back/close button returns to previous screen

---

### **Phase 2: Connection Tab Migration**
**Objective**: Migrate existing connection settings to the new settings page

#### Tasks:
1. Refactor existing `layoutSettings()` content into Connection tab
2. Move network selection tabs (Mainnet/Testnet/Simulator) to Connection tab
3. Move node management UI to Connection tab
4. Move Gnomon toggle to Connection tab
5. Keep the old `layoutSettings()` for backward compatibility during transition

#### Files to Modify:
- `layouts.go`: Migrate connection settings logic to `layoutAppSettings()`
- `layouts.go`: Update Connection tab implementation

#### Git Commit Message:
```
feat(settings): migrate connection settings to centralized page

- Move network selection tabs to Connection tab
- Move node management (add/remove/test) to Connection tab  
- Move Gnomon toggle to Connection tab
- Preserve all existing functionality and settings storage

Refs: settings-consolidation-phase-2
```

#### Testing Checklist:
- [ ] Network tabs switch correctly
- [ ] Nodes display for selected network
- [ ] Can add new custom nodes
- [ ] Can remove nodes
- [ ] Node connection testing works
- [ ] Gnomon toggle saves/loads correctly
- [ ] All settings persist after app restart

---

### **Phase 3: Cyberdeck Settings Migration**
**Objective**: Extract Cyberdeck configuration UI and move to settings page

#### Tasks:
1. Extract RPC server configuration UI from `layoutCyberdeck()`
2. Extract WebSocket (XSWD) configuration UI from `layoutCyberdeck()`
3. Create Cyberdeck tab content with both configurations
4. Ensure settings still sync with Cyberdeck module
5. Keep existing server management in Cyberdeck module (for connections list)

#### Files to Modify:
- `layouts.go`: Extract RPC/WS configuration from `layoutCyberdeck()`
- `layouts.go`: Add Cyberdeck tab content to `layoutAppSettings()`
- `layouts.go`: Ensure shared state between settings and Cyberdeck module

#### Git Commit Message:
```
feat(settings): migrate Cyberdeck configuration to settings page

- Extract RPC server configuration to Cyberdeck tab
- Extract WebSocket (XSWD) configuration to Cyberdeck tab
- Maintain synchronization with Cyberdeck module
- Preserve server toggle functionality

Refs: settings-consolidation-phase-3
```

#### Testing Checklist:
- [ ] RPC port configuration saves/loads
- [ ] WS port configuration saves/loads
- [ ] Toggle buttons work in settings
- [ ] Settings sync with Cyberdeck module status
- [ ] Credentials display correctly
- [ ] Server status reflects actual state

---

### **Phase 4: TELA Settings Migration**
**Objective**: Move TELA settings from module dropdown to centralized settings

#### Tasks:
1. Extract TELA settings UI from `layoutTELA()` (lines ~15350-15650)
2. Move all settings controls to TELA tab:
   - Restrictive Mode checkbox
   - Allow Content Updates dropdown
   - Rescan Recheck dropdown
   - Sort By dropdown
   - Port Start entry
   - Min Likes % entry
   - dURL Exclusions entry
   - Reset Default Settings button
   - Delete Search Data button
3. Update `layoutTELA()` to remove Settings dropdown option
4. Ensure encrypted storage still works correctly

#### Files to Modify:
- `layouts.go`: Extract TELA settings from `layoutTELA()`
- `layouts.go`: Add TELA tab content to `layoutAppSettings()`
- `layouts.go`: Remove "Settings" option from TELA dropdown

#### Git Commit Message:
```
feat(settings): migrate TELA settings to centralized page

- Move all TELA configuration to TELA tab
- Remove Settings dropdown from TELA module
- Preserve encrypted storage for TELA settings
- Maintain search functionality in TELA module

Refs: settings-consolidation-phase-4
```

#### Testing Checklist:
- [ ] Restrictive Mode toggle works
- [ ] Content Updates setting saves
- [ ] Rescan Recheck setting saves
- [ ] Sort By preference works
- [ ] Port Start validates and saves
- [ ] Min Likes validates (0-100)
- [ ] Exclusions save correctly
- [ ] Reset Defaults works
- [ ] Delete Search Data works
- [ ] TELA search still functions normally

---

### **Phase 5: Advanced Tab & Cleanup**
**Objective**: Create Advanced tab and clean up old settings implementations

#### Tasks:
1. Create Advanced tab with:
   - Clear Local Data button
   - Restore Defaults button
   - Scan recent blocks option
   - Any other advanced options
2. Remove old `layoutSettings()` function (keep for reference if needed)
3. Update all navigation references to point to new settings
4. Clean up any unused imports or variables
5. Final UI polish and consistency checks

#### Files to Modify:
- `layouts.go`: Add Advanced tab content
- `layouts.go`: Remove or deprecate old `layoutSettings()`
- `layouts.go`: Update all settings navigation

#### Git Commit Message:
```
feat(settings): add advanced settings tab and cleanup

- Create Advanced tab with maintenance options
- Remove deprecated settings code
- Update all navigation to use centralized settings
- Final UI consistency improvements

Refs: settings-consolidation-phase-5
Closes: settings-consolidation
```

#### Testing Checklist:
- [ ] Clear Local Data works for each network
- [ ] Restore Defaults resets all settings
- [ ] Scan blocks option functions
- [ ] No broken navigation links
- [ ] Settings accessible from all screens
- [ ] No duplicate settings UI remains

---

## Cross-Cutting Concerns

### Settings Persistence
All settings must continue to use the existing storage mechanisms:
- Graviton settings tree for general settings
- Encrypted stores for sensitive TELA settings
- No migration needed - use existing keys

### Mobile/PC Compatibility
- Use Fyne's responsive design
- Ensure touch targets are adequate for Android
- Test both mouse click and touch interactions
- Verify settings button placement works on different screen sizes

### Accessibility
- Maintain keyboard navigation support
- Ensure proper focus management
- Keep text labels clear and descriptive

### Backward Compatibility
- Existing settings should load correctly
- No user data loss during migration
- Consider gradual rollout if needed

---

## Git Workflow

### Branch Strategy:
```bash
# Create feature branch
git checkout -b feature/settings-consolidation

# After each phase:
git add .
git commit -m "feat(settings): [phase description]"

# At project completion:
git push origin feature/settings-consolidation
# Create PR for review
```

### Commit Message Format:
- Use conventional commits: `feat(settings): description`
- Reference phase number in commit
- Include "Refs: settings-consolidation-phase-N"

---

## Risk Assessment & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|---------|------------|
| Settings data loss | Low | High | Don't change storage keys; test thoroughly |
| UI layout issues on mobile | Medium | Medium | Test on actual Android device; use Fyne responsive containers |
| Broken navigation | Low | Medium | Comprehensive testing after each phase |
| Server state sync issues | Medium | High | Keep shared state variables; test Cyberdeck thoroughly |
| TELA search functionality broken | Low | High | Ensure settings still affect TELA module behavior |

---

## Testing Strategy

### Unit Testing:
- Test each settings tab independently
- Verify settings persistence
- Test navigation paths

### Integration Testing:
- Test full user workflows
- Verify cross-module communication (Cyberdeck settings → Cyberdeck module)
- Test settings across app restarts

### Platform Testing:
- PC (Linux/Windows/Mac)
- Android device
- Different screen sizes

---

## Success Criteria

1. ✅ Single settings page accessible via cogwheel from all screens
2. ✅ 4 functional tabs: Connection, Cyberdeck, TELA, Advanced
3. ✅ All existing settings migrated without data loss
4. ✅ No duplicate settings UI remaining
5. ✅ Mobile and PC compatible
6. ✅ No regression in Cyberdeck or TELA functionality
7. ✅ Clean git history with 5 commits (one per phase)

---

## Notes

- The login page "Connection Settings" button can remain as-is for now (Phase 0), or be updated to point to the new settings in Phase 2
- Consider adding a "What's New" notification for users after update
- Document any breaking changes for users accustomed to old settings locations
