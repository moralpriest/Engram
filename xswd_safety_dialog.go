package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
	"github.com/deroproject/derohe/walletapi/xswd"
)

var openBlockDialog = showXSWDBlockDialog

func showXSWDBlockDialog(ad *xswd.ApplicationData, method, reason, detail string, onRevoke func()) {
	if session.Window == nil {
		return
	}
	overlay := session.Window.Canvas().Overlays()

	// Reference-based overlay layers: removal targets the exact layers this
	// dialog added, so it can never pop another dialog's widgets.
	var dialogLayers [2]fyne.CanvasObject

	appName := ""
	if ad != nil {
		appName = ad.Name
	}

	header := canvas.NewText(i18n.T("xswdsafety.blocked_title_prefix")+": "+reason, apptheme.C.Gray)
	header.TextSize = 18
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}
	header.Resize(fyne.NewSize(440, header.MinSize().Height))

	appVal := widget.NewLabel(appName)
	appVal.Truncation = fyne.TextTruncateEllipsis
	methodVal := widget.NewLabel(method)
	methodVal.Truncation = fyne.TextTruncateEllipsis
	detailVal := widget.NewLabel(detail)
	detailVal.Wrapping = fyne.TextWrapWord
	detailVal.Truncation = fyne.TextTruncateEllipsis

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: i18n.T("xswdsafety.app_label"), Widget: appVal},
			{Text: i18n.T("xswdsafety.method_label"), Widget: methodVal},
			{Text: i18n.T("xswdsafety.detail_label"), Widget: detailVal},
		},
	}
	form.Resize(fyne.NewSize(440, form.MinSize().Height))

	explain := widget.NewLabel(i18n.T("xswdsafety.explanation"))
	explain.Wrapping = fyne.TextWrapWord
	explain.Resize(fyne.NewSize(440, explain.MinSize().Height))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 12))

	btnDismiss := widget.NewButtonWithIcon(i18n.T("xswdsafety.dismiss"), theme.ConfirmIcon(), nil)
	btnDismiss.Importance = widget.WarningImportance

	linkRevoke := widget.NewHyperlinkWithStyle(i18n.T("xswdsafety.revoke_link"), nil,
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRevoke.Truncation = fyne.TextTruncateEllipsis

	body := container.NewVBox(
		header,
		rectSpacer,
		form,
		rectSpacer,
		explain,
		rectSpacer,
		container.NewHBox(layout.NewSpacer(), btnDismiss, layout.NewSpacer(), linkRevoke, layout.NewSpacer()),
		rectSpacer,
	)

	dismiss := func() {
		fyne.Do(func() {
			overlay.Remove(dialogLayers[0])
			overlay.Remove(dialogLayers[1])
		})
	}
	revoke := func() {
		dismiss()
		if onRevoke != nil {
			onRevoke()
		}
	}

	btnDismiss.OnTapped = dismiss
	linkRevoke.OnTapped = revoke

	fyne.Do(func() {
		backdrop := container.NewStack(
			&iframe{},
			canvas.NewRectangle(apptheme.C.DarkMatter),
		)
		dialog := container.NewStack(
			&iframe{},
			container.NewCenter(body),
		)
		dialogLayers[0] = backdrop
		dialogLayers[1] = dialog
		overlay.Add(backdrop)
		overlay.Add(dialog)
		session.Window.RequestFocus()
	})
}
