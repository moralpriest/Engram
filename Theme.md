# Engram Theme & Design System

> Design rules for the Engram DERO Protocol wallet — Desktop (Windows, Linux, macOS) & Mobile (Android, iOS).
>
> The current default theme is **"Engram Classic"** (green accent on dark background).
> The architecture supports additional theme variants — see [Future-Proofing](#xiv-future-proofing-for-theme-variants).

---

## I. Color System — "Engram Classic" Default

### Palette

Source: `theme.go` (custom `eTheme`), `main.go:163–176` (`colors.*` struct)

| Token | Hex | RGBA | Role |
|-------|-----|------|------|
| **DarkMatter** | `#15171E` | `(21,23,30,255)` | App background, shadows |
| **Green** | `#13CA69` | `(19,202,105,255)` | Primary accent, active state |
| **DarkGreen** | `#117F4E` | `(17,127,78,255)` | Secondary green, pressed |
| **Foreground** | `#D0D0D0` | `(208,208,208,255)` | Body text, icons |
| **Gray** | `#636363` | `(99,99,99,255)` | Labels, secondary text |
| **Flint** | `#2C2C34` | `(44,44,52,255)` | Button/card backgrounds |
| **MenuBackground** | `#1F2128` | `(31,33,40,238)` | Dropdowns, overlays |
| **Cold** | `#3C495C` | `(60,73,92,255)` | Inactive, muted elements |
| **Network** | `#43EF43` | `(67,239,67,255)` | Connection status indicators |
| **Red** | `#F44336` | `(244,67,54,255)` | Errors, disconnected state |
| **SoftRed** | `#F06E6E` | `(240,110,110,255)` | Soft error, caution |
| **Yellow** | `#F4D00B` | `(244,208,11,255)` | Warnings, attention |
| **LightBlue** | `#38B6FF` | `(56,182,255,255)` | Info, marquee text |
| **Blue** | `#1BFD7F` | `(27,249,127,255)` | Links, secondary accent |
| **Purple** | `#BF40BF` | `(191,64,191,255)` | Special status |
| **Account** | `#E9E4E9` | `(233,228,233,255)` | Balance text, highlighted data |

### Semantic Usage Map

| Element | Color |
|---------|-------|
| Headings | `colors.Green`, `Bold: true` |
| Body text | `theme.Foreground` (`#D0D0D0`) |
| Labels | `colors.Gray` |
| Primary buttons | `theme.ColorNameButton` (green at 75%) |
| Disabled buttons | `theme.ColorNameDisabledButton` (green at 13%) |
| Error text | `theme.ColorNameError` / `colors.Red` |
| Warning text | `colors.Yellow` |
| Status dot (connected) | `colors.Network` (green) |
| Status dot (disconnected) | `colors.Red` |
| Background | `theme.ColorNameBackground` (`#15171E`) |
| Input background | Transparent |
| Separator | `#888888` at 35% |
| Placeholder | `#888888` |
| Scrollbar | `colors.Green` at 44% |
| Focus ring | `colors.Green` at 88% |
| Hover | `colors.Green` at 99% |
| Overlay background | `#1F2128` |

### Future Remapping

The `colors.*` struct (`functions.go:96–111`) holds all semantic color variables. A future theme variant replaces these values while keeping the same usage pattern.

---

## II. Typography

### Font Stack

Source: `theme.go:69–91` (eTheme.Font)

| Style | Resource | Variant |
|-------|----------|---------|
| Regular body | `resourceRegularTtf` | Default |
| **Bold** | `resourceBoldTtf` | `TextStyle{Bold: true}` |
| *Italic* | `resourceItalicTtf` | `TextStyle{Italic: true}` |
| ***Bold Italic*** | `resourceBoldItalicTtf` | `TextStyle{Bold: true, Italic: true}` |
| Monospace | `resourceRegularTtf` | `TextStyle{Monospace: true}` |
| Symbol/Display | `resourceAstrolytTtf` | `TextStyle{Symbol: true}` |

**Symbol font (`resourceAstrolytTtf`)** is reserved for the dashboard marquee display text only. Do not use `Symbol` style for body copy, labels, or buttons.

### Size Scale

Source: `theme.go:97–126` (getBaseSize), `scaling.go:97–103` (scaleFont)

All sizes pass through `scaleFont()`, which caps growth above 1.2×:
```go
if factor > 1.2 {
    factor = 1.2 + (factor-1.2)*0.5
}
```

| Token | Base (px) | Scaled | Usage |
|-------|-----------|--------|-------|
| `SizeNameCaptionText` | 11 | ~11–15 | Timestamps, meta labels |
| `SizeNameText` | 15 | ~15–20 | Body text, button labels |
| `SizeNameHeadingText` | 24 | ~24–32 | Screen titles, balance |
| Custom text | — | `scaleFont(n)` | Ad-hoc sizes in layouts |

### Text Style Rules

- **Headings**: `Bold: true` + `colors.Green`, size `scaleFont(22)`
- **Section labels**: `Bold: true` + `colors.Gray`, size `scaleFont(14)`
- **Body text**: Regular weight, no style override
- **Balance/amounts**: `Bold: true` + `colors.Account`, size `scaleFont(28)`
- **Never** use italic or underlined text in production UI (except hyperlinks)
- **Never** use Symbol font outside the dashboard marquee (`widgets_ui.go:pulseText`)

---

## III. Spacing & Layout Grid

### Reference Canvas

| Constant | Value | Source |
|----------|-------|--------|
| `ReferenceWidth` | 324px | `scaling.go:11` |
| `ReferenceHeight` | 680px | `scaling.go:12` |
| `MinScale` | 0.75 | `scaling.go:13` |
| `MaxScale` | 1.50 | `scaling.go:14` |

- Desktop always returns **scale = 1.0**
- Mobile scale = `min(width/324, height/680)`, clamped [0.75, 1.5]

### Predefined Spacers

Source: `scaling.go:105–119`

All spacers are transparent `canvas.Rectangle` with `SetMinSize(scalePoint(w, h))`.

| Helper | Base Size | Scaled | Usage |
|--------|-----------|--------|-------|
| `smallSpacerSize()` | 5×5 | `scalePoint(5,5)` | Tight grouping |
| `compactSpacerSize()` | 6×5 | `scalePoint(6,5)` | Between form fields |
| `standardSpacerSize()` | 10×5 | `scalePoint(10,5)` | Between sections |
| `scaleSpacer(w)` | w×1 | `scalePoint(w,1)` | Custom horizontal gaps |

### Rule

**Never use `widget.NewLabel(" ")` for spacing.** Always use `canvas.NewRectangle(color.Transparent)` with `SetMinSize()`.

### Padding

```go
ui.Padding = ui.MaxWidth * 0.05  // 5% of screen width
```

---

## IV. Window & Screen Dimensions

### Desktop

Source: `main.go:336–341`

| Property | Value |
|----------|-------|
| `ui.MaxWidth` | 360 |
| `ui.MaxHeight` | 680 |
| `ui.Width` | 324 (`MaxWidth * 0.9`) |
| `ui.Height` | 680 |
| Window mode | `SetFixedSize(true)` |
| Scale | 1.0 (`scale()` returns 1.0 for desktop) |
| Centering | `session.Window.CenterOnScreen()` |
| Padding | `session.Window.SetPadded(false)` |

### Mobile

Source: `main.go:321–332`, `shared_layouts.go:287–375`

| Property | Value |
|----------|-------|
| `ui.MaxWidth` | 3600 (10× for high-DPI pixel space) |
| `ui.MaxHeight` | 6800 |
| `ui.Width` | 3240 (`MaxWidth * 0.9`) |
| `ui.Height` | 6800 |
| Orientation | Dynamic (polled every 1s in `layoutFrame()`) |
| Horizontal mode | Width goes to `MaxWidth * 0.7` |

---

## V. Layout Patterns

### Every Screen Must Have This Root Structure

```go
func layoutMyScreen() fyne.CanvasObject {
    resizeWindow(ui.MaxWidth, ui.MaxHeight)
    session.Domain = "app.myscreen"

    // Define visual elements at function top
    heading := canvas.NewText("Screen Title", colors.Green)
    heading.TextSize = scaleFont(22)
    heading.TextStyle = fyne.TextStyle{Bold: true}
    heading.Alignment = fyne.TextAlignCenter

    label := canvas.NewText("Section", colors.Gray)
    label.TextSize = scaleFont(14)
    label.TextStyle = fyne.TextStyle{Bold: true}

    // Use standard spacers
    rectSpacer := canvas.NewRectangle(color.Transparent)
    rectSpacer.SetMinSize(standardSpacerSize())

    // Group elements in middle
    form := container.NewVBox(rectSpacer, heading, rectSpacer, label, ...)

    // Anchor with Border layout
    frame := &iframe{}
    c := container.NewBorder(top, bottom, nil, nil, form)

    // Stack over background + wrap in scroll
    layout := container.NewStack(frame, res.mainBg, c)
    return NewVScroll(layout)
}
```

### Container Hierarchy Rules

| Level | Container | Purpose |
|-------|-----------|---------|
| 1 | `container.NewStack` | Full-screen root: iframe + background + content |
| 2 | `container.NewBorder` | Top/bottom anchors + center content area |
| 3 | `container.NewVBox` / `NewHBox` | Vertical/horizontal linear sections |
| 4 | `container.NewCenter` | Centering within a section |
| 5 | `container.New(layout.NewGridLayout(n))` | Equal-width columns (n-column button rows) |
| 5 | `container.NewGridWrap(fyne.Size, ...)` | Fixed-size tiles in a row |

### Scroll Pattern

- Always wrap the final layout in `NewVScroll()` (defined in `custom.go:298`)
- `NewVScroll` adds `container.NewCenter` around content
- On mobile: call `SetCurrentScrollBox(scrollContainer)` at page mount for keyboard scroll handling

---

## VI. Navigation System

### Stack-Based Navigation

Source: `navigation.go`

```go
type NavigationStack struct {
    history []NavEntry  // {Domain string, CanGoBack bool}
    maxSize int         // 20
}
```

| Method | Behavior |
|--------|----------|
| `Push(domain, canGoBack)` | Add to history (deduplicates consecutive) |
| `Pop()` | Remove current, return previous |
| `CanGoBack()` | Check if back is possible |
| `Current()` | Peek at top |
| `Clear()` | Reset stack |

### Domain Registry

Source: `navigation.go:137–154`

| Domain | Layout Function | Back-Nav |
|--------|----------------|----------|
| `app.main` | `layoutMain()` | Exit (mobile) |
| `app.wallet` | `layoutDashboard()` | Yes |
| `app.send` | `layoutSend()` | Yes |
| `app.receive` | `layoutReceive()` | Yes |
| `app.service` | `layoutServiceAddress()` | Yes |
| `app.create` | `layoutNewAccount()` | Yes |
| `app.restore` | `layoutRestore()` | Yes |
| `app.explorer` | `layoutAssetExplorer()` | Yes |
| `app.myassets` | `layoutMyAssets()` | Yes |
| `app.transfers` | `layoutTransfers()` | Yes |
| `app.settings` | `layoutSettings()` | Yes |
| `app.appsettings` | `layoutAppSettings()` | Yes |
| `app.messages` | `layoutMessages()` | Yes |
| `app.messages.contact` | `layoutMessages()` → `layoutPM()` | Yes |
| `app.remoteaccess` | `layoutRemoteAccess()` | Yes |
| `app.register` | `layoutNewAccount()` | No |

### Transition Rule

```go
session.Window.SetContent(layoutTransition())
session.Window.SetContent(layoutTarget())
session.NavStack.Push(session.Domain, true)
```

Always use `layoutTransition()` as the interstitial between screens. This shows the loading animation while the next screen builds.

---

## VII. Custom Widget Catalog

| Widget | File:Line | Type | When To Use |
|--------|-----------|------|-------------|
| `iframe` | `widgets_ui.go:106` | Transparent full-screen wrapper | **Every** screen root |
| `walletBtn` | `widgets_ui.go:18` | Tappable card with colored bg | Wallet file selection, menu cards |
| `pulseText` | `widgets_ui.go` | Animated glow text | Status updates, attention calls |
| `returnEntry` | `custom.go:45` | Entry with Enter handler | Search bars, single-field forms |
| `mobileEntry` | `custom.go:234` | Entry with mobile keyboard scroll | Mobile forms, address input |
| `mobileEntryWithScroll` | `custom.go:250` | Entry + scroll hint | Mobile forms in scroll containers |
| `slimProgressBar` | `custom.go:580` | Themed progress bar | Sync progress, PoW hashing |
| `tintTheme` | `custom.go:33` | Color overlay theme | Icon coloring in buttons |
| `contextMenuButton` | `custom.go:279` | Button with dropdown menu | Options, filtering |
| `hoverButton` | `custom.go:365` | Desktop hover events | Nav tiles with icon swap |
| `slimProgressBar` | `custom.go:580` | 6px green progress bar | Sync/download progress |
| `NewSpacer` | `custom.go:711` | Sized empty spacer | Precise layout gaps |
| `verticalSeparator` | `custom.go` | Thin vertical line | Dividing sections |
| `circularIcon` | `custom.go` | Icon in a circle | Profile, status badges |

---

## VIII. Button System

### All Button Variants

| Variant | Function | Desktop Min | Mobile Min | Use Case |
|---------|----------|-------------|------------|----------|
| Standard | `widget.NewButton` | Fyne default | Fyne default | Simple actions |
| Wallet Btn | `newWalletBtn()` | 100×36 | 100×36 | Wallet selection cards |
| Icon Label | `newIconLabelButton()` | 100×60 | 100×68 | Dashboard nav tiles |
| Gunmetal | `newGunmetalButtonWithIcon()` | 160×40 | 160×48 | Primary actions |
| Bordered | `newBorderedButtonWithIcon()` | 160×40 | 160×48 | Secondary actions |
| Sized Text | `newSizedTextButton()` | 160×40 | 160×48 | Inline text buttons |
| Sized Icon | `newSizedIconButton()` | 100×40 | 100×48 | Icon-only buttons |
| Image Button | `NewImageButton()` | 100×40 | 100×48 | Image-labeled buttons |
| Large Icon | `newLargeIconButton()` | custom×48 | custom×56 | Featured actions |
| TELA | `newTELAButton()` | custom×60 | custom×68 | TELA marketplace entry |
| Small Icon | `newSmallIconButton()` | 110×40 | 110×40 | Compact actions |
| Small Icon Link | `newSmallIconLink()` | 80×20 | 80×20 | Inline icon-hyperlinks |

### Height Rule

```go
h := float32(40)   // desktop base
if isMobile() {
    h = 48          // minimum tap target
}
```

### Importance

- Primary actions: `widget.MediumImportance`
- Secondary actions: `widget.LowImportance`
- Destructive/delete: use `widget.DangerImportance`

### Button Container Pattern

Most non-standard buttons follow this pattern:

```go
btn := widget.NewButton(label, onTap)
btn.Importance = widget.MediumImportance

sizeEnforcer := canvas.NewRectangle(color.Transparent)
sizeEnforcer.SetMinSize(fyne.NewSize(width, scaleSize(height)))

return container.NewStack(sizeEnforcer, btn)
```

---

## IX. Responsive Behavior

### Mobile Detection

Two patterns coexist in the codebase. **This is a known inconsistency.**

| Pattern | File | Method | Scope |
|---------|------|--------|-------|
| Compile-time | `scaling.go:17` | `isMobile() = runtime.GOOS == "android" \|\| "ios"` | Scale factor, spacers |
| Runtime | `auth_layouts.go:596` | `a.Driver().Device().IsMobile()` | Layout branching |

**Concern:** `runtime.GOOS` is a compile-time constant. It does not detect tablets, Fyne desktop test runners on mobile devices, or windowed mode. Prefer `a.Driver().Device().IsMobile()` for future layout branches.

### Branching Pattern

```go
if isMobile() || a.Driver().Device().IsMobile() {
    // Mobile: wrap buttons in wrapMobileButton(),
    // tighter spacing, keyboard scroll handling
    form = container.NewVBox(
        wrapMobileButton(btn),
    )
} else {
    // Desktop: standard sizing, hover animations,
    // no wrapMobileButton()
    form = container.NewVBox(btn)
}
```

### Scale Factor

| Platform | Scale | Behavior |
|----------|-------|----------|
| Desktop | Always 1.0 | `scale()` returns 1.0 immediately |
| Mobile | `min(w/324, h/680)` | Clamped [0.75, 1.5], font tapers above 1.2× |
| Small screen | Height < 700 or scale < 0.9 | `compactSpacing()` returns 0.5× |

### Icon Sizing

```go
buttonIconSize()  // min(scaleSize(20), 24) × same
navIconSize()     // min(scaleSize(16), 20) × same
```

---

## X. Mobile-Specific Rules

### Tap Targets

Minimum interactive area: **48×48 density-independent pixels**.

Enforced by `wrapMobileButton()` (`widgets_ui.go:98`):
```go
func wrapMobileButton(obj fyne.CanvasObject) fyne.CanvasObject {
    if isMobile() {
        sizeEnforcer := canvas.NewRectangle(color.Transparent)
        sizeEnforcer.SetMinSize(scalePoint(48, 48))
        return container.NewStack(sizeEnforcer, obj)
    }
    return obj
}
```

All interactive elements smaller than 48×48 must be wrapped in `wrapMobileButton()`.

### Keyboard Handling

1. Use `mobileEntry` or `mobileEntryWithScroll` instead of `widget.Entry` for input fields
2. Call `SetCurrentScrollBox(scrollContainer)` at page mount
3. On focus, `scrollToFieldOnMobile()` auto-scrolls the field to `desiredY = scaleSize(160)` after a 300ms delay

### Orientation

- Handled by `layoutFrame()` in `shared_layouts.go:287–375`
- Mobile polls orientation every 1 second
- Width is reduced to `MaxWidth * 0.7` in horizontal orientation
- On orientation change: swap width/height, re-layout, keep content

### Lifecycle

- Foreground events >30s apart trigger wallet reconnection + UI refresh (`main.go:244–314`)
- `mobile.KeyBack` captured for back navigation (`main.go:228–234`)
- On app exit from login screen: use `mobile.Driver.GoBack()` (`main.go:416–421`)

---

## XI. Desktop-Specific Rules

### Window

- Fixed **360×680**, no resize (`SetFixedSize(true)`)
- Centered on screen: `session.Window.CenterOnScreen()`
- Unpadded: `session.Window.SetPadded(false)`
- Scale always **1.0** — no responsive scaling

### Hover Effects

- Fyne standard buttons: built-in green hover (99% opacity)
- Nav tiles: `hoverButton` + `newIconLabelButtonWithColor` swap icon tint on hover
- Custom widgets: test with `desktop.MouseIn`/`MouseOut` for hover state

### Animations

- Fade animations for marquee text (`wallet_layouts.go:82–126`)
- `pulseButton`: 2-second green stroke animation on rectangles
- `NewColorRGBAAnimation` for fade transitions (400ms)

---

## XII. Status Indicators

### Connection Dots

Source: `main.go:182–201`

```go
status.Connection = canvas.NewCircle(colors.Red)  // default: red
// Turns green when connected via colors.Network
```

| Dot | Variable | Size |
|-----|----------|------|
| Daemon connection | `status.Connection` | `statusDotSize()` (10×10) |
| Sync status | `status.Sync` | `statusDotSize()` (10×10) |
| Gnomon | `status.Gnomon` | `statusDotSize()` (10×10) |
| EPOCH | `status.EPOCH` | `statusDotSize()` (10×10) |

Color: `colors.Red` → `colors.Network` (green) when active.

### Animated Text

- `pulseText`: Alternates between `colors.Green` and `colors.LightBlue` glow
- Used for loading, connecting, attention states

### Progress Bar

- `slimProgressBar`: 6px height, green fill
- Track: `color.NRGBA{R: 36, G: 38, B: 44}`
- Label: percentage in `colors.Gray`, trailing alignment, 11pt

---

## XIII. Future-Proofing for Theme Variants

### Architecture Extension Points

The codebase already supports multiple themes. The current default is **"Engram Classic"**.

| Mechanism | Location | Purpose |
|-----------|----------|---------|
| `Theme` struct | `functions.go:242` | Holds theme implementations (`main`, `alt`) |
| `eTheme` | `theme.go:26` | Engram Classic implementation |
| `eTheme2` | `theme.go:128` | Alternative theme (Noto fonts, same palette) |
| `tintTheme` | `custom.go:33` | Per-element color overlay |
| `themes` global | `main.go:84` | Global theme manager |
| `a.Settings().SetTheme()` | `main.go:127` | Activation mechanism |

### How to Add a New Theme

1. Create a new struct implementing the `fyne.Theme` interface (e.g., `eThemeNight`):
   - `Color(name, variant)` — return your palette
   - `Font(style)` — return your font resources
   - `Icon(name)` — delegate to `theme.DefaultTheme().Icon()`
   - `Size(name)` — call `scaleFont(getBaseSize(s))`
2. Add the instance to the `Theme` struct in `functions.go`
3. Wire a theme selector in `settings_layouts.go` or `auth_layouts.go`:
   ```go
   a.Settings().SetTheme(themes.night)
   ```
4. Theme selection persists via `store.go` encrypted storage

### Design Rules for New Themes

- The layout system, spacing, scaling, navigation, and widget architecture are **theme-agnostic** — they work with any color/font combination
- Only `theme.go` + font resources need changes for a new theme
- Do **not** change `scaling.go`, `custom.go`, or layout files for a theme variant
- `tintTheme` can be used for per-widget color overrides in any theme

---

## XIV. Best Practices

### Code Style

- Run `gofmt -w` on all touched Go files
- Run `goimports -w` if imports change
- Group imports: stdlib → third-party → `github.com/DEROFDN/engram`
- Avoid unused imports — build must stay clean

### File Organization

| Pattern | Contents |
|---------|----------|
| `*_layouts.go` | One `layout*()` function per screen domain |
| `theme.go` | Theme implementations (`eTheme`, `eTheme2`) |
| `scaling.go` | Responsive scale, spacers, icon sizes |
| `widgets_ui.go` | App-specific widgets (`walletBtn`, `pulseText`, `iframe`) |
| `custom.go` | Reusable custom widgets (`returnEntry`, `slimProgressBar`) |
| `navigation.go` | `NavigationStack`, domain registry |

### UI Thread Safety

```go
// All UI updates from goroutines:
fyne.Do(func() {
    // UI code here
})

// Helper wrapper used throughout:
func uiDo(fn func()) {
    fyne.Do(fn)
}

// Window resize wrapper:
func resizeWindow(w, h float32) {
    uiDo(func() {
        session.Window.Resize(fyne.NewSize(w, h))
    })
}
```

### Background Work

- Blocking calls (RPC, wallet, Gnomon, file I/O) in goroutines
- Check `appExiting` before touching UI from goroutines
- Check wallet generation is still active (`isWalletGenerationActive(generation)`)
- Use `fyne.Do()` for every UI mutation from async paths
- Never block the UI thread with network or disk operations

### Layout Construction Order

1. Set `session.Domain` first
2. Call `resizeWindow(ui.MaxWidth, ui.MaxHeight)`
3. Define all visual elements at function top (canvases, texts, entries, buttons)
4. Assemble spacers and groupings in the middle
5. Create `&iframe{}` and pack into `container.NewStack`
6. Wrap in `NewVScroll()` and return
7. If applicable, push to `NavStack`

---

## XV. Appendix: Complete Screen Map

| # | Layout Function | File | Domain | Back-Nav | Notes |
|---|----------------|------|--------|----------|-------|
| 1 | `layoutMain()` | `auth_layouts.go` | `app.main` | Exit (mobile) | Login, file/seed auth |
| 2 | `layoutNewAccount()` | `auth_layouts.go` | `app.create`, `app.register` | No | Wallet creation + PoW |
| 3 | `layoutRestore()` | `auth_layouts.go` | `app.restore` | Yes | Seed phrase restore |
| 4 | `layoutLanguageSelector()` | `language_layouts.go` | — | No | First-run language picker |
| 5 | `layoutDashboard()` | `wallet_layouts.go` | `app.wallet` | Yes | Main wallet dashboard |
| 6 | `layoutSend()` | `wallet_layouts.go` | `app.send` | Yes | Send DERO/assets |
| 7 | `layoutReceive()` | `wallet_layouts.go` | `app.receive` | Yes | Receive, QR display |
| 8 | `layoutServiceAddress()` | `wallet_layouts.go` | `app.service` | Yes | Service address mgmt |
| 9 | `layoutTransfers()` | `wallet_layouts.go` | `app.transfers` | Yes | Transfer history |
| 10 | `layoutMessages()` | `message_layouts.go` | `app.messages` | Yes | Message inbox list |
| 11 | `layoutPM()` | `message_layouts.go` | `app.messages.contact` | Yes | Individual conversation |
| 12 | `layoutAccount()` | `account_layouts.go` | — | Yes | Wallet file management |
| 13 | `layoutAssetExplorer()` | `asset_layouts.go` | `app.explorer` | Yes | SC/asset browsing |
| 14 | `layoutMyAssets()` | `asset_layouts.go` | `app.myassets` | Yes | User's owned assets |
| 15 | `layoutSettings()` | `settings_layouts.go` | `app.settings` | Yes | Network, node, language |
| 16 | `layoutAppSettings()` | `settings_layouts.go` | `app.appsettings` | Yes | App preferences |
| 17 | `layoutDatapad()` | `datapad_layouts.go` | — | Yes | Encrypted notepad |
| 18 | `layoutFileStorage()` | `file_layouts.go` | — | Yes | File explorer/storage |
| 19 | `layoutIdentity()` | `identity_layouts.go` | — | Yes | DID management |
| 20 | `layoutTELA()` | `tela_layouts.go` | — | Yes | TELA app marketplace |
| 21 | `layoutTelaAppDetails()` | `tela_layouts.go` | — | Yes | TELA app detail view |
| 22 | `layoutXSWD()` | `xswd_layouts.go` | — | Yes | xswd daemon interface |
| 23 | `layoutRemoteAccess()` | `xswd_layouts.go` / `functions.go` | `app.remoteaccess` | Yes | Remote access settings |
| 24 | `layoutTransition()` | `shared_layouts.go` | — | — | Loading interstitial |
| 25 | `layoutAlert(t)` | `shared_layouts.go` | — | — | Error/warning modals |
| 26 | `layoutWaiting()` | `shared_layouts.go` | — | — | PoW registration wait |
