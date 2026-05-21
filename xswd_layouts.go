// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license can be found in the LICENSE file.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"fmt"
	"image/color"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/civilware/epoch"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi/xswd"
)

func layoutXSWDAppManager(ad *xswd.ApplicationData) fyne.CanvasObject {
	session.Domain = "app.remoteaccess.manager"

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.58))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	labelName := widget.NewRichText(&widget.TextSegment{
		Text: ad.Name,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
		}})
	labelName.Wrapping = fyne.TextWrapWord

	labelDesc := widget.NewRichText(&widget.TextSegment{
		Text: ad.Description,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			TextStyle: fyne.TextStyle{Bold: false},
		}})
	labelDesc.Wrapping = fyne.TextWrapWord

	labelID := canvas.NewText("   APP  ID", colors.Gray)
	labelID.TextSize = scaleFont(14)
	labelID.Alignment = fyne.TextAlignLeading
	labelID.TextStyle = fyne.TextStyle{Bold: true}

	textID := widget.NewRichTextFromMarkdown(ad.Id)
	textID.Wrapping = fyne.TextWrapWord

	labelSignature := canvas.NewText("   SIGNATURE", colors.Gray)
	labelSignature.TextSize = scaleFont(14)
	labelSignature.Alignment = fyne.TextAlignLeading
	labelSignature.TextStyle = fyne.TextStyle{Bold: true}

	textSignature := widget.NewRichTextFromMarkdown("")
	textSignature.Wrapping = fyne.TextWrapWord

	labelURL := canvas.NewText("   URL", colors.Gray)
	labelURL.TextSize = scaleFont(14)
	labelURL.Alignment = fyne.TextAlignLeading
	labelURL.TextStyle = fyne.TextStyle{Bold: true}

	textURL := widget.NewRichTextFromMarkdown(ad.Url)
	textURL.Wrapping = fyne.TextWrapWord

	labelPermissions := canvas.NewText("   PERMISSIONS", colors.Gray)
	labelPermissions.TextSize = scaleFont(14)
	labelPermissions.Alignment = fyne.TextAlignLeading
	labelPermissions.TextStyle = fyne.TextStyle{Bold: true}

	labelEvents := canvas.NewText("   EVENTS", colors.Gray)
	labelEvents.TextSize = scaleFont(14)
	labelEvents.Alignment = fyne.TextAlignLeading
	labelEvents.TextStyle = fyne.TextStyle{Bold: true}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")
	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")
	labelSeparator3 := widget.NewRichTextFromMarkdown("")
	labelSeparator3.Wrapping = fyne.TextWrapOff
	labelSeparator3.ParseMarkdown("---")
	labelSeparator4 := widget.NewRichTextFromMarkdown("")
	labelSeparator4.Wrapping = fyne.TextWrapOff
	labelSeparator4.ParseMarkdown("---")
	labelSeparator5 := widget.NewRichTextFromMarkdown("")
	labelSeparator5.Wrapping = fyne.TextWrapOff
	labelSeparator5.ParseMarkdown("---")
	labelSeparator6 := widget.NewRichTextFromMarkdown("")
	labelSeparator6.Wrapping = fyne.TextWrapOff
	labelSeparator6.ParseMarkdown("---")

	signatureItems := container.NewVBox(
		labelSeparator2,
		rectSpacer,
		rectSpacer,
		labelSignature,
		textSignature,
		rectSpacer,
		rectSpacer,
	)

	// Show signature result if one exists
	signatureItems.Hide()
	if len(ad.Signature) > 0 {
		signatureItems.Show()
		_, message, err := engram.Disk.CheckSignature(ad.Signature)
		if err != nil {
			textSignature.ParseMarkdown(err.Error())
		} else {
			textSignature.ParseMarkdown(strings.TrimSpace(string(message)))
		}
	}

	// Find Permissions for connected app and build UI object
	var methods []string
	for k := range ad.Permissions {
		methods = append(methods, k)
	}

	permissionItems := container.NewVBox()

	permissions := []string{
		xswd.Ask.String(),
		xswd.Allow.String(),
		xswd.Deny.String(),
		xswd.AlwaysAllow.String(),
		xswd.AlwaysDeny.String(),
	}

	if len(methods) > 0 {
		sort.Strings(methods)
		for _, name := range methods {
			permission := widget.NewSelect(permissions, nil)
			permission.SetSelected(ad.Permissions[name].String())
			permission.Disable()
			permissionItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### "+name), permission))
		}
	} else {
		permissionItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("No Permissions"), nil))
	}

	// Find RegisteredEvents for connected app and build UI object
	var events []rpc.EventType
	for k := range ad.RegisteredEvents {
		events = append(events, k)
	}

	eventItems := container.NewVBox()

	if len(events) > 0 {
		sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
		for _, name := range events {
			event := widget.NewSelect([]string{"false", "true"}, nil)
			event.SetSelected(strconv.FormatBool(ad.RegisteredEvents[name]))
			event.Disable()
			eventItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown(fmt.Sprintf("### %s", name)), event))
		}
	} else {
		eventItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("No Events"), nil))
	}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
	})

	image := canvas.NewImageFromResource(resourceWebsocketPng)
	image.SetMinSize(fyne.NewSize(ui.Width*0.25, ui.Width*0.25))
	image.FillMode = canvas.ImageFillContain

	// Check if the application is TELA
	telaURL := "http://localhost"
	if strings.HasPrefix(ad.Url, telaURL) {
		for _, serv := range getTelaActiveServers() {
			if strings.HasPrefix(ad.Url, telaURL+serv.Address) {
				name, _, icon, _, _ := getContractHeader(crypto.HashHexToHash(serv.SCID))
				if icon != "" {
					if img, err := handleImageURL(name, icon, fyne.NewSize(ui.Width*0.25, ui.Width*0.25)); err == nil {
						image = img
					} else {
						logger.Errorf("[Engram] Could not validate icon image: %s\n", err)
					}
				}

				break
			}
		}
	}

	linkURL := widget.NewHyperlinkWithStyle("Open in browser", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkURL.OnTapped = func() {
		link, err := url.Parse(ad.Url)
		if err != nil {
			logger.Errorf("[Engram] Error parsing XSWD application URL: %s\n", err)
			return
		}
		_ = fyne.CurrentApp().OpenURL(link)
	}

	btnRemove := widget.NewButton("Remove", nil)
	btnRemove.OnTapped = func() {
		if remoteAccess.WS.server != nil && len(remoteAccess.WS.apps) > 0 {
			remoteAccess.WS.server.RemoveApplication(ad)
			removeOverlays()
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutRemoteAccess())
		}
	}

	center := container.NewStack(
		rectBox,
		container.NewVScroll(
			container.NewStack(
				rectWidth90,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						container.NewHBox(
							layout.NewSpacer(),
							image,
							layout.NewSpacer(),
						),
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								labelName,
							),
							layout.NewSpacer(),
						),
						labelDesc,
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelID,
						textID,
						rectSpacer,
						rectSpacer,
						signatureItems,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelURL,
						rectSpacer,
						textURL,
						container.NewHBox(
							layout.NewSpacer(),
						),
						container.NewHBox(
							linkURL,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labelPermissions,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
						),
						permissionItems,
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelEvents,
						rectSpacer,
						eventItems,
						container.NewStack(
							rectWidth90,
						),
						rectSpacer,
						rectSpacer,
						labelSeparator6,
						rectSpacer,
						rectSpacer,
						wrapMobileButton(btnRemove),
						rectSpacer,
						rectSpacer,
					),
					layout.NewSpacer(),
				),
			),
		),
		rectSpacer,
		rectSpacer,
	)

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			top,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}

// Layout XSWD permissions settings
func layoutXSWDPermissions() fyne.CanvasObject {
	session.Domain = "app.remoteaccess.permissions"

	wSpacer := widget.NewLabel(" ")

	frame := &iframe{}

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, scaleSize(20)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	title := canvas.NewText("G L O B A L   P E R M I S S I O N S", colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	xswdLabel := canvas.NewText("W E B   S O C K E T S", colors.Gray)
	xswdLabel.TextSize = scaleFont(11)
	xswdLabel.Alignment = fyne.TextAlignCenter
	xswdLabel.TextStyle = fyne.TextStyle{Bold: true}

	labelMethods := canvas.NewText("  METHODS", colors.Gray)
	labelMethods.TextSize = scaleFont(14)
	labelMethods.Alignment = fyne.TextAlignLeading
	labelMethods.TextStyle = fyne.TextStyle{Bold: true}

	labelConnection := canvas.NewText("  CONNECTIONS", colors.Gray)
	labelConnection.TextSize = scaleFont(14)
	labelConnection.Alignment = fyne.TextAlignLeading
	labelConnection.TextStyle = fyne.TextStyle{Bold: true}

	labelEpoch := canvas.NewText("  EPOCH", colors.Gray)
	labelEpoch.TextSize = scaleFont(14)
	labelEpoch.Alignment = fyne.TextAlignLeading
	labelEpoch.TextStyle = fyne.TextStyle{Bold: true}

	permissionInfo := canvas.NewText("APPLY ON CONNECTION", colors.Gray)
	permissionInfo.TextSize = scaleFont(12)
	permissionInfo.Alignment = fyne.TextAlignCenter
	permissionInfo.TextStyle = fyne.TextStyle{Bold: true}

	btnDefaults := widget.NewButton("Restore Defaults", nil)

	wMode := widget.NewCheck("Restrictive Mode", nil)

	// Simple/Advanced Mode Toggle
	wSimpleMode := widget.NewCheck("Simple Mode (Recommended)", nil)
	wSimpleMode.Checked = IsSimpleMode()

	wConnection := widget.NewSelect([]string{xswd.Ask.String(), xswd.Allow.String()}, nil)

	wGlobalPermissions := widget.NewSelect([]string{"Off", "Apply"}, nil)

	wEpoch := widget.NewSelect([]string{xswd.Deny.String(), xswd.Allow.String()}, nil)

	wEpochAddress := widget.NewSelect([]string{"My Address", "dApp Chooses"}, nil)

	/*
		if remoteAccess.EPOCH.enabled {
			wEpoch.SetSelectedIndex(1)
		} else {
			wEpoch.SetSelectedIndex(0)
			wEpochAddress.Disable()
		}

		if remoteAccess.EPOCH.allowWithAddress {
			wEpochAddress.SetSelectedIndex(1)
		} else {
			wEpochAddress.SetSelectedIndex(0)
		}

		wEpoch.OnChanged = func(s string) {
			if s == xswd.Allow.String() {
				remoteAccess.EPOCH.enabled = true
				wEpochAddress.Enable()
				return
			}

			remoteAccess.EPOCH.enabled = false
			wEpochAddress.SetSelectedIndex(0)
			wEpochAddress.Disable()
		}

		wEpochAddress.OnChanged = func(s string) {
			if s == "dApp Chooses" {
				remoteAccess.EPOCH.allowWithAddress = true
				return
			}

			remoteAccess.EPOCH.allowWithAddress = false
		}
	*/

	spacerEpoch := canvas.NewRectangle(color.Transparent)
	spacerEpoch.SetMinSize(fyne.NewSize(140, 0))

	entryEpochWork := widget.NewEntry()
	entryEpochWork.SetPlaceHolder(":10100")
	entryEpochWork.SetText(epoch.GetPort())
	entryEpochWork.Validator = func(s string) (err error) {
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid port")
		}

		return epoch.SetPort(i)
	}

	entryEpochHash := widget.NewEntry()
	entryEpochHash.SetPlaceHolder("Max hashes")
	entryEpochHash.SetText(strconv.Itoa(epoch.GetMaxHashes()))
	entryEpochHash.Validator = func(s string) (err error) {
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid hash value")
		}

		return epoch.SetMaxHashes(i)
	}

	wEpochPower := widget.NewSelect([]string{"Less", "More"}, nil)
	wEpochPower.SetSelectedIndex(0)
	if epoch.GetMaxThreads() > 2 {
		wEpochPower.SetSelectedIndex(1)
	}

	wEpochPower.OnChanged = func(s string) {
		if s == "More" {
			half := runtime.NumCPU() / 2
			if half > epoch.DEFAULT_MAX_THREADS {
				epoch.SetMaxThreads(half)
			}

			return
		}

		epoch.SetMaxThreads(epoch.DEFAULT_MAX_THREADS)
	}

	if session.Offline {
		wMode.Disable()
		wEpoch.Disable()
		wEpochAddress.Disable()
		entryEpochWork.Disable()
		entryEpochHash.Disable()
		wEpochPower.Disable()
	} else if remoteAccess.WS.server != nil {
		wEpoch.Disable()
		wEpochAddress.Disable()
		entryEpochWork.Disable()
		entryEpochHash.Disable()
		wEpochPower.Disable()
	}

	if remoteAccess.WS.advanced {
		wMode.SetChecked(false)
		if remoteAccess.WS.global.enabled {
			wGlobalPermissions.SetSelectedIndex(1)
			if remoteAccess.WS.global.connect {
				wConnection.SetSelectedIndex(1)
			} else {
				wConnection.SetSelectedIndex(0)
			}
		} else {
			wGlobalPermissions.SetSelectedIndex(0)
			wConnection.SetSelectedIndex(0)
			wConnection.Disable()
			btnDefaults.Disable()
		}
	} else {
		wMode.SetChecked(false)
		wConnection.SetSelectedIndex(0)
		wConnection.Disable()
		wGlobalPermissions.SetSelectedIndex(0)
		wGlobalPermissions.Disable()
		btnDefaults.Disable()
	}

	wMode.OnChanged = func(b bool) {
		remoteAccess.WS.advanced = !b // inverse as check box is for restrictive mode on/off
		if remoteAccess.WS.advanced {
			wGlobalPermissions.Enable()
		} else {
			wGlobalPermissions.SetSelectedIndex(0) // calling this here resets and disables wConnection
			wGlobalPermissions.Disable()
		}
	}

	wConnection.OnChanged = func(s string) {
		if s == xswd.Allow.String() {
			remoteAccess.WS.global.connect = true
		} else {
			remoteAccess.WS.global.connect = false
		}
	}

	formItems := container.NewVBox()

	// Permission options for select widgets
	permissions := []string{
		xswd.Ask.String(),
		xswd.AlwaysAllow.String(),
		xswd.AlwaysDeny.String(),
	}

	noStorePermissions := []string{
		xswd.Ask.String(),
		xswd.AlwaysDeny.String(),
	}

	// onChanged handler for Advanced Mode individual permissions
	onChanged := func(n string) func(s string) {
		return func(s string) {
			remoteAccess.WS.Lock()
			defer remoteAccess.WS.Unlock()

			switch s {
			case xswd.Ask.String():
				remoteAccess.WS.global.permissions[n] = xswd.Ask
			case xswd.AlwaysAllow.String():
				remoteAccess.WS.global.permissions[n] = xswd.AlwaysAllow
			case xswd.AlwaysDeny.String():
				remoteAccess.WS.global.permissions[n] = xswd.AlwaysDeny
			default:
				remoteAccess.WS.global.permissions[n] = xswd.Ask
			}

			// Save updated permissions to storage
			setPermissions()
		}
	}

	// Build Simple Mode UI (6 grouped permissions)
	buildSimpleUI := func() {
		formItems.Objects = []fyne.CanvasObject{}

		for _, group := range permissionGroups {
			if !group.SimpleMode {
				continue // Skip hidden groups
			}

			// Group header with description
			header := widget.NewRichTextFromMarkdown("### " + group.Name)
			desc := canvas.NewText(group.Description, colors.Gray)
			desc.TextSize = scaleFont(11)

			// Permission selector
			permSelect := widget.NewSelect(permissions, nil)

			// Set current value from storage
			currentPerm := GetGroupPermission(group.Name)
			permSelect.SetSelected(currentPerm.String())

			// Disable if WebSocket is not enabled
			if !remoteAccess.WS.global.enabled {
				permSelect.SetSelectedIndex(0)
				permSelect.Disable()
			}

			// OnChanged handler
			permSelect.OnChanged = func(g string) func(s string) {
				return func(s string) {
					var perm xswd.Permission
					switch s {
					case xswd.AlwaysAllow.String():
						perm = xswd.AlwaysAllow
					case xswd.AlwaysDeny.String():
						perm = xswd.AlwaysDeny
					default:
						perm = xswd.Ask
					}
					SetGroupPermission(g, perm)
					logger.Printf("[Engram] Set group '%s' permission to %s", g, s)
				}
			}(group.Name)

			// Add to form
			groupContainer := container.NewVBox(
				header,
				desc,
				permSelect,
				rectSpacer,
			)
			formItems.Add(groupContainer)
		}
	}

	// Build Advanced Mode UI (all individual permissions)
	buildAdvancedUI := func() {
		formItems.Objects = []fyne.CanvasObject{}

		stored, methods := getPermissions()
		for _, name := range methods {
			n := name
			permission := widget.NewSelect([]string{}, nil)
			if engramCanStoreMethod(n) {
				permission.SetOptions(permissions)
			} else {
				permission.SetOptions(noStorePermissions)
			}

			if remoteAccess.WS.global.enabled {
				permission.SetSelected(stored[n].String())
				permission.OnChanged = onChanged(n)
			} else {
				permission.SetSelectedIndex(0)
				permission.Disable()
			}
			formItems.Add(container.NewBorder(nil, nil, widget.NewRichTextFromMarkdown("### "+n), permission))
		}
	}

	// Simple/Advanced Mode Toggle Handler
	wSimpleMode.OnChanged = func(checked bool) {
		SetSimpleMode(checked)
		if checked {
			buildSimpleUI()
		} else {
			buildAdvancedUI()
		}
		formItems.Refresh()
		logger.Printf("[Engram] Switched to %s mode", map[bool]string{true: "Simple", false: "Advanced"}[checked])
	}

	// Build initial UI based on current mode
	if IsSimpleMode() {
		buildSimpleUI()
	} else {
		buildAdvancedUI()
	}

	statusText := "Disabled"
	statusColor := colors.Gray
	if remoteAccess.WS.global.enabled {
		statusText = "Enabled"
		statusColor = colors.Green
	}

	remoteAccess.WS.global.status = canvas.NewText(statusText, statusColor)
	remoteAccess.WS.global.status.TextSize = scaleFont(22)
	remoteAccess.WS.global.status.TextStyle = fyne.TextStyle{Bold: true}

	btnDefaults.OnTapped = func() {
		if !remoteAccess.WS.global.enabled {
			return
		}

		header := canvas.NewText("RESTORE  DEFAULT  PERMISSIONS", colors.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText("Are you sure?", colors.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkCancel := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkCancel.OnTapped = func() {
			removeOverlays()
		}

		btnSubmit := widget.NewButton("Restore Defaults", nil)
		btnSubmit.OnTapped = func() {
			wConnection.SetSelectedIndex(0)

			if IsSimpleMode() {
				// Restore Simple Mode defaults
				for _, group := range permissionGroups {
					if !group.SimpleMode {
						continue
					}
					defaultPerm := getSimpleDefault(group.Category)
					SetGroupPermission(group.Name, defaultPerm)
				}
				// Rebuild UI to show defaults
				buildSimpleUI()
			} else {
				// Restore Advanced Mode defaults
				remoteAccess.WS.Lock()
				remoteAccess.WS.global.permissions = SetDefaultPermissions()
				remoteAccess.WS.Unlock()
				setPermissions()
				// Rebuild UI
				buildAdvancedUI()
			}

			formItems.Refresh()
			removeOverlays()
			logger.Printf("[Engram] Restored default permissions in %s mode", map[bool]string{true: "Simple", false: "Advanced"}[IsSimpleMode()])
		}

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay := session.Window.Canvas().Overlays()

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(colors.DarkMatter),
			),
		)

		overlay.Add(
			container.NewStack(
				&iframe{},
				container.NewCenter(
					container.NewVBox(
						span,
						container.NewCenter(
							header,
						),
						rectSpacer,
						rectSpacer,
						subHeader,
						widget.NewLabel(""),
						wrapMobileButton(btnSubmit),
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkCancel,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)
	}

	wGlobalPermissions.OnChanged = func(s string) {
		if s != "Apply" {
			setPermissions()
			btnDefaults.Disable()
			remoteAccess.WS.global.status.Text = "Disabled"
			remoteAccess.WS.global.status.Color = colors.Gray
			remoteAccess.WS.global.status.Refresh()
			remoteAccess.WS.global.enabled = false
			wConnection.SetSelectedIndex(0)
			wConnection.Disable()

			// Disable all select widgets in formItems (works for both Simple and Advanced modes)
			for _, obj := range formItems.Objects {
				if container, ok := obj.(*fyne.Container); ok {
					for _, child := range container.Objects {
						if selectWidget, ok := child.(*widget.Select); ok {
							selectWidget.OnChanged = nil
							selectWidget.SetSelectedIndex(0)
							selectWidget.Disable()
						}
					}
				}
			}
		} else {
			remoteAccess.WS.global.status.Text = "Enabled"
			remoteAccess.WS.global.status.Color = colors.Green
			remoteAccess.WS.global.status.Refresh()
			remoteAccess.WS.global.enabled = true
			wConnection.Enable()
			btnDefaults.Enable()

			go func() {
				if IsSimpleMode() {
					// Rebuild Simple Mode UI with enabled selects
					fyne.Do(func() {
						buildSimpleUI()
						formItems.Refresh()
					})
				} else {
					// Rebuild Advanced Mode UI with enabled selects
					fyne.Do(func() {
						buildAdvancedUI()
						formItems.Refresh()
					})
				}
			}()
		}
	}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(colors.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		setPermissions()
		removeOverlays()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
	})

	// Initialized in layoutRemoteAccess()
	remoteAccess.WS.portText.SetText(getRemoteAccess("WS"))

	center := container.NewVScroll(
		container.NewStack(
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewVBox(
						title,
						rectSpacer,
					),
				),
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					xswdLabel,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
				container.NewCenter(
					container.NewVBox(
						rectWidth90,
						rectSpacer,
						container.NewCenter(
							remoteAccess.WS.global.status,
						),
						rectSpacer,
						container.NewCenter(
							permissionInfo,
						),
					),
				),
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewCenter(
						container.NewVBox(
							container.NewBorder(
								nil,
								nil,
								nil,
								nil,
								container.NewCenter(wMode),
							),
							rectSpacer,
							remoteAccess.WS.portText,
							rectSpacer,
							labelConnection,
							rectSpacer,
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Type"),
								wConnection,
							),
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Global Permissions"),
								wGlobalPermissions,
							),
							wSpacer,
							labelEpoch,
							rectSpacer,
							/*
								container.NewBorder(
									nil,
									nil,
									widget.NewRichTextFromMarkdown("### Preference"),
									wEpoch,
								),
								container.NewBorder(
									nil,
									nil,
									widget.NewRichTextFromMarkdown("### Reward Address"),
									wEpochAddress,
								),
							*/
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Get Work"),
								container.NewHBox(
									layout.NewSpacer(),
									container.NewStack(
										spacerEpoch,
										entryEpochWork,
									),
								),
							),
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Max Hashes"),
								container.NewHBox(
									layout.NewSpacer(),
									container.NewStack(
										spacerEpoch,
										entryEpochHash,
									),
								),
							),
							container.NewBorder(
								nil,
								nil,
								widget.NewRichTextFromMarkdown("### Power"),
								wEpochPower,
							),
							wSpacer,
							labelMethods,
							rectSpacer,
							container.NewCenter(wSimpleMode),
							rectSpacer,
							container.NewCenter(
								formItems,
							),
							wSpacer,
						),
					),
					layout.NewSpacer(),
				),
				container.NewCenter(
					container.NewVBox(
						wrapMobileButton(btnDefaults),
						rectWidth90,
					),
				),
				wSpacer,
			),
		),
	)
	center.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1),
					btnBack,
				),
			),
			rectSpacer,
		),
	)

	layout := container.NewStack(
		frame,
		container.NewBorder(
			nil,
			bottom,
			nil,
			nil,
			center,
		),
	)

	return NewVScroll(layout)
}
