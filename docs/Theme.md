# Engram Theme & Design System

> Design rules for the Engram DERO Protocol wallet — Desktop (Windows, Linux, macOS) & Mobile (Android, iOS).
>
> Five themes are available: **Engram Classic** (default), **Derotopia**, **El Dorado**, **Crystallina**, and **Atlantis**.
> Theme selection persists via encrypted storage and is switchable at runtime from Settings.

---

## I. Theme Palette Reference

Each theme defines a `Colors` struct (`internal/theme/colors.go`) with the following tokens. The active theme's colors are accessed at runtime through the package-level pointer `apptheme.C`.

### Engram Classic

| Token | Hex | RGBA | Role |
|-------|-----|------|------|
| **DarkMatter** | `#15171E` | `(21,23,30,255)` | Background |
| **Green** | `#13CA69` | `(19,202,105,255)` | Primary accent, active state |
| **DarkGreen** | `#117F4E` | `(17,127,78,255)` | Secondary green, pressed |
| **LightBlue** | `#38B6FF` | `(56,182,255,255)` | Marquee, info |
| **Blue** | `#1BFD7F` | `(27,249,127,255)` | Links, secondary accent |
| **Purple** | `#BF40BF` | `(191,64,191,255)` | Special status |
| **Yellow** | `#F4D00B` | `(244,208,11,255)` | Warnings, attention |
| **Red** | `#F44336` | `(244,67,54,255)` | Errors |
| **SoftRed** | `#F06E6E` | `(240,110,110,255)` | Soft error, caution |
| **Gray** | `#636363` | `(99,99,99,255)` | Labels, secondary text |
| **Flint** | `#2C2C34` | `(44,44,52,255)` | Button/card backgrounds |
| **Cold** | `#3C495C` | `(60,73,92,255)` | Inactive, muted |
| **Network** | `#43EF43` | `(67,239,67,255)` | Connection status |
| **Account** | `#E9E4E9` | `(233,228,233,255)` | Balance text, highlighted data |

### Derotopia

| Token | Hex | RGBA | Role |
|-------|-----|------|------|
| **DarkMatter** | `#120C1C` | `(18,12,28,255)` | Dark eggplant background |
| **Green** | `#8A2BE2` | `(138,43,226,255)` | Purple accent |
| **DarkGreen** | `#6214B4` | `(98,20,180,255)` | Dark purple |
| **LightBlue** | `#FF69B4` | `(255,105,180,255)` | Candy pink marquee |
| **Blue** | `#6A0DAD` | `(106,13,173,255)` | Deep purple |
| **Purple** | `#13CA69` | `(19,202,105,255)` | Green (inverted role) |
| **Yellow** | `#F4D00B` | `(244,208,11,255)` | Warnings |
| **Red** | `#D64A46` | `(214,74,70,255)` | Errors |
| **SoftRed** | `#F06E6E` | `(240,110,110,255)` | Soft error |
| **Gray** | `#636363` | `(99,99,99,255)` | Labels |
| **Flint** | `#2C2C34` | `(44,44,52,255)` | Card backgrounds |
| **Cold** | `#3C495C` | `(60,73,92,255)` | Inactive |
| **Network** | `#8A2BE2` | `(138,43,226,255)` | Connection status (purple) |
| **Account** | `#E9E4E9` | `(233,228,233,255)` | Balance text |

### El Dorado

| Token | Hex | RGBA | Role |
|-------|-----|------|------|
| **DarkMatter** | `#1E140A` | `(30,20,10,255)` | Dark bronze background |
| **Green** | `#FFD700` | `(255,215,0,255)` | Gold accent |
| **DarkGreen** | `#B8860B` | `(184,134,11,255)` | Dark goldenrod |
| **LightBlue** | `#50C878` | `(80,200,120,255)` | Emerald green marquee |
| **Blue** | `#DAA520` | `(218,165,32,255)` | Goldenrod |
| **Purple** | `#CD7F32` | `(205,127,50,255)` | Bronze |
| **Yellow** | `#FFBF00` | `(255,191,0,255)` | Amber |
| **Red** | `#D64A46` | `(214,74,70,255)` | Errors |
| **SoftRed** | `#F06E6E` | `(240,110,110,255)` | Soft error |
| **Gray** | `#636363` | `(99,99,99,255)` | Labels |
| **Flint** | `#2C2C34` | `(44,44,52,255)` | Card backgrounds |
| **Cold** | `#3C495C` | `(60,73,92,255)` | Inactive |
| **Network** | `#FFD700` | `(255,215,0,255)` | Connection status (gold) |
| **Account** | `#E9E4E9` | `(233,228,233,255)` | Balance text |

### Crystallina

| Token | Hex | RGBA | Role |
|-------|-----|------|------|
| **DarkMatter** | `#F0F2F5` | `(240,242,245,255)` | Off-white background |
| **Green** | `#7C5CBF` | `(124,92,191,255)` | Amethyst accent |
| **DarkGreen** | `#5C3C9F` | `(92,60,159,255)` | Dark amethyst |
| **LightBlue** | `#38B6FF` | `(56,182,255,255)` | Sky blue marquee |
| **Blue** | `#5CB4F0` | `(92,180,240,255)` | Ice blue |
| **Purple** | `#C896FF` | `(200,150,255,255)` | Light lilac |
| **Yellow** | `#F5D744` | `(245,215,68,255)` | Warm yellow |
| **Red** | `#D64A46` | `(214,74,70,255)` | Errors |
| **SoftRed** | `#F06E6E` | `(240,110,110,255)` | Soft error |
| **Gray** | `#8E8EA0` | `(142,142,160,255)` | Labels |
| **Flint** | `#D8DAE6` | `(216,218,230,255)` | Light card surfaces |
| **Cold** | `#A0AABE` | `(160,170,190,255)` | Inactive |
| **Network** | `#64C8D2` | `(100,200,210,255)` | Connection status (aquamarine) |
| **Account** | `#38384A` | `(56,56,74,255)` | Dark slate text |

### Atlantis

| Token | Hex | RGBA | Role |
|-------|-----|------|------|
| **DarkMatter** | `#041215` | `(4,18,21,255)` | Hadal zone background |
| **Green** | `#34A2B5` | `(52,162,181,255)` | Primary cyan-teal accent |
| **DarkGreen** | `#136C7A` | `(19,108,122,255)` | Darker cyan, pressed |
| **LightBlue** | `#E8B84B` | `(232,184,75,255)` | Ancient amber marquee |
| **Blue** | `#1A6378` | `(26,99,120,255)` | Deep ocean blue |
| **Purple** | `#6B5B8A` | `(107,91,138,255)` | Bioluminescent purple |
| **Yellow** | `#7A9A4A` | `(122,154,74,255)` | Phosphorescent green |
| **Red** | `#D64A46` | `(214,74,70,255)` | Errors |
| **SoftRed** | `#F06E6E` | `(240,110,110,255)` | Soft error |
| **Gray** | `#5A7A7A` | `(90,122,122,255)` | Muted seafoam labels |
| **Flint** | `#122A2E` | `(18,42,46,255)` | Button/card surfaces |
| **Cold** | `#1C3F45` | `(28,63,69,255)` | Inactive, muted |
| **Network** | `#34A2B5` | `(52,162,181,255)` | Connection status (cyan) |
| **Account** | `#B8D4D0` | `(184,212,208,255)` | Pale seafoam balance text |

### Color Architecture

**Source:** `internal/theme/colors.go`

The active palette is selected via `Activate(name)` which swaps the `C` pointer:

```go
func Activate(name string) {
    switch name {
    case ThemeDerotopia:   C = &derotopiaColors;   ThemeMode = ThemeDerotopia
    case ThemeElDorado:    C = &eldoradoColors;    ThemeMode = ThemeElDorado
    case ThemeCrystallina: C = &crystallinaColors; ThemeMode = ThemeCrystallina
    case ThemeAtlantis:    C = &atlantisColors;    ThemeMode = ThemeAtlantis
    default:               C = &engramColors;      ThemeMode = ThemeEngram
    }
}
```

All theme color access goes through `apptheme.C.<Token>` — the pointer swap makes every reference instantly theme-aware.

---

## II. Typography

Same as existing document — unchanged across themes.

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
- **Balance/amounts**: `Bold: true` + `apptheme.BalanceColor()`, size `scaleFont(28)`
- **Never** use italic or underlined text in production UI (except hyperlinks)
- **Never** use Symbol font outside the dashboard marquee (`widgets_ui.go:pulseText`)

---

## III. Spacing & Layout Grid

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

### Theme-Aware Button Card Colors

Source: `custom.go:386–398`

**`buttonCardColor()`** — background of dashboard nav tiles and gunmetal buttons:

| Theme | Hex | RGBA |
|-------|-----|------|
| Engram Classic | `#282A32` | `(40,42,50,255)` |
| Derotopia | `#282A32` | `(40,42,50,255)` |
| El Dorado | `#282A32` | `(40,42,50,255)` |
| Crystallina | `#E6E8F0` | `(230,232,240,255)` |
| Atlantis | `#282A32` | `(40,42,50,255)` |

**`buttonTextColor()`** — label text on those buttons:

| Theme | Hex | RGBA |
|-------|-----|------|
| Engram Classic | `#FFFFFF` | White |
| Derotopia | `#FFFFFF` | White |
| El Dorado | `#FFFFFF` | White |
| Crystallina | `#38384A` | `(56,56,74,255)` |
| Atlantis | `#FFFFFF` | White |

---

## IX. Responsive Behavior

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

Same as existing document — unchanged across themes.

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

Color: `colors.Red` → `colors.Network` (theme-specific) when active.

### Animated Text

- `pulseText`: Alternates between `colors.Green` and `colors.LightBlue` glow
- Used for loading, connecting, attention states

### Progress Bar

- `slimProgressBar`: 6px height, green fill
- Track: `color.NRGBA{R: 36, G: 38, B: 44}`
- Label: percentage in `colors.Gray`, trailing alignment, 11pt

---

## XIII. Theme-Specific Color Functions

Several functions return theme-appropriate colors for specific UI roles rather than using `C.<Token>` directly.

### Status / Loading Text Color

Source: `internal/theme/colors.go` — `StatusTextColor()`

Used for TELA loading indicators ("Connecting to node", "Fetching content", "Preparing app", etc.), wallet connection status text, and the TELA app count display.

| Theme | Color | Hex | RGBA |
|-------|-------|-----|------|
| Engram Classic | LightBlue (sky blue) | `#38B6FF` | `(56,182,255,255)` |
| Derotopia | LightBlue (candy pink) | `#FF69B4` | `(255,105,180,255)` |
| El Dorado | LightBlue (emerald) | `#50C878` | `(80,200,120,255)` |
| Crystallina | Dark teal (hardcoded) | `#008296` | `(0,130,150,255)` |
| Atlantis | Cyan (hardcoded) | `#34A2B5` | `(52,162,181,255)` |

### Balance Text Color

Source: `internal/theme/colors.go` — `BalanceColor()`

Used for the DERO balance amount on the dashboard.

| Theme | Color | Hex | RGBA |
|-------|-------|-----|------|
| Engram Classic | Green | `#13CA69` | `(19,202,105,255)` |
| Derotopia | Sky blue | `#38B6FF` | `(56,182,255,255)` |
| El Dorado | Green (gold) | `#FFD700` | `(255,215,0,255)` |
| Crystallina | Green (amethyst) | `#7C5CBF` | `(124,92,191,255)` |
| Atlantis | Green (cyan) | `#34A2B5` | `(52,162,181,255)` |

### Dashboard Icon Colors

Source: `wallet_layouts.go:395–424`

Used for the Settings, Notes, Messages, and Contracts buttons on the main dashboard.

| Theme | Color | Hex | RGBA |
|-------|-------|-----|------|
| Engram Classic | Green | `#13CA69` | `(19,202,105,255)` |
| Derotopia | Sky blue | `#38B6FF` | `(56,182,255,255)` |
| El Dorado | Goldenrod | `#DAA520` | `(218,165,32,255)` |
| Crystallina | Amethyst | `#7C5CBF` | `(124,92,191,255)` |
| Atlantis | Cyan | `#34A2B5` | `(52,162,181,255)` |

### Balance Pulse Animation

Source: `functions.go:3011–3054` — `pulseBalancePending()`

When a send transaction is pending, the balance text pulses between two colors:

| Theme | Color A | Color B |
|-------|---------|---------|
| Engram Classic | Green `#13CA69` | Yellow `#F4D00B` |
| Derotopia | Sky blue `#38B6FF` | Candy pink `#FF69B4` |
| El Dorado | Emerald `#13CA69` | Gold `#FFD700` |
| Crystallina | Amethyst `#7C5CBF` | Sky blue `#38B6FF` |
| Atlantis | Cyan `#34A2B5` | Amber `#E8B84B` |

### Marquee Text Color

Source: `wallet_layouts.go:67` — dashboard marquee uses `C.LightBlue`

| Theme | Hex | Value |
|-------|-----|-------|
| Engram Classic | `#38B6FF` | Sky blue |
| Derotopia | `#FF69B4` | Candy pink |
| El Dorado | `#50C878` | Emerald |
| Crystallina | `#38B6FF` | Sky blue |
| Atlantis | `#E8B84B` | Ancient amber |

### Globe Icon Resources

Source: `browser_globe_resource.go`

Two separate icon functions for different contexts:

**`globeResource()`** — TELA browser tab "Apps" button. Uses theme accent colors:

| Theme | Hex |
|-------|-----|
| Engram Classic | `#13CA69` (green) |
| Derotopia | `#8A2BE2` (purple) |
| El Dorado | `#FFD700` (gold) |
| Crystallina | `#7C5CBF` (amethyst) |
| Atlantis | `#34A2B5` (cyan) |

**`explorerGlobeResource()`** — TELA app detail "Open in explorer" button. Uses white/dark:

| Theme | Hex |
|-------|-----|
| Engram Classic | `#FFFFFF` (white) |
| Derotopia | `#FFFFFF` (white) |
| El Dorado | `#FFFFFF` (white) |
| Crystallina | `#38384A` (dark slate) |

---

## XIV. Architecture: Per-Theme Color System

### How It Works

The theme system uses a **pointer-swap** architecture, not separate `fyne.Theme` implementations:

1. **`apptheme.C`** — a package-level pointer to the active `Colors` struct. All semantic color access goes through `apptheme.C.<Token>`.
2. **`apptheme.ThemeMode`** — a package-level enum tracking the active theme name.
3. **`apptheme.Activate(name)`** — swaps `C` and `ThemeMode` to the selected theme.
4. **`a.Settings().SetTheme(apptheme.Main)`** — tells Fyne to re-read theme colors after activation.

### File Layout

| File | Contents |
|------|----------|
| `internal/theme/colors.go` | `Colors` struct, palette definitions, `Activate()`, `StatusTextColor()`, `BalanceColor()` |
| `internal/theme/theme.go` | `ETheme`/`ETheme2` Fyne implementations, `bgColor()`, `accentNRGBA()`, `crystallinaFyneColor()` |

### The Fyne Theme Layer

The Fyne theme providers (`ETheme` and `ETheme2`) in `theme.go` return colors via:

- **`bgColor()`** — reads `C.DarkMatter` and converts to `color.NRGBA` for `ColorNameBackground`. Makes the app-wide background per-theme.
- **`accentNRGBA()`** — returns the theme's accent color (Green equivalent) for `ColorNameButton`, `ColorNamePrimary`, `ColorNameFocus`, etc.
- **`crystallinaFyneColor()`** — Crystallina-only intercept that overrides 10+ Fyne color names (Background, Foreground, MenuBackground, InputBackground, etc.) for the light theme.

### Adding a New Theme

1. Add a new `var <name>Colors` struct literal in `internal/theme/colors.go` with all 14 tokens.
2. Add a `Theme<Name>` constant.
3. Add a case in `Activate()`.
4. If the theme is light, add entries to `crystallinaFyneColor()` (or rename it to handle all light themes).
5. If `StatusTextColor()` or `BalanceColor()` need special handling, add a case there.
6. Theme selection UI is in `settings_layouts.go:1888`.

### Design Rules

- Layout, spacing, scaling, navigation, and widget architecture are **theme-agnostic** — they work with any palette.
- The `C` pointer-swap means **no layout files need changes** for new palette values.
- Explicit per-theme color functions (`StatusTextColor()`, `BalanceColor()`) are only needed when a color role doesn't map to a single `C.<Token>` value.
- For dark themes: DarkMatter should use `(R, G, B)` values where each channel <= 40 to maintain contrast with white/gray text.
- For light themes: add entries to `crystallinaFyneColor()` and update `IsLightTheme()`, `buttonCardColor()`, `buttonTextColor()`.

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
