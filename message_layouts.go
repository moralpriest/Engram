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
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/DEROFDN/engram/i18n"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/transaction"
	"github.com/deroproject/derohe/walletapi"
)

func showWarningPopup(messageKey string) {
	if session.Window == nil {
		return
	}

	header := canvas.NewText(i18n.T("messages.note_title"), colors.Yellow)
	header.TextSize = 18
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	messageText := i18n.T(messageKey)
	message := widget.NewLabel(messageText)
	message.Alignment = fyne.TextAlignCenter
	message.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer1 := canvas.NewRectangle(color.Transparent)
	rectSpacer1.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer2 := canvas.NewRectangle(color.Transparent)
	rectSpacer2.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer3 := canvas.NewRectangle(color.Transparent)
	rectSpacer3.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer4 := canvas.NewRectangle(color.Transparent)
	rectSpacer4.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer5 := canvas.NewRectangle(color.Transparent)
	rectSpacer5.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer6 := canvas.NewRectangle(color.Transparent)
	rectSpacer6.SetMinSize(fyne.NewSize(0, 10))
	rectSpacer7 := canvas.NewRectangle(color.Transparent)
	rectSpacer7.SetMinSize(fyne.NewSize(0, 10))

	var overlay *fyne.Container
	var blocker *fyne.Container

	btnOk := widget.NewButton(i18n.T("messages.ok"), func() {
		uiDo(func() {
			overlays := session.Window.Canvas().Overlays()
			overlays.Remove(overlay)
			overlays.Remove(blocker)
		})
	})
	btnOk.Importance = widget.MediumImportance

	btnRow := container.NewHBox(layout.NewSpacer(), btnOk, layout.NewSpacer())

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer1,
				rectSpacer2,
				btnRow,
				rectSpacer3,
				rectSpacer4,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						message,
						rectSpacer5,
					),
				),
			),
		),
	)

	span1 := canvas.NewRectangle(color.Transparent)
	span1.SetMinSize(fyne.NewSize(ui.Width, 10))

	blocker = container.NewStack(
		&iframe{},
		canvas.NewRectangle(colors.DarkMatter),
	)

	overlay = container.NewStack(
		&iframe{},
		container.NewCenter(
			container.NewVBox(
				span1,
				container.NewCenter(
					header,
				),
				rectSpacer6,
				rectSpacer7,
				content,
			),
		),
	)

	uiDo(func() {
		overlays := session.Window.Canvas().Overlays()
		overlays.Add(blocker)
		overlays.Add(overlay)
	})
}

func layoutMessages() fyne.CanvasObject {
	session.Domain = "app.messages"

	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}

	if !isDaemonConnected() {
		session.Window.SetContent(layoutSettings())
	}

	if !session.MessageWarningShown {
		session.MessageWarningShown = true
		showWarningPopup("messages.warning_daemon")
	}

	title := canvas.NewText(i18n.T("messages.contacts"), colors.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width*0.95, 10))

	// Move definitions up
	contactInput := widget.NewEntry()
	contactInput.PlaceHolder = i18n.T("messages.search_username")
	contactInput.SetIcon(theme.SearchIcon())

	btnSend := widget.NewButton(i18n.T("messages.new_message"), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutPM())
		removeOverlays()
	})
	btnSend.Disable()

	rebuildBtn := widget.NewButton(i18n.T("messages.rebuild"), func() {
		rebuildMessageHistory()
	})

	checkLimit := widget.NewCheck(i18n.T("messages.recent"), nil)
	checkLimit.OnChanged = func(b bool) {
		if b {
			session.LimitMessages = true
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		} else {
			session.LimitMessages = false
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		}
	}

	if session.LimitMessages {
		checkLimit.Checked = true
	}

	top := container.NewVBox(
		canvas.NewRectangle(color.Transparent),
		container.NewCenter(
			title,
		),
		canvas.NewRectangle(color.Transparent),
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectWidth90,
				container.NewVBox(
					contactInput,
					canvas.NewRectangle(color.Transparent),
					wrapMobileButton(btnSend),
					canvas.NewRectangle(color.Transparent),
					wrapMobileButton(rebuildBtn),
					canvas.NewRectangle(color.Transparent),
					container.NewCenter(checkLimit),
				),
			),
			layout.NewSpacer(),
		),
		canvas.NewRectangle(color.Transparent),
	)

	// Set spacer sizes
	for _, obj := range top.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}
	// Also set sizes for spacers inside the nested VBox
	if hbox, ok := top.Objects[2].(*fyne.Container); ok {
		if stack, ok := hbox.Objects[1].(*fyne.Container); ok {
			if vbox, ok := stack.Objects[1].(*fyne.Container); ok {
				for _, obj := range vbox.Objects {
					if r, ok := obj.(*canvas.Rectangle); ok {
						r.SetMinSize(standardSpacerSize())
					}
				}
			}
		}
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rectEmpty := canvas.NewRectangle(color.Transparent)
	rectEmpty.SetMinSize(statusDotSize())
	frame := &iframe{}
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	listMinFrac := float32(0.43)
	if !isMobile() {
		listMinFrac = 0.36
	}
	rectListBox.SetMinSize(fyne.NewSize(ui.Width, ui.Height*listMinFrac))

	messages.Data = nil

	var height uint64

	if session.LimitMessages {
		height = engram.Disk.Get_Height() - 1000000
	} else {
		height = 0
	}

	threadSummaries := getMessageThreadSnapshot()
	data := []string{}
	if len(threadSummaries) > 0 {
		for _, thread := range threadSummaries {
			label := thread.Label
			if label == "" {
				label = resolveAddressDisplay(thread.ContactKey)
			}
			if label == "" && thread.ContactKey == "" {
				continue
			}
			data = append(data, thread.ContactKey+"~~~"+label)
		}
	}
	if len(data) == 0 {
		data = getMessages(height)
	}
	temp := data

	list := binding.BindStringList(&data)

	msgbox.List = widget.NewListWithData(list,
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}
			dataItem := strings.SplitN(str, "~~~", 4)
			if len(dataItem) < 2 {
				return
			}
			short := dataItem[0]
			address := short
			if len(short) > DEFAULT_USERADDR_SHORTEN_LENGTH {
				address = short[len(short)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
			}
			username := dataItem[1]
			if username == "" {
				username = resolveAddressDisplay(dataItem[0])
			}
			// If a username is longer than what *would* be a 'short' address of ...xyzxyzxyzx (e.g. 13), then shorten as well to be similar sizing
			if len(username) > DEFAULT_USERADDR_SHORTEN_LENGTH+3 {
				username = "..." + username[len(username)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
			}

			label := co.(*widget.Label)
			if username == "" {
				label.SetText("..." + address)
			} else {
				label.SetText(username)
			}
			label.Wrapping = fyne.TextWrapWord
			label.TextStyle.Bold = false
			label.Alignment = fyne.TextAlignLeading
		})

	msgbox.List.OnSelected = func(id widget.ListItemID) {
		msgbox.List.UnselectAll()
		split := strings.Split(data[id], "~~~")
		if len(split) < 2 {
			return
		}
		if split[1] == "" {
			messages.Contact = split[0]
		} else {
			messages.Contact = split[1]
		}

		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutPM())
		removeOverlays()
	}

	filterContacts := func(query string) {
		query = strings.ToLower(strings.TrimSpace(query))
		searchList := []string{}
		if query == "" {
			_ = list.Set(temp)
			return
		}

		for _, d := range temp {
			tempd := strings.ToLower(d)
			split := strings.SplitN(tempd, "~~~", 4)
			if len(split) < 2 {
				continue
			}

			if strings.Contains(split[0], query) || strings.Contains(split[1], query) {
				searchList = append(searchList, d)
			}
		}

		_ = list.Set(searchList)
	}

	contactInput.OnChanged = func(s string) {
		filterContacts(s)
		val := strings.TrimSpace(s)
		messages.Contact = val

		if val == "" {
			btnSend.Disable()
			return
		}

		if _, err := globals.ParseValidateAddress(val); err == nil {
			btnSend.Enable()
			return
		}

		btnSend.Disable()

		go func(check string) {
			if _, err := checkUsername(check, -1); err == nil {
				uiDo(func() {
					// Only enable if the user hasn't changed the input in the meantime
					if strings.TrimSpace(contactInput.Text) == check {
						btnSend.Enable()
					}
				})
			}
		}(val)
	}

	features := container.NewHBox(
		layout.NewSpacer(),
		container.NewStack(
			rectWidth90,
			rectListBox,
			msgbox.List,
		),
		layout.NewSpacer(),
	)

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.messages" {
			return
		}

		if k.Name == fyne.KeyUp {
			session.Dashboard = "main"

			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
			removeOverlays()
		} else if k.Name == fyne.KeyF5 {
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		}
	})

	subContainer := container.NewStack(
		container.NewVBox(
			canvas.NewRectangle(color.Transparent),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			canvas.NewRectangle(color.Transparent),
		),
	)

	// Set spacer sizes for subContainer
	if vbox, ok := subContainer.Objects[0].(*fyne.Container); ok {
		for _, obj := range vbox.Objects {
			if r, ok := obj.(*canvas.Rectangle); ok {
				r.SetMinSize(standardSpacerSize())
			}
		}
	}

	c := container.NewBorder(
		top,
		subContainer,
		nil,
		nil,
		features,
	)

	mainLayout := container.NewStack(
		frame,
		c,
	)

	return mainLayout
}

func layoutPM() fyne.CanvasObject {
	session.Domain = "app.messages.contact"

	if !isMobile() {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}

	if !isDaemonConnected() {
		session.Window.SetContent(layoutSettings())
	}

	getPrimaryUsername()

	selectedKey, selectedLabel := resolveMessageContact(messages.Contact, -1)
	contactAddress := messages.Contact
	if selectedLabel != "" {
		contactAddress = selectedLabel
	} else if display := resolveAddressDisplay(selectedKey); display != "" {
		contactAddress = display
	} else if display := resolveAddressDisplay(strings.TrimSpace(messages.Contact)); display != "" {
		contactAddress = display
	}

	if len(contactAddress) > DEFAULT_USERADDR_SHORTEN_LENGTH+3 {
		short := contactAddress[len(contactAddress)-DEFAULT_USERADDR_SHORTEN_LENGTH:]
		contactAddress = "..." + short
	}

	heading := canvas.NewText(contactAddress, colors.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	lastActive := canvas.NewText("", colors.Gray)
	lastActive.TextSize = scaleFont(12)
	lastActive.Alignment = fyne.TextAlignCenter
	lastActive.TextStyle = fyne.TextStyle{Bold: false}

	backFromThread := func() {
		prev := session.LastDomain
		prevDom := session.PreviousDomain
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		if prevDom != "" && prev != nil {
			session.Window.SetContent(prev)
			session.Domain = prevDom
			session.PreviousDomain = ""
		} else {
			session.Window.SetContent(layoutMessages())
		}
		removeOverlays()
	}
	btnBack := newSizedIconButton(theme.NavigateBackIcon(), backFromThread)

	rectStatus := canvas.NewRectangle(color.Transparent)
	rectStatus.SetMinSize(statusDotSize())
	rectEmpty := canvas.NewRectangle(color.Transparent)
	rectEmpty.SetMinSize(statusDotSize())
	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(statusDotSize())
	frame := &iframe{}
	subframe := canvas.NewRectangle(color.Transparent)
	if isMobile() {
		subframe.SetMinSize(fyne.NewSize(ui.Width, scaleSize(48)))
	} else {
		subframe.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.36))
	}
	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, scaleSize(35)))
	rectListBox := canvas.NewRectangle(color.Transparent)
	rectListBox.SetMinSize(fyne.NewSize(ui.Width*0.42, 30))
	rectOutbound := canvas.NewRectangle(color.Transparent)
	rectOutbound.SetMinSize(fyne.NewSize(ui.Width*0.166, 30))

	messages.Data = nil

	chats := container.NewVBox()

	chatFrame := container.NewStack(
		rectListBox,
		container.NewStack(
			chats,
		),
	)

	chatbox := container.NewVScroll(
		container.NewStack(
			chatFrame,
		),
	)

	var e *fyne.Container
	var height uint64

	if session.LimitMessages {
		height = engram.Disk.Get_Height() - 1000000
	} else {
		height = 0
	}

	messageRecords := getCachedThreadMessages(messages.Contact, height)
	if len(messageRecords) == 0 {
		records := getMessageCacheSnapshot()
		if len(records) == 0 {
			records = scanMessageTransfers(height)
		}
		for _, message := range records {
			if height > 0 && message.Entry.Height < height {
				continue
			}
			if messageMatchesContact(message, messages.Contact) {
				messageRecords = append(messageRecords, message)
			}
		}
	}
	originalMessageRecords := make([]MessageRecord, len(messageRecords))
	copy(originalMessageRecords, messageRecords)
	renderThread := func(filtered []MessageRecord) {
		messages.Data = nil
		chats.Objects = nil
		if len(filtered) == 0 {
			empty := widget.NewLabel(i18n.T("messages.no_messages_found"))
			empty.Alignment = fyne.TextAlignCenter
			chats.Add(empty)
			chats.Refresh()
			chatbox.Refresh()
			return
		}

		renderedMessages := make([]RenderedThreadMessage, 0, len(filtered))
		useCachedRender := len(filtered) == len(originalMessageRecords)
		if useCachedRender {
			if cached, ok := getRenderedThreadCache(messages.Contact, height); ok {
				renderedMessages = cached
			} else {
				useCachedRender = false
			}
		}

		if !useCachedRender {
			for d := range filtered {
				if filtered[d].Entry.Incoming {
					replyback := messageReplyback(filtered[d].Entry)
					if replyback != "" {
						t := filtered[d].Entry.Time
						time := string(t.Format(time.RFC822))
						comment := filtered[d].Comment
						links := getTextURL(comment)

						for i := range links {
							if comment == links[i] {
								if len(links[i]) > 25 {
									comment = `[ ` + links[i][0:25] + "..." + ` ](` + links[i] + `)`
								} else {
									comment = `[ ` + links[i] + ` ](` + links[i] + `)`
								}
							} else {
								linkText := ""
								split := strings.Split(comment, links[i])
								if len(links[i]) > 25 {
									linkText = links[i][0:25] + "..."
								} else {
									linkText = links[i]
								}
								comment = `` + split[0] + `[link]` + split[1] + "\n\n›" + `[ ` + linkText + ` ](` + links[i] + `)`
							}
						}
						renderedMessages = append(renderedMessages, RenderedThreadMessage{Sender: replyback, Comment: comment, Timestamp: time, IsIncoming: true})
					}
				} else {
					t := filtered[d].Entry.Time
					time := string(t.Format(time.RFC822))
					comment := filtered[d].Comment
					links := getTextURL(comment)

					for i := range links {
						if comment == links[i] {
							if len(links[i]) > 25 {
								comment = `[ ` + links[i][0:25] + "..." + ` ](` + links[i] + `)`
							} else {
								comment = `[ ` + links[i] + ` ](` + links[i] + `)`
							}
						} else {
							linkText := ""
							split := strings.Split(comment, links[i])
							if len(links[i]) > 25 {
								linkText = links[i][0:25] + "..."
							} else {
								linkText = links[i]
							}
							comment = `` + split[0] + `[link]` + split[1] + "\n\n›" + `[ ` + linkText + ` ](` + links[i] + `)`
						}
					}
					renderedMessages = append(renderedMessages, RenderedThreadMessage{Sender: engram.Disk.GetAddress().String(), Comment: comment, Timestamp: time, IsIncoming: false})
				}
			}
			if len(filtered) == len(originalMessageRecords) {
				setRenderedThreadCache(messages.Contact, height, renderedMessages)
			}
		}

		if len(renderedMessages) > 0 {
			newObjects := make([]fyne.CanvasObject, 0, len(renderedMessages))
			newData := make([]string, 0, len(renderedMessages))
			for _, rendered := range renderedMessages {
				mdata := widget.NewRichTextFromMarkdown("")
				mdata.Wrapping = fyne.TextWrapWord
				datetime := canvas.NewText("", colors.Green)
				datetime.TextSize = scaleFont(11)
				boxColor := colors.Flint
				rect := canvas.NewRectangle(boxColor)
				rect.SetMinSize(fyne.NewSize(ui.Width*0.80, 30))
				rect.CornerRadius = scaleSize(5)
				rect5 := canvas.NewRectangle(color.Transparent)
				rect5.SetMinSize(smallSpacerSize())

				if !rendered.IsIncoming {
					rect.FillColor = colors.DarkGreen
					mdata.ParseMarkdown(rendered.Comment)
					datetime.Text = rendered.Timestamp
					e = container.NewBorder(
						nil,
						container.NewVBox(
							container.NewHBox(
								layout.NewSpacer(),
								datetime,
								rect5,
							),
							rect5,
						),
						rectOutbound,
						container.NewStack(
							rect,
							container.NewVBox(
								mdata,
							),
						),
					)
				} else {
					rect.FillColor = colors.Flint
					mdata.ParseMarkdown(rendered.Comment)
					datetime.Text = rendered.Timestamp
					e = container.NewBorder(
						nil,
						container.NewVBox(
							container.NewHBox(
								rect5,
								datetime,
								layout.NewSpacer(),
							),
							rect5,
						),
						container.NewStack(
							rect,
							container.NewVBox(
								mdata,
							),
						),
						rectOutbound,
					)
				}

				newData = append(newData, rendered.Sender+";;;;"+rendered.Comment+";;;;"+rendered.Timestamp)
				newObjects = append(newObjects, e)
			}
			messages.Data = newData
			chats.Objects = newObjects
			lastActive.Text = "Last Updated:  " + time.Now().Format(time.RFC822)
			lastActive.Refresh()
			chats.Refresh()
			chatbox.Refresh()
			chatbox.ScrollToBottom()
		}
	}

	renderThread(messageRecords)
	threadSearch := widget.NewEntry()
	threadSearch.PlaceHolder = i18n.T("messages.search_thread")
	threadSearch.OnChanged = func(s string) {
		query := strings.ToLower(strings.TrimSpace(s))
		if query == "" {
			renderThread(originalMessageRecords)
			return
		}

		filtered := make([]MessageRecord, 0)
		for _, message := range originalMessageRecords {
			if strings.Contains(strings.ToLower(message.Comment), query) {
				filtered = append(filtered, message)
			}
		}

		renderThread(filtered)
	}

	btnSend := widget.NewButton(i18n.T("messages.send"), nil)
	btnSend.Disable()
	labelLimit := canvas.NewText("", colors.Gray)
	labelLimit.TextSize = scaleFont(11)
	labelLimit.Alignment = fyne.TextAlignLeading
	updateMessageLimit := func(message string, sender string) {
		if sender == "" && engram.Disk != nil {
			sender = engram.Disk.GetAddress().String()
		}

		args := rpc.Arguments{
			{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
			{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: uint64(1)},
			{Name: rpc.RPC_EXPIRY, DataType: rpc.DataTime, Value: time.Now().Add(time.Hour).UTC()},
			{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: message},
			{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: sender},
		}

		packed, err := args.MarshalBinary()
		if err != nil {
			labelLimit.Text = fmt.Sprintf("%d chars", len(message))
			labelLimit.Refresh()
			return
		}

		remaining := transaction.PAYLOAD0_LIMIT - len(packed)
		if remaining < 0 {
			remaining = 0
		}

		labelLimit.Text = fmt.Sprintf("%d chars, ~%d bytes left", len(message), remaining)
		labelLimit.Refresh()
	}

	var entry *mobileEntry
	entry = NewMobileEntry()
	entry.MultiLine = false
	entry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	entry.SetPlaceHolder(i18n.T("messages.message"))
	updateMessageLimit("", session.Username)
	entry.OnChanged = func(s string) {
		messages.Message = s
		contact := messages.Contact
		//check, err := engram.Disk.NameToAddress(messages.Contact)
		check, err := checkUsername(messages.Contact, -1)
		if err == nil {
			contact = check
		}

		_, err = globals.ParseValidateAddress(contact)
		if err != nil {
			session.LastDomain = session.Window.Content()
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
			return
		}
		updateMessageLimit(messages.Message, session.Username)

		err = checkMessagePack(messages.Message, session.Username, contact)
		if err != nil {
			btnSend.Text = i18n.T("messages.too_long")
			btnSend.Disable()
			btnSend.Refresh()
			return
		} else {
			if messages.Message == "" {
				btnSend.Text = i18n.T("messages.send")
				btnSend.Disable()
				btnSend.Refresh()
			} else {
				btnSend.Text = i18n.T("messages.send")
				btnSend.Enable()
				btnSend.Refresh()
			}
		}
	}

	btnSend.OnTapped = func() {
		if messages.Message == "" {
			return
		}
		contact := ""
		_, err := globals.ParseValidateAddress(messages.Contact)
		if err != nil {
			//check, err := engram.Disk.NameToAddress(messages.Contact)
			check, err := checkUsername(messages.Contact, -1)
			if err != nil {
				logger.Errorf("[Message] Failed to send: %s\n", err)
				btnSend.Text = i18n.T("messages.invalid_address")
				btnSend.Disable()
				btnSend.Refresh()
				return
			}
			contact = check
		} else {
			contact = messages.Contact
		}

		btnSend.Text = i18n.T("messages.setting_up")
		btnSend.Disable()
		btnSend.Refresh()

		txid, err := sendMessage(messages.Message, session.Username, contact)
		if err != nil {
			logger.Errorf("[Message] Failed to send: %s\n", err)
			btnSend.Text = i18n.T("messages.failed_send")
			btnSend.Disable()
			btnSend.Refresh()
			return
		}

		logger.Printf("[Message] Dispatched transaction successfully to: %s\n", messages.Contact)
		btnSend.Text = i18n.T("messages.confirming")
		btnSend.Disable()
		btnSend.Refresh()
		sentMessage := messages.Message
		messages.Message = ""
		entry.Text = ""
		entry.Refresh()
		updateMessageLimit("", session.Username)

		go func() {
			generation := currentWalletGeneration()
			walletapi.WaitNewHeightBlock()
			sHeight := walletapi.Get_Daemon_Height()
			var success bool
			for session.Domain == "app.messages.contact" {
				if !isWalletGenerationActive(generation) {
					return
				}

				var zeroscid crypto.Hash
				_, result := engram.Disk.Get_Payments_TXID(zeroscid, txid.String())

				if result.TXID != txid.String() {
					time.Sleep(time.Second * 1)
				} else {
					success = true
				}

				// If we go DEFAULT_CONFIRMATION_TIMEOUT blocks without exiting 'Confirming...' loop, display failed to transfer and break
				if walletapi.Get_Daemon_Height() > sHeight+int64(DEFAULT_CONFIRMATION_TIMEOUT) {
					btnSend.Text = i18n.T("messages.failed_send")
					btnSend.Disable()
					btnSend.Refresh()
					break
				}

				// If daemon height has incremented, print retry counters into button space
				if walletapi.Get_Daemon_Height()-sHeight > 0 {
					btnSend.Text = fmt.Sprintf(i18n.T("transfers.confirming_progress"), walletapi.Get_Daemon_Height()-sHeight, DEFAULT_CONFIRMATION_TIMEOUT)
					btnSend.Refresh()
				}

				// If success, reload page w/ latest content. Otherwise retain the Failure message for UX relay
				if success {
					refreshMessageHistoryAsync(false)
					uiDo(func() {
						if !isWalletGenerationActive(generation) {
							return
						}
						messageRecords = append(messageRecords, MessageRecord{
							Entry: rpc.Entry{
								TXID:     txid.String(),
								Time:     time.Now(),
								Incoming: false,
							},
							ContactKey: messages.Contact,
							Label:      messages.Contact,
							Comment:    sentMessage,
						})
						originalMessageRecords = append(originalMessageRecords, messageRecords[len(messageRecords)-1])
						renderThread(messageRecords)
						btnSend.Text = i18n.T("messages.send")
						btnSend.Disable()
						btnSend.Refresh()
					})
					break
				} else {
					time.Sleep(time.Second * 1)
				}
			}
		}()
	}

	top := container.NewVBox(
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
		container.NewCenter(
			heading,
		),
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
	)

	// Set spacer sizes for top
	for _, obj := range top.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	topContent := container.NewVBox(
		lastActive,
		canvas.NewRectangle(color.Transparent),
		threadSearch,
		canvas.NewRectangle(color.Transparent),
	)

	// Set spacer sizes for topContent
	for _, obj := range topContent.Objects {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	middle := container.NewStack(subframe, chatbox)

	composerItems := []fyne.CanvasObject{
		canvas.NewRectangle(color.Transparent),
		labelLimit,
		canvas.NewRectangle(color.Transparent),
		entry,
		canvas.NewRectangle(color.Transparent),
		wrapMobileButton(btnSend),
		canvas.NewRectangle(color.Transparent),
	}

	for _, obj := range composerItems {
		if r, ok := obj.(*canvas.Rectangle); ok {
			r.SetMinSize(standardSpacerSize())
		}
	}

	bottomBlock := container.NewVBox(composerItems...)

	bottom := container.NewStack(
		container.NewVBox(
			canvas.NewRectangle(color.Transparent),
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			canvas.NewRectangle(color.Transparent),
		),
	)

	// Set spacer sizes for bottom
	if vbox, ok := bottom.Objects[0].(*fyne.Container); ok {
		for _, obj := range vbox.Objects {
			if r, ok := obj.(*canvas.Rectangle); ok {
				r.SetMinSize(standardSpacerSize())
			}
		}
	}

	center := container.NewBorder(topContent, bottomBlock, nil, nil, middle)

	var gridItem1 *fyne.Container
	if isMobile() {
		pmScroll := container.NewVScroll(center)
		SetCurrentScrollBox(pmScroll)
		entry.OnFocusGained = func() {
			showVirtualKeyboard(entry)
		}
		gridItem1 = container.NewMax(pmScroll)
	} else {
		SetCurrentScrollBox(nil)
		gridItem1 = container.NewMax(center)
	}

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if session.Domain != "app.messages.contact" {
			return
		}

		if k.Name == fyne.KeyUp {
			session.Dashboard = "app.messages"
			messages.Contact = ""
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		} else if k.Name == fyne.KeyEscape {
			session.Dashboard = "app.messages"
			messages.Contact = ""
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutMessages())
			removeOverlays()
		} else if k.Name == fyne.KeyF5 {
			session.Window.SetContent(layoutPM())
		}
	})

	// Center slot receives all space between window edges and bottom bar; do not put
	// main content in Border "top" — top height is only MinSize() (see Fyne borderLayout).
	c := container.NewBorder(
		top,
		bottom,
		nil,
		nil,
		gridItem1,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return layout
}
