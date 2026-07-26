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
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"
	"github.com/civilware/epoch"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi/xswd"
	"github.com/deroproject/graviton"
)

var settingsActiveTab int

func layoutSettings() fyne.CanvasObject {
	stopGnomon()
	rectScroll := canvas.NewRectangle(color.Transparent)
	rectScroll.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	heading := canvas.NewText(i18n.T("settings.title"), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	labelNetwork := canvas.NewText(i18n.T("settings.network"), apptheme.C.Gray)
	labelNetwork.TextStyle = fyne.TextStyle{Bold: true}
	labelNetwork.TextSize = scaleFont(14)

	labelNode := canvas.NewText(i18n.T("settings.connection"), apptheme.C.Gray)
	labelNode.TextStyle = fyne.TextStyle{Bold: true}
	labelNode.TextSize = scaleFont(14)

	labelSecurity := canvas.NewText(i18n.T("settings.security"), apptheme.C.Gray)
	labelSecurity.TextStyle = fyne.TextStyle{Bold: true}
	labelSecurity.TextSize = scaleFont(14)

	textRemoteAccess := widget.NewRichTextWithText("A username and password is required in order to allow application connectivity.")
	textRemoteAccess.Wrapping = fyne.TextWrapWord

	btnRestore := widget.NewButton(i18n.T("settings.restore_defaults"), nil)
	btnDelete := widget.NewButton(i18n.T("settings.clear_data"), nil)

	type NodeItem struct {
		Address string
		Status  string
	}

	mainnetNodes := []NodeItem{
		{Address: "127.0.0.1:10102", Status: "unknown"},
		{Address: "dero.rabidmining.com:10102", Status: "unknown"},
		{Address: "dero-node.net:10102", Status: "unknown"},
		{Address: "community-pools.mysrv.cloud:10102", Status: "unknown"},
		{Address: "node.derofoundation.org:11012", Status: "unknown"},
	}
	testnetNodes := []NodeItem{
		{Address: "69.30.234.163:40402", Status: "unknown"},
		{Address: "testnet.derofoundation.co:40402", Status: "unknown"},
		{Address: "127.0.0.1:40402", Status: "unknown"},
	}
	simulatorNodes := []NodeItem{
		{Address: "127.0.0.1:20000", Status: "unknown"},
	}

	getNodesKey := func(network string) string {
		switch network {
		case NETWORK_TESTNET:
			return "testnet_nodes"
		case NETWORK_SIMULATOR:
			return "simulator_nodes"
		default:
			return "mainnet_nodes"
		}
	}

	getDefaultNodes := func(network string) []NodeItem {
		switch network {
		case NETWORK_TESTNET:
			return testnetNodes
		case NETWORK_SIMULATOR:
			return simulatorNodes
		default:
			return mainnetNodes
		}
	}

	loadNodesForNetwork := func(network string) []NodeItem {
		nodesKey := getNodesKey(network)
		if data, err := GetValue("settings", []byte(nodesKey)); err == nil && len(data) > 0 {
			var savedNodes []NodeItem
			if err := json.Unmarshal(data, &savedNodes); err == nil && len(savedNodes) > 0 {
				return savedNodes
			}
		}
		return getDefaultNodes(network)
	}

	currentNetwork := getNetwork()
	nodeData := loadNodesForNetwork(currentNetwork)

	nodeContainer := container.NewVBox()

	var updateNodeContainer func()

	updateNodeContainer = func() {
		nodeContainer.Objects = nil

		for i := range nodeData {
			i := i // capture loop variable
			item := &nodeData[i]

			var iconResource fyne.Resource
			switch item.Status {
			case "connected":
				iconResource = theme.ConfirmIcon()
			case "failed":
				iconResource = theme.CancelIcon()
			}

			rowIcon := widget.NewIcon(iconResource)

			removeBtn := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
				if len(nodeData) <= 1 {
					return
				}

				removedAddress := item.Address
				wasConnected := item.Status == "connected" || getDaemon() == removedAddress

				nodeData = append(nodeData[:i], nodeData[i+1:]...)

				if wasConnected {
					newIndex := i - 1
					if newIndex < 0 {
						newIndex = 0
					}
					if newIndex >= len(nodeData) {
						newIndex = len(nodeData) - 1
					}
					nodeData[newIndex].Status = "connected"
					setDaemon(nodeData[newIndex].Address)

					for j := range nodeData {
						if j != newIndex {
							nodeData[j].Status = "unknown"
						}
					}
				}

				if data, err := json.Marshal(nodeData); err == nil {
					StoreValue("settings", []byte(getNodesKey(session.Network)), data)
				}

				updateNodeContainer()
			})
			removeBtn.Importance = widget.MediumImportance
			if len(nodeData) <= 1 {
				removeBtn.Disable()
			}

			addressLabel := widget.NewLabel(item.Address)
			addressLabel.Truncation = fyne.TextTruncateEllipsis

			row := container.NewBorder(
				nil, nil, nil,
				container.NewHBox(rowIcon, wrapMobileButton(removeBtn)),
				addressLabel,
			)

			tapBtn := widget.NewButton("", func() {
				if testNodeConnection(item.Address) {
					item.Status = "connected"
					setDaemon(item.Address)

					for j := range nodeData {
						if j != i {
							nodeData[j].Status = "unknown"
						}
					}

					if data, err := json.Marshal(nodeData); err == nil {
						StoreValue("settings", []byte(getNodesKey(session.Network)), data)
					}
				} else {
					item.Status = "failed"
				}
				updateNodeContainer()
			})
			tapBtn.Importance = widget.LowImportance
			tapBtn.Alignment = widget.ButtonAlignLeading
			tapBtn.Text = ""

			clickableRow := container.NewMax(
				wrapMobileButton(tapBtn),
				row,
			)

			nodeContainer.Add(clickableRow)
		}
		nodeContainer.Refresh()
	}

	currentDaemon := getDaemon()
	for i := range nodeData {
		if nodeData[i].Address == currentDaemon {
			nodeData[i].Status = "connected"
		}
	}
	updateNodeContainer()

	getNodePlaceholder := func(network string) string {
		switch network {
		case NETWORK_TESTNET:
			return "hostname:40402"
		case NETWORK_SIMULATOR:
			return "hostname:20000"
		default:
			return "hostname:10102"
		}
	}

	entryCustomNode := widget.NewEntry()
	entryCustomNode.PlaceHolder = getNodePlaceholder(currentNetwork)

	showNodeError := func(err error) {
		entryCustomNode.Validator = func(s string) error {
			return err
		}
		entryCustomNode.SetValidationError(err)
		entryCustomNode.FocusGained()
		entryCustomNode.FocusLost()
	}

	clearNodeError := func() {
		entryCustomNode.Validator = nil
		entryCustomNode.SetValidationError(nil)
		entryCustomNode.Refresh()
	}

	btnAddNode := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		nodeAddress := strings.TrimSpace(entryCustomNode.Text)
		nodeAddress = strings.ReplaceAll(nodeAddress, " ", "")
		if nodeAddress != "" {
			// Check for duplicate
			for _, node := range nodeData {
				if node.Address == nodeAddress {
					showNodeError(errors.New("node already exists"))
					return
				}
			}

			if testNodeConnectionTimeout(nodeAddress, 500*time.Millisecond) {
				clearNodeError()

				for i := range nodeData {
					nodeData[i].Status = "unknown"
				}

				nodeData = append(nodeData, NodeItem{
					Address: nodeAddress,
					Status:  "connected",
				})
				setDaemon(nodeAddress)

				if data, err := json.Marshal(nodeData); err == nil {
					StoreValue("settings", []byte(getNodesKey(session.Network)), data)
				}

				entryCustomNode.Text = ""
				entryCustomNode.Refresh()
				updateNodeContainer()
			} else {
				showNodeError(errors.New("node unreachable"))
			}
		}
	})
	btnAddNode.Importance = widget.MediumImportance
	btnAddNode.Disable()

	entryCustomNode.OnChanged = func(s string) {
		clearNodeError()
		s = strings.TrimSpace(s)
		if s != "" {
			btnAddNode.Enable()
		} else {
			btnAddNode.Disable()
		}
	}

	entrySection := container.NewBorder(nil, nil, nil, wrapMobileButton(btnAddNode), entryCustomNode)
	entryWrapper := container.NewStack(
		canvas.NewRectangle(color.Transparent),
		entrySection,
	)
	entryWrapper.Resize(fyne.NewSize(ui.Width*0.9, 35))

	labelScan := widget.NewRichTextFromMarkdown("Enter the number of past blocks that the wallet should scan:")
	labelScan.Wrapping = fyne.TextWrapWord

	entryScan := widget.NewEntry()
	entryScan.PlaceHolder = "# of Latest Blocks (Optional)"
	entryScan.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		_, err := strconv.ParseInt(s, 10, 64)
		return err
	}
	entryScan.OnChanged = func(s string) {
		if s == "" {
			session.TrackRecentBlocks = 0
			return
		}
		if blocks, err := strconv.ParseInt(s, 10, 64); err == nil && blocks > 0 {
			session.TrackRecentBlocks = blocks
		} else {
			session.TrackRecentBlocks = 0
		}
	}

	if session.TrackRecentBlocks > 0 {
		blocks := strconv.FormatInt(session.TrackRecentBlocks, 10)
		entryScan.Text = blocks
		entryScan.Refresh()
	}

	tabsNetwork := container.NewAppTabs(
		container.NewTabItem(NETWORK_MAINNET, container.NewVBox()),
		container.NewTabItem(NETWORK_TESTNET, container.NewVBox()),
		container.NewTabItem(NETWORK_SIMULATOR, container.NewVBox()),
	)

	tabsNetwork.OnSelected = func(tab *container.TabItem) {
		s := tab.Text
		if s != NETWORK_TESTNET && s != NETWORK_SIMULATOR {
			s = NETWORK_MAINNET
		}
		setNetwork(s)

		nodeData = loadNodesForNetwork(s)

		for i := range nodeData {
			if nodeData[i].Address == getDaemon() {
				nodeData[i].Status = "connected"
			} else {
				nodeData[i].Status = "unknown"
			}
		}

		globals.InitNetwork()

		entryCustomNode.PlaceHolder = getNodePlaceholder(s)
		clearNodeError()

		updateNodeContainer()
	}

	net, _ := GetValue("settings", []byte("network"))
	switch string(net) {
	case NETWORK_TESTNET:
		tabsNetwork.SelectTabIndex(1)
	case NETWORK_SIMULATOR:
		tabsNetwork.SelectTabIndex(2)
	default:
		tabsNetwork.SelectTabIndex(0)
	}

	entryUser := widget.NewEntry()
	entryUser.PlaceHolder = "Username"
	entryUser.SetText(remoteAccess.RPC.user)

	entryPass := widget.NewEntry()
	entryPass.PlaceHolder = "Password"
	entryPass.Password = true
	entryPass.SetText(remoteAccess.RPC.pass)

	entryUser.OnChanged = func(s string) {
		remoteAccess.RPC.user = s
		StoreValue("settings", []byte("rpc_user"), []byte(s))
	}

	entryPass.OnChanged = func(s string) {
		remoteAccess.RPC.pass = s
		StoreValue("settings", []byte("rpc_pass"), []byte(s))
	}

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		initSettings()

		resizeWindow(ui.MaxWidth, ui.MaxHeight)
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
	})

	btnRestore.OnTapped = func() {
		verificationOverlay(
			false,
			i18n.T("settings.title"),
			i18n.T("settings.reset_all_prompt"),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					setNetwork(NETWORK_MAINNET)
					setDaemon(DEFAULT_REMOTE_DAEMON)
					setAuthMode("true")
					setGnomon("1")

					StoreValue("settings", []byte("mainnet_nodes"), []byte{})
					StoreValue("settings", []byte("testnet_nodes"), []byte{})
					StoreValue("settings", []byte("simulator_nodes"), []byte{})

					remoteAccess.RPC.user = newRPCUsername()
					remoteAccess.RPC.pass = newRPCPassword()
					StoreValue("settings", []byte("rpc_user"), []byte(remoteAccess.RPC.user))
					StoreValue("settings", []byte("rpc_pass"), []byte(remoteAccess.RPC.pass))

					resizeWindow(ui.MaxWidth, ui.MaxHeight)
					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutSettings())
					removeOverlays()
				}
			},
		)
	}

	statusText := canvas.NewText("", apptheme.C.Account)
	statusText.TextSize = scaleFont(12)

	btnDelete.OnTapped = func() {
		verificationOverlay(
			false,
			i18n.T("settings.title"),
			fmt.Sprintf(i18n.T("settings.delete_local_data_fmt"), strings.ToLower(session.Network)),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					err := cleanGnomonData()
					if err != nil {
						if parseError, ok := err.(*os.PathError); !ok {
							err = fmt.Errorf(i18n.T("settings.error_clearing_data_fmt"), session.Network)
						} else {
							err = parseError.Err
						}

						statusText.Color = apptheme.C.Red
						statusText.Text = err.Error()
						statusText.Refresh()
						return
					}

					statusText.Color = apptheme.C.Green
					statusText.Text = fmt.Sprintf(i18n.T("settings.gnomon_deleted"), strings.ToLower(session.Network))
					statusText.Refresh()
				}
			},
		)
	}

	formSettings := container.NewVBox(
		labelNetwork,
		rectSpacer,
		tabsNetwork,
		widget.NewLabel(""),
		labelNode,
		rectSpacer,
		rectSpacer,
		entryWrapper,
		rectSpacer,
		nodeContainer,
		rectSpacer,
		labelScan,
		rectSpacer,
		entryScan,
		widget.NewLabel(""),
		labelSecurity,
		rectSpacer,
		textRemoteAccess,
		rectSpacer,
		entryUser,
		rectSpacer,
		entryPass,
		rectSpacer,
		statusText,
		wrapMobileButton(btnDelete),
		wrapMobileButton(btnRestore),
	)

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectScroll,
				formSettings,
			),
			layout.NewSpacer(),
		),
	)

	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.MaxHeight*0.8))

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(heading),
		rectSpacer,
	)

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		footer,
		nil,
		nil,
		scrollBox,
	)

	return c
}

// layoutAppSettings creates the centralized settings page with 3 tabs:
// Remote Access, TELA, and Advanced
func layoutAppSettings() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	previousDomain := session.Domain // Save before overwriting

	// Track the actual caller if we aren't coming from a settings sub-page
	if previousDomain != "app.remoteaccess.manager" && previousDomain != "app.remoteaccess.permissions" {
		settingsCallerDomain = previousDomain
	}

	session.Domain = "app.appsettings"

	frame := &iframe{}

	rectScroll := canvas.NewRectangle(color.Transparent)
	rectScroll.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	heading := canvas.NewText(i18n.T("settings.title"), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.Width, scaleSize(1)))

	// Remote Access Tab Content
	go refreshXSWDList()

	wSpacer := widget.NewLabel(" ")

	title := canvas.NewText(i18n.T("settings.remote_access_heading"), apptheme.C.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.20))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	rpcLabel := canvas.NewText(i18n.T("settings.rpc_config"), apptheme.C.Gray)
	rpcLabel.TextSize = scaleFont(11)
	rpcLabel.Alignment = fyne.TextAlignCenter
	rpcLabel.TextStyle = fyne.TextStyle{Bold: true}

	wsLabel := canvas.NewText(i18n.T("settings.rpc_config"), apptheme.C.Gray)
	wsLabel.TextSize = scaleFont(11)
	wsLabel.Alignment = fyne.TextAlignCenter
	wsLabel.TextStyle = fyne.TextStyle{Bold: true}

	labelConnections := canvas.NewText(i18n.T("settings.connections"), apptheme.C.Gray)
	labelConnections.TextSize = scaleFont(11)
	labelConnections.Alignment = fyne.TextAlignCenter
	labelConnections.TextStyle = fyne.TextStyle{Bold: true}

	sep1 := canvas.NewRectangle(apptheme.C.Gray)
	sep1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep1,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(apptheme.C.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	shortShard := canvas.NewText(i18n.T("settings.app_connections"), apptheme.C.Gray)
	shortShard.TextStyle = fyne.TextStyle{Bold: true}
	shortShard.TextSize = scaleFont(12)

	linkColor := apptheme.C.Green

	if remoteAccess.RPC.server == nil {
		session.Link = i18n.T("settings.blocked")
		linkColor = apptheme.C.Gray
	}

	remoteAccess.RPC.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.RPC.status.TextSize = scaleFont(22)
	remoteAccess.RPC.status.TextStyle = fyne.TextStyle{Bold: true}

	serverStatus := canvas.NewText(i18n.T("settings.app_connections"), apptheme.C.Gray)
	serverStatus.TextSize = scaleFont(12)
	serverStatus.Alignment = fyne.TextAlignCenter
	serverStatus.TextStyle = fyne.TextStyle{Bold: true}

	linkCenter := container.NewCenter(
		remoteAccess.RPC.status,
	)

	remoteAccess.RPC.userText = widget.NewEntry()
	remoteAccess.RPC.userText.PlaceHolder = i18n.T("settings.username")
	remoteAccess.RPC.userText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.user = s
		}
	}

	remoteAccess.RPC.passText = widget.NewEntry()
	remoteAccess.RPC.passText.Password = true
	remoteAccess.RPC.passText.PlaceHolder = i18n.T("settings.password")
	remoteAccess.RPC.passText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.pass = s
		}
	}

	remoteAccess.RPC.portText = widget.NewEntry()
	remoteAccess.RPC.portText.PlaceHolder = i18n.T("settings.rpc_placeholder")
	remoteAccess.RPC.portText.Validator = func(s string) (err error) {
		regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
		test := regexp.MustCompile(regex)
		if test.MatchString(s) {
			remoteAccess.RPC.portText.SetValidationError(nil)
		} else {
			err = errors.New("invalid host name")
			remoteAccess.RPC.portText.SetValidationError(err)
		}
		return
	}
	remoteAccess.RPC.portText.SetText(getRemoteAccess("RPC"))

	linkColor = apptheme.C.Green

	if remoteAccess.WS.server == nil {
		session.Link = i18n.T("settings.blocked")
		linkColor = apptheme.C.Gray
	}

	remoteAccess.WS.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.WS.status.TextSize = scaleFont(22)
	remoteAccess.WS.status.TextStyle = fyne.TextStyle{Bold: true}

	deckChoice := widget.NewSelect([]string{i18n.T("settings.ws_label"), i18n.T("settings.rpc_label")}, nil)

	remoteAccess.RPC.toggle = widget.NewButton(i18n.T("settings.turn_on"), nil)
	remoteAccess.RPC.toggle.OnTapped = func() {
		switch session.Network {
		case NETWORK_TESTNET:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_TESTNET_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		case NETWORK_SIMULATOR:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_SIMULATOR_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		default:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		}

		toggleRPCServer(remoteAccess.RPC.port)
		if remoteAccess.RPC.server != nil {
			setRemoteAccess(remoteAccess.RPC.port, "RPC")
			deckChoice.Disable()
			remoteAccess.RPC.portText.Disable()
		} else {
			deckChoice.Enable()
			remoteAccess.RPC.portText.Enable()
		}
	}

	if remoteAccess.WS.portText == nil {
		remoteAccess.WS.portText = widget.NewEntry()
		remoteAccess.WS.portText.PlaceHolder = i18n.T("settings.ws_placeholder")
		remoteAccess.WS.portText.Validator = func(s string) (err error) {
			regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
			test := regexp.MustCompile(regex)
			if test.MatchString(s) {
				remoteAccess.WS.portText.SetValidationError(nil)
			} else {
				err = errors.New("invalid host name")
				remoteAccess.WS.portText.SetValidationError(err)
			}
			return
		}
	}

	remoteAccess.WS.toggle = widget.NewButton(i18n.T("settings.turn_on"), nil)
	remoteAccess.WS.toggle.OnTapped = func() {
		if remoteAccess.WS.portText.Validate() != nil {
			remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
			remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
		} else {
			_, err := net.ResolveTCPAddr("tcp", remoteAccess.WS.port)
			if err != nil {
				logger.Errorf("[Remote Access] XSWD port: %s\n", err)
				remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
				remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
			} else {
				remoteAccess.WS.port = remoteAccess.WS.portText.Text
			}
		}

		remoteAccess.EPOCH.err = nil
		toggleXSWD(remoteAccess.WS.port)
		if remoteAccess.WS.server != nil {
			setRemoteAccessDual(remoteAccess.WS.port, "WS") // Use dual storage for consistency
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
			if remoteAccess.EPOCH.enabled {
				err := epoch.StartGetWork(engram.Disk.GetAddress().String(), session.Daemon)
				if err != nil {
					logger.Errorf("[EPOCH] Connecting: %s\n", err)
					remoteAccess.EPOCH.err = err
				} else {
					remoteAccess.EPOCH.err = nil
					setRemoteAccess(epoch.GetPort(), "EPOCH")
				}
			}
		} else {
			stopEPOCH()
			remoteAccess.WS.portText.Enable()
			deckChoice.Enable()
		}
	}

	if session.Offline {
		remoteAccess.RPC.toggle.Text = i18n.T("settings.disabled_offline")
		remoteAccess.RPC.toggle.Disable()
		remoteAccess.RPC.portText.Disable()
		remoteAccess.WS.toggle.Text = i18n.T("settings.disabled_offline")
		remoteAccess.WS.toggle.Disable()
		remoteAccess.WS.portText.Disable()
	} else {
		if remoteAccess.RPC.server != nil {
			remoteAccess.RPC.status.Text = i18n.T("settings.allowed")
			remoteAccess.RPC.status.Color = apptheme.C.Green
			remoteAccess.RPC.toggle.Text = i18n.T("settings.turn_off")
			remoteAccess.RPC.userText.Disable()
			remoteAccess.RPC.passText.Disable()
			remoteAccess.RPC.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.RPC.status.Text = i18n.T("settings.blocked")
			remoteAccess.RPC.status.Color = apptheme.C.Gray
			remoteAccess.RPC.toggle.Text = i18n.T("settings.turn_on")
			remoteAccess.RPC.userText.Enable()
			remoteAccess.RPC.passText.Enable()
			remoteAccess.RPC.portText.Enable()
		}

		if remoteAccess.WS.server != nil {
			remoteAccess.WS.status.Text = i18n.T("settings.allowed")
			remoteAccess.WS.status.Color = apptheme.C.Green
			remoteAccess.WS.toggle.Text = i18n.T("settings.turn_off")
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.WS.status.Text = i18n.T("settings.blocked")
			remoteAccess.WS.status.Color = apptheme.C.Gray
			remoteAccess.WS.toggle.Text = i18n.T("settings.turn_on")
			remoteAccess.WS.portText.Enable()
		}
	}

	remoteAccess.RPC.userText.SetText(remoteAccess.RPC.user)
	remoteAccess.RPC.passText.SetText(remoteAccess.RPC.pass)

	linkCopy := widget.NewHyperlinkWithStyle(i18n.T("settings.copy_creds"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopy.OnTapped = func() {
		a.Clipboard().SetContent(remoteAccess.RPC.user + ":" + remoteAccess.RPC.pass)
	}

	linkPermissions := widget.NewHyperlinkWithStyle(i18n.T("settings.advanced_link"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkPermissions.OnTapped = func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDPermissions())
		removeOverlays()
	}

	remoteAccess.WS.list = widget.NewList(
		func() int {
			return len(remoteAccess.WS.apps)
		},
		func() fyne.CanvasObject {
			return container.NewVBox(
				widget.NewLabel(""),
			)
		},
		func(li widget.ListItemID, co fyne.CanvasObject) {
			app := remoteAccess.WS.apps[li]
			fyne.Do(func() {
				co.(*fyne.Container).Objects[0].(*widget.Label).SetText(app.Name)
			})
		},
	)

	remoteAccess.WS.list.OnSelected = func(id widget.ListItemID) {
		remoteAccess.WS.list.UnselectAll()
		remoteAccess.WS.list.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDAppManager(&remoteAccess.WS.apps[id]))
		removeOverlays()
	}

	xswdForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			wsLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					container.NewCenter(
						remoteAccess.WS.status,
					),
					rectSpacer,
					serverStatus,
					wSpacer,
					wrapMobileButton(remoteAccess.WS.toggle),
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkPermissions,
						layout.NewSpacer(),
					),
				),
			),
		),
		container.NewStack(
			rectWidth90,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					labelConnections,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewStack(
						rect,
						remoteAccess.WS.list,
					),
				),
			),
		),
		layout.NewSpacer(),
	)

	rpcForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			rpcLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					linkCenter,
					rectSpacer,
					serverStatus,
					wSpacer,
					wrapMobileButton(remoteAccess.RPC.toggle),
					wSpacer,
					remoteAccess.RPC.portText,
					rectSpacer,
					remoteAccess.RPC.userText,
					rectSpacer,
					remoteAccess.RPC.passText,
					wSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkCopy,
						layout.NewSpacer(),
					),
				),
			),
			layout.NewSpacer(),
		),
	)

	deckFeatures := container.NewStack()
	if remoteAccess.RPC.server != nil {
		deckFeatures.Add(rpcForm)
		deckChoice.SetSelectedIndex(1)
	} else {
		deckFeatures.Add(xswdForm)
		deckChoice.SetSelectedIndex(0)
	}

	deckChoice.OnChanged = func(s string) {
		if s == i18n.T("settings.rpc_label") {
			deckFeatures.Objects[0] = rpcForm
		} else {
			deckFeatures.Objects[0] = xswdForm
		}
	}

	remoteAccessContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				title,
			),
		),
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewStack(
				rectWidth90,
				deckChoice,
			),
		),
		container.NewBorder(
			deckFeatures,
			nil,
			nil,
			nil,
		),
	)

	// TELA Tab Content
	telaTitle := canvas.NewText(i18n.T("settings.tela_heading"), apptheme.C.Gray)
	telaTitle.TextStyle = fyne.TextStyle{Bold: true}
	telaTitle.TextSize = scaleFont(16)

	// Port Start entry
	entryPortStart := NewMobileEntry()
	entryPortStart.SetPlaceHolder(strconv.Itoa(tela.DEFAULT_PORT_START))
	// Load Port Start setting from dual storage
	if portStart, found := getTELADual("Port Start"); found {
		entryPortStart.SetText(portStart)
		logger.Printf("[Engram] TELA Port Start loaded from storage: %s", portStart)
	} else {
		logger.Printf("[Engram] TELA Port Start not found in storage, using default")
	}
	entryPortStart.Validator = func(s string) (err error) {
		if s == "" {
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("%s", i18n.T("settings.invalid_port"))
		}
		return tela.SetPortStart(i)
	}
	entryPortStart.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Port Start", []byte(s))
		}
	}

	// Min Likes entry
	entryMinLikes := NewMobileEntry()
	entryMinLikes.SetPlaceHolder("30")
	if storedMinLikes, found := getTELADual("Min Likes"); found {
		entryMinLikes.SetText(storedMinLikes)
	} else {
		entryMinLikes.SetText("30")
	}
	entryMinLikes.Validator = func(s string) (err error) {
		if s == "" {
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("%s", i18n.T("settings.invalid_percent"))
		}
		if i < 0 || i > 100 {
			return fmt.Errorf("%s", i18n.T("settings.percent_range"))
		}
		return nil
	}
	entryMinLikes.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Min Likes", []byte(s))
		}
	}

	// Exclusions entry
	entryExclusions := NewMobileEntry()
	entryExclusions.SetPlaceHolder(i18n.T("settings.exclusions_placeholder"))
	if storedExclusions, found := getTELADual("Exclusions"); found {
		entryExclusions.SetText(storedExclusions)
	}
	entryExclusions.OnChanged = func(s string) {
		if s != "" {
			setTELADual("Exclusions", []byte(s))
		} else {
			deleteTELADual("Exclusions")
		}
	}

	// Restrictive Mode checkbox
	wRestrictiveMode := widget.NewCheck("", nil)
	// Load Restrictive Mode setting from dual storage
	restrictiveModeEnabled := false // Default to OFF (unrestrictive mode)
	if restrictiveMode, found := getTELADual("Restrictive Mode"); found {
		if restrictiveMode == "false" {
			restrictiveModeEnabled = false
			logger.Printf("[Engram] TELA Restrictive Mode loaded from storage: Disabled")
		} else {
			restrictiveModeEnabled = true
			logger.Printf("[Engram] TELA Restrictive Mode loaded from storage: Enabled")
		}
	} else {
		// Also check the old "Mode" key for backward compatibility
		if storedTelaMode, found := getTELADual("Mode"); found {
			if storedTelaMode == "Unrestrictive" {
				restrictiveModeEnabled = false
				logger.Printf("[Engram] TELA Restrictive Mode loaded from legacy Mode key: Disabled")
			} else {
				restrictiveModeEnabled = true
				logger.Printf("[Engram] TELA Restrictive Mode loaded from legacy Mode key: Enabled")
			}
		} else {
			// Default to unrestricted mode
			restrictiveModeEnabled = false
			logger.Printf("[Engram] TELA Restrictive Mode using default: Disabled")
		}
	}
	wRestrictiveMode.SetChecked(restrictiveModeEnabled)
	wRestrictiveMode.OnChanged = func(b bool) {
		if b {
			// For restrictive mode, save the key
			setTELADual("Restrictive Mode", []byte("true"))
			logger.Printf("[Engram] TELA Restrictive Mode enabled (saved true)")
		} else {
			// For unrestricted mode, delete both keys since unrestrictive is the default
			deleteTELADual("Restrictive Mode")
			deleteTELADual("Mode")
			logger.Printf("[Engram] TELA Restrictive Mode disabled (deleted keys)")
		}
	}

	// Allow Content Updates radio buttons
	allowOptions := []string{i18n.T("settings.permissions.deny"), i18n.T("settings.permissions.allow")}
	wAllowUpdates := widget.NewRadioGroup(allowOptions, nil)
	wAllowUpdates.Horizontal = true
	// Load Allow Updates setting from dual storage
	if allowUpdates, found := getTELADual("Allow Updates"); found {
		if allowUpdates == "Allow" {
			wAllowUpdates.SetSelected(i18n.T("settings.permissions.allow"))
			logger.Printf("[Engram] TELA Allow Updates loaded from storage: Allow")
		} else {
			wAllowUpdates.SetSelected(i18n.T("settings.permissions.deny"))
			logger.Printf("[Engram] TELA Allow Updates loaded from storage: Deny")
		}
	} else {
		// Default to Allow when no stored value exists
		wAllowUpdates.SetSelected(i18n.T("settings.permissions.allow"))
		tela.AllowUpdates(true)
		setTELADual("Allow Updates", []byte("Allow"))
		logger.Printf("[Engram] TELA Allow Updates defaulting to: Allow")
	}
	wAllowUpdates.OnChanged = func(s string) {
		if s == i18n.T("settings.permissions.allow") {
			tela.AllowUpdates(true)
			setTELADual("Allow Updates", []byte("Allow"))
			logger.Printf("[Engram] TELA Allow Updates set to Allow")
		} else {
			tela.AllowUpdates(false)
			setTELADual("Allow Updates", []byte("Deny"))
			logger.Printf("[Engram] TELA Allow Updates set to Deny")
		}
	}

	// Rescan Recheck radio buttons
	rescanOptions := []string{i18n.T("common.no"), i18n.T("common.yes")}
	wRescanRecheck := widget.NewRadioGroup(rescanOptions, nil)
	wRescanRecheck.Horizontal = true
	if storedRescanRecheck, found := getTELADual("Rescan Recheck"); found {
		if storedRescanRecheck == "Yes" || storedRescanRecheck == i18n.T("common.yes") {
			wRescanRecheck.SetSelected(i18n.T("common.yes"))
		} else {
			wRescanRecheck.SetSelected(i18n.T("common.no"))
		}
	} else {
		wRescanRecheck.SetSelected(i18n.T("common.no"))
	}
	wRescanRecheck.OnChanged = func(s string) {
		if s == i18n.T("common.yes") {
			setTELADual("Rescan Recheck", []byte("Yes"))
		} else {
			setTELADual("Rescan Recheck", []byte("No"))
		}
	}

	// Sort By radio buttons
	sortByOptions := []string{i18n.T("settings.ratings"), i18n.T("settings.az")}
	wSortBy := widget.NewRadioGroup(sortByOptions, nil)
	wSortBy.Horizontal = true
	if storedSortBy, found := getTELADual("Sort By"); found {
		if storedSortBy == "Z-A" {
			wSortBy.SetSelected(i18n.T("settings.az"))
			setTELADual("Sort By", []byte("A-Z"))
			setTELADual("Sort Order", []byte("Descending"))
		} else if storedSortBy == "A-Z" {
			wSortBy.SetSelected(i18n.T("settings.az"))
		} else {
			wSortBy.SetSelected(i18n.T("settings.ratings"))
		}
	} else {
		wSortBy.SetSelected(sortByOptions[0])
	}
	wSortBy.OnChanged = func(s string) {
		if s != "" {
			if s == i18n.T("settings.ratings") {
				setTELADual("Sort By", []byte("Ratings"))
				setTELADual("Sort Order", []byte("Descending"))
			} else {
				setTELADual("Sort By", []byte("A-Z"))
				setTELADual("Sort Order", []byte("Ascending"))
			}
		}
	}

	// Reset Defaults button
	btnResetDefaults := widget.NewButton(i18n.T("settings.reset_defaults"), func() {
		wRestrictiveMode.SetChecked(false)
		wAllowUpdates.SetSelected(i18n.T("settings.permissions.deny"))
		wRescanRecheck.SetSelected(i18n.T("common.no"))
		wSortBy.SetSelected(sortByOptions[0])
		setTELADual("Sort Order", []byte("Descending"))
		entryPortStart.SetText(strconv.Itoa(tela.DEFAULT_PORT_START))
		entryMinLikes.SetText("30")
		entryExclusions.SetText("")
	})

	// Delete Search Data button
	btnDeleteSearchData := widget.NewButton(i18n.T("settings.delete_search"), func() {
		verificationOverlay(
			false,
			i18n.T("settings.tela_browser"),
			i18n.T("settings.delete_search_prompt"),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					DeleteKey("TELA Search", []byte("SCIDs"))
					DeleteKey("TELA Search", []byte("Searched SCIDs"))
					DeleteKey("TELA Search", []byte("Last Scan"))
					DeleteKey("TELA Search", []byte("Last Indexed Height"))
					DeleteKey("TELA Search", []byte("CandidateCache"))
					DeleteKey("TELA Search", []byte("NegativeCache"))
					DeleteKey("TELA Search", []byte("IndexCache"))
					DeleteKey("TELA Search", []byte("DisplayCache"))
				}
			},
		)
	})

	// Shutdown TELA button
	btnShutdownTela := widget.NewButton(i18n.T("settings.shutdown_tela"), func() {
		verificationOverlay(
			false,
			i18n.T("settings.tela_browser"),
			i18n.T("settings.shutdown_tela_prompt"),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					tela.ShutdownTELA()
				}
			},
		)
	})

	// Clear History button
	btnClearHistory := widget.NewButton(i18n.T("settings.clear_history"), func() {
		verificationOverlay(
			false,
			i18n.T("settings.tela_browser"),
			i18n.T("settings.clear_history_prompt"),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					shard, err := GetShard()
					if err != nil {
						return
					}

					store, err := graviton.NewDiskStore(shard)
					if err != nil {
						return
					}

					ss, err := store.LoadSnapshot(0)
					if err != nil {
						return
					}

					tree, err := ss.GetTree("TELA History")
					if err != nil {
						return
					}

					c := tree.Cursor()

					for k, _, err := c.First(); err == nil; k, _, err = c.Next() {
						DeleteKey(tree.GetName(), k)
					}
				}
			},
		)
	})

	telaContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				telaTitle,
			),
		),
		rectSpacer,
		wrapMobileButton(btnShutdownTela),
		rectSpacer,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.restrictive_mode")),
		wRestrictiveMode,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.allow_updates")),
		wAllowUpdates,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.rescan_recheck")),
		wRescanRecheck,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.sort_by")),
		wSortBy,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.start_port_range")),
		entryPortStart,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.search_min_likes")),
		entryMinLikes,
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.search_exclusions")),
		entryExclusions,
		rectSpacer,
		rectSpacer,
		wrapMobileButton(btnResetDefaults),
		rectSpacer,
		wrapMobileButton(btnDeleteSearchData),
		rectSpacer,
		wrapMobileButton(btnClearHistory),
	)

	// Advanced Tab Content
	advancedTitle := canvas.NewText(i18n.T("settings.advanced_heading"), apptheme.C.Gray)
	advancedTitle.TextStyle = fyne.TextStyle{Bold: true}
	advancedTitle.TextSize = scaleFont(16)

	// GNOMON Section
	gnomonTitle := canvas.NewText(i18n.T("settings.gnomon_section"), apptheme.C.Gray)
	gnomonTitle.TextSize = scaleFont(11)
	gnomonTitle.Alignment = fyne.TextAlignCenter
	gnomonTitle.TextStyle = fyne.TextStyle{Bold: true}

	gnomonDescription := widget.NewRichTextFromMarkdown(i18n.T("settings.gnomon_desc"))
	gnomonDescription.Wrapping = fyne.TextWrapWord

	checkGnomon := widget.NewCheck(i18n.T("settings.enable_gnomon"), func(b bool) {
		if b {
			StoreValue("settings", []byte("gnomon"), []byte("1"))
			gnomon.Active = 1
		} else {
			StoreValue("settings", []byte("gnomon"), []byte("0"))
			gnomon.Active = 0
		}
	})

	gmn, err := GetValue("settings", []byte("gnomon"))
	if err != nil || string(gmn) == "1" {
		gnomon.Active = 1
		checkGnomon.SetChecked(true)
		if err != nil {
			StoreValue("settings", []byte("gnomon"), []byte("1"))
		}
	} else {
		gnomon.Active = 0
		checkGnomon.SetChecked(false)
	}

	// NOTIFICATIONS Section
	notifTitle := canvas.NewText(i18n.T("settings.notifications_section"), apptheme.C.Gray)
	notifTitle.TextSize = scaleFont(11)
	notifTitle.Alignment = fyne.TextAlignCenter
	notifTitle.TextStyle = fyne.TextStyle{Bold: true}

	notifDescription := widget.NewRichTextFromMarkdown(i18n.T("settings.notifications_desc"))
	notifDescription.Wrapping = fyne.TextWrapWord

	checkNotif := widget.NewCheck(i18n.T("settings.enable_notifications"), func(b bool) {
		setNotificationsEnabled(b)
	})
	checkNotif.SetChecked(getNotificationsEnabled())

	// STATUS AREA Section
	statusAreaTitle := canvas.NewText(i18n.T("settings.status_area"), apptheme.C.Gray)
	statusAreaTitle.TextSize = scaleFont(11)
	statusAreaTitle.Alignment = fyne.TextAlignCenter
	statusAreaTitle.TextStyle = fyne.TextStyle{Bold: true}

	// PRIORITISE STATUS Section
	prioritiseDesc := widget.NewRichTextFromMarkdown(i18n.T("settings.prioritise_status_desc"))
	prioritiseDesc.Wrapping = fyne.TextWrapWord

	prioritiseCheck := widget.NewCheck(i18n.T("settings.prioritise_status"), func(b bool) {
		setPrioritiseStatus(b)
	})
	prioritiseCheck.SetChecked(getPrioritiseStatus())

	// EPOCH STATISTICS Section
	epochTitle := canvas.NewText(i18n.T("settings.epoch_section"), apptheme.C.Gray)
	epochTitle.TextSize = scaleFont(11)
	epochTitle.Alignment = fyne.TextAlignCenter
	epochTitle.TextStyle = fyne.TextStyle{Bold: true}

	spacerEpoch := canvas.NewRectangle(color.Transparent)
	spacerEpoch.SetMinSize(fyne.NewSize(140, 0))

	wEpoch := widget.NewSelect([]string{i18n.T("settings.session"), i18n.T("settings.total")}, nil)
	wEpoch.SetSelected(i18n.T("settings.session"))

	epochSession, _ := epoch.GetSession(time.Second * 4)

	labelEpochHashes := widget.NewRichTextFromMarkdown(i18n.T("settings.hashes"))
	labelEpochHashes.Wrapping = fyne.TextWrapWord

	epochHashes := fmt.Sprintf("%.1fK", float64(epochSession.Hashes)/1000)
	textEpochHashes := widget.NewRichTextFromMarkdown(epochHashes)
	textEpochHashes.Wrapping = fyne.TextWrapWord

	labelEpochBlocks := widget.NewRichTextFromMarkdown(i18n.T("settings.miniblocks"))
	labelEpochBlocks.Wrapping = fyne.TextWrapWord

	epochBlocks := fmt.Sprintf("%d", epochSession.MiniBlocks)
	textEpochBlocks := widget.NewRichTextFromMarkdown(epochBlocks)
	textEpochBlocks.Wrapping = fyne.TextWrapWord

	wEpoch.OnChanged = func(s string) {
		epochSession, _ := epoch.GetSession(time.Second * 4)
		if s == i18n.T("settings.total") {
			total := epoch.GetSessionEPOCH_Result{
				Hashes:     remoteAccess.EPOCH.total.Hashes,
				MiniBlocks: remoteAccess.EPOCH.total.MiniBlocks,
			}

			if epoch.IsActive() {
				total.Hashes += epochSession.Hashes
				total.MiniBlocks += epochSession.MiniBlocks
			}

			textEpochHashes.ParseMarkdown(epoch.HashesToString(total.Hashes))
			textEpochBlocks.ParseMarkdown(fmt.Sprintf("%d", total.MiniBlocks))

			return
		}

		textEpochHashes.ParseMarkdown(epoch.HashesToString(epochSession.Hashes))
		textEpochBlocks.ParseMarkdown(fmt.Sprintf("%d", epochSession.MiniBlocks))
	}

	// SCANNING Section
	scanningTitle := canvas.NewText(i18n.T("settings.scanning_section"), apptheme.C.Gray)
	scanningTitle.TextSize = scaleFont(11)
	scanningTitle.Alignment = fyne.TextAlignCenter
	scanningTitle.TextStyle = fyne.TextStyle{Bold: true}

	scanningDescription := widget.NewRichTextFromMarkdown(i18n.T("settings.scanning_desc"))
	scanningDescription.Wrapping = fyne.TextWrapWord

	entryTrackBlocks := NewMobileEntry()
	entryTrackBlocks.SetPlaceHolder(i18n.T("settings.blocks_placeholder"))
	entryTrackBlocks.Validator = func(s string) (err error) {
		if s == "" {
			return nil
		}
		_, parseErr := strconv.ParseInt(s, 10, 64)
		return parseErr
	}
	entryTrackBlocks.OnChanged = func(s string) {
		if s == "" {
			session.TrackRecentBlocks = 0
			return
		}
		if blocks, err := strconv.ParseInt(s, 10, 64); err == nil && blocks > 0 {
			session.TrackRecentBlocks = blocks
		} else {
			session.TrackRecentBlocks = 0
		}
	}

	if session.TrackRecentBlocks > 0 {
		blocks := strconv.FormatInt(session.TrackRecentBlocks, 10)
		entryTrackBlocks.SetText(blocks)
	}

	// MAINTENANCE Section
	maintenanceTitle := canvas.NewText(i18n.T("settings.maintenance_section"), apptheme.C.Gray)
	maintenanceTitle.TextSize = scaleFont(11)
	maintenanceTitle.Alignment = fyne.TextAlignCenter
	maintenanceTitle.TextStyle = fyne.TextStyle{Bold: true}

	btnClearLocalData := widget.NewButton(i18n.T("settings.clear_data"), func() {
		verificationOverlay(
			false,
			i18n.T("settings.confirm_dialog"),
			fmt.Sprintf(i18n.T("settings.delete_local_data_fmt"), strings.ToLower(session.Network)),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					err := cleanGnomonData()
					if err != nil {
						if parseError, ok := err.(*os.PathError); !ok {
							err = fmt.Errorf(i18n.T("settings.error_clearing_data_fmt"), session.Network)
						} else {
							err = parseError.Err
						}

						errorDialog := dialog.NewError(err, session.Window)
						errorDialog.SetOnClosed(func() {})
						errorDialog.Show()
						return
					}

					successDialog := dialog.NewInformation(i18n.T("common.success"), fmt.Sprintf(i18n.T("settings.gnomon_deleted"), strings.ToLower(session.Network)), session.Window)
					successDialog.SetOnClosed(func() {})
					successDialog.Show()
				}
			},
		)
	})

	btnRestoreDefaults := widget.NewButton(i18n.T("settings.restore_defaults"), func() {
		verificationOverlay(
			false,
			i18n.T("settings.confirm_dialog"),
			i18n.T("settings.reset_all_prompt"),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					setNetwork(NETWORK_MAINNET)
					setDaemon(DEFAULT_REMOTE_DAEMON)
					setAuthMode("true")
					setGnomon("1")
					remoteAccess.RPC.user = "username"
					remoteAccess.RPC.pass = "password"
					remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
					setRemoteAccess(remoteAccess.RPC.port, "RPC")

					successDialog := dialog.NewInformation(i18n.T("common.success"), i18n.T("settings.all_defaults"), session.Window)
					successDialog.SetOnClosed(func() {})
					successDialog.Show()
				}
			},
		)
	})

	btnExportDebugLog := widget.NewButton(i18n.T("settings.export_debug"), func() {
		debugLogPath := getDebugLogPath()
		data, err := os.ReadFile(debugLogPath)
		if err != nil {
			if os.IsNotExist(err) {
				dialog.ShowInformation(i18n.T("settings.debug_log"), i18n.T("settings.debug_log_not_found"), session.Window)
				return
			}

			logger.Errorf("[Engram] Could not read debug log %s: %s\n", debugLogPath, err)
			dialog.ShowError(fmt.Errorf("could not read debug log"), session.Window)
			return
		}

		dialogFileSave := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
			if err != nil {
				logger.Errorf("[Engram] File dialog: %s\n", err)
				dialog.ShowError(fmt.Errorf("could not export debug log"), session.Window)
				return
			}

			if uri == nil {
				return
			}

			if _, err = writeToURI(data, uri); err != nil {
				logger.Errorf("[Engram] Exporting debug log %s: %s\n", debugLogPath, err)
				dialog.ShowError(fmt.Errorf("could not export debug log"), session.Window)
				return
			}

			dialog.ShowInformation(i18n.T("settings.debug_log"), i18n.T("settings.debug_log_exported"), session.Window)
		}, session.Window)

		if !a.Driver().Device().IsMobile() {
			uri, err := storage.ListerForURI(storage.NewFileURI(AppPath()))
			if err == nil {
				dialogFileSave.SetLocation(uri)
			}
		}

		dialogFileSave.SetView(dialog.ListView)
		dialogFileSave.SetFileName(debugLogFileName)
		dialogFileSave.Resize(fyne.NewSize(ui.Width, ui.Height))
		dialogFileSave.Show()
	})

	// DATASHARD Section components
	labelDatashard := canvas.NewText(i18n.T("settings.datashard_section"), apptheme.C.Gray)
	labelDatashard.TextSize = scaleFont(11)
	labelDatashard.Alignment = fyne.TextAlignCenter
	labelDatashard.TextStyle = fyne.TextStyle{Bold: true}

	headerDatashard := canvas.NewText(i18n.T("settings.datashard_id"), apptheme.C.Gray)
	headerDatashard.TextSize = scaleFont(16)
	headerDatashard.Alignment = fyne.TextAlignCenter
	headerDatashard.TextStyle = fyne.TextStyle{Bold: true}

	address := engram.Disk.GetAddress().String()
	shardID := fmt.Sprintf("%x", sha1.Sum([]byte(address)))

	textDatashard := widget.NewRichTextFromMarkdown("### " + shardID)
	textDatashard.Wrapping = fyne.TextWrapWord

	textDatashardDesc := widget.NewRichTextFromMarkdown(i18n.T("settings.datashard_desc"))
	textDatashardDesc.Wrapping = fyne.TextWrapWord

	textDatashardDesc2 := widget.NewRichTextFromMarkdown(i18n.T("settings.datashard_examples"))
	textDatashardDesc2.Wrapping = fyne.TextWrapWord

	btnClearDatashard := widget.NewButton(i18n.T("settings.delete_datashard"), nil)
	btnClearDatashard.OnTapped = func() {
		header := canvas.NewText(i18n.T("settings.datashard_delete_request"), apptheme.C.Gray)
		header.TextSize = scaleFont(14)
		header.Alignment = fyne.TextAlignCenter
		header.TextStyle = fyne.TextStyle{Bold: true}

		subHeader := canvas.NewText(i18n.T("settings.are_you_sure"), apptheme.C.Account)
		subHeader.TextSize = scaleFont(22)
		subHeader.Alignment = fyne.TextAlignCenter
		subHeader.TextStyle = fyne.TextStyle{Bold: true}

		linkClose := widget.NewHyperlinkWithStyle(i18n.T("common.cancel"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkClose.OnTapped = func() {
			session.Datapad = ""
			session.DatapadChanged = false
			removeOverlays()
		}

		btnSubmit := widget.NewButton("Delete Datashard", nil)

		btnSubmit.OnTapped = func() {
			err := cleanWalletData()
			removeOverlays()
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("failed to delete datashard: %v", err), session.Window)
				} else {
					dialog.ShowInformation(i18n.T("common.success"), i18n.T("settings.datashard_deleted"), session.Window)
				}
			})
		}

		span := canvas.NewRectangle(color.Transparent)
		span.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

		overlay := session.Window.Canvas().Overlays()

		overlay.Add(
			container.NewStack(
				&iframe{},
				canvas.NewRectangle(apptheme.C.DarkMatter),
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
							linkClose,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
					),
				),
			),
		)
	}

	// Create EPOCH statistics section (conditionally hidden when offline)
	epochSection := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			epochTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rectWidth90,
					container.NewVBox(
						rectSpacer,
						wEpoch,
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochHashes,
							),
							container.NewStack(
								spacerEpoch,
								textEpochHashes,
							),
						),
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochBlocks,
							),
							container.NewStack(
								spacerEpoch,
								textEpochBlocks,
							),
						),
					),
				),
				layout.NewSpacer(),
			),
		),
	)

	if session.Offline {
		epochSection.Hide()
	}

	languageTitle := canvas.NewText(i18n.T("settings.language_section"), apptheme.C.Gray)
	languageTitle.TextSize = scaleFont(11)
	languageTitle.Alignment = fyne.TextAlignCenter
	languageTitle.TextStyle = fyne.TextStyle{Bold: true}

	themeTitle := canvas.NewText(i18n.T("settings.theme_section"), apptheme.C.Gray)
	themeTitle.TextSize = scaleFont(11)
	themeTitle.Alignment = fyne.TextAlignCenter
	themeTitle.TextStyle = fyne.TextStyle{Bold: true}

	advancedContent := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewVBox(
				advancedTitle,
			),
		),
		rectSpacer,
		rectSpacer,

		// GNOMON Section
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			gnomonTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		gnomonDescription,
		rectSpacer,
		checkGnomon,

		// NOTIFICATIONS Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			notifTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		notifDescription,
		rectSpacer,
		checkNotif,

		// LANGUAGE Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			languageTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.language_label")),
		rectSpacer,
		func() *fyne.Container {
			langNames := []string{}
			langCodes := i18n.LanguageOrder()
			for _, code := range langCodes {
				langNames = append(langNames, i18n.AvailableLanguages()[code])
			}
			wLang := widget.NewSelect(langNames, func(s string) {
				idx := 0
				for i, name := range langNames {
					if name == s {
						idx = i
						break
					}
				}
				if langCodes[idx] == i18n.GetLanguage() {
					return
				}
				i18n.SetLanguageFromIndex(idx)
				StoreValue("settings", []byte("language"), []byte(langCodes[idx]))
				settingsActiveTab = 2
				session.Window.SetContent(layoutAppSettings())
			})
			currentLang := i18n.GetLanguage()
			for i, code := range langCodes {
				if code == currentLang {
					wLang.SetSelectedIndex(i)
					break
				}
			}
			return container.NewStack(
				rectWidth90,
				wLang,
			)
		}(),

		// THEMES Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			themeTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		widget.NewRichTextFromMarkdown("### "+i18n.T("settings.theme_label")),
		rectSpacer,
		func() *fyne.Container {
			themeNames := []string{"Engram Classic", "Derotopia", "El Dorado", "Crystallina", "Atlantis"}
			themeKeys := []string{apptheme.ThemeEngram, apptheme.ThemeDerotopia, apptheme.ThemeElDorado, apptheme.ThemeCrystallina, apptheme.ThemeAtlantis}
			savedTheme := apptheme.ThemeEngram
			if data, err := GetValue("settings", []byte("theme")); err == nil && len(data) > 0 {
				savedTheme = string(data)
			}
			wTheme := widget.NewSelect(themeNames, func(s string) {
				idx := 0
				for i, name := range themeNames {
					if name == s {
						idx = i
						break
					}
				}
				key := themeKeys[idx]
				if key == savedTheme {
					return
				}
				StoreValue("settings", []byte("theme"), []byte(key))
				apptheme.Activate(key)
				// Clear daemon/miner icon tint caches so the next
				// syncStateIndicators tick re-tints SVGs with the new
				// theme palette.
				clearDaemonColoredCache()
				clearMinerColoredCache()
				a.Settings().SetTheme(apptheme.Main)
				UpdateThemeLogo()
				RasterizeEnigmaLogo()
				settingsActiveTab = 2
				session.Window.SetContent(layoutAppSettings())
			})
			for i, key := range themeKeys {
				if key == savedTheme {
					wTheme.SetSelectedIndex(i)
					break
				}
			}
			return container.NewStack(
				rectWidth90,
				wTheme,
			)
		}(),

		// SECURITY Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			func() fyne.CanvasObject {
				securityTitle := canvas.NewText(i18n.T("settings.security_section"), apptheme.C.Gray)
				securityTitle.TextSize = scaleFont(11)
				securityTitle.Alignment = fyne.TextAlignCenter
				securityTitle.TextStyle = fyne.TextStyle{Bold: true}
				return securityTitle
			}(),
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		func() fyne.CanvasObject {
			securityDesc := widget.NewRichTextFromMarkdown(i18n.T("settings.require_password"))
			securityDesc.Wrapping = fyne.TextWrapWord
			return securityDesc
		}(),
		rectSpacer,
		func() fyne.CanvasObject {
			checkPassword := widget.NewCheck("", func(b bool) {
				setPasswordForSend(b)
			})
			checkPassword.SetChecked(getPasswordForSend())
			return checkPassword
		}(),

		func() fyne.CanvasObject {
			if isMobile() {
				return rectSpacer
			}
			return container.NewStack(
				container.NewHBox(
					layout.NewSpacer(),
					container.NewStack(
						rectWidth90,
						container.NewVBox(
							rectSpacer,
							rectSpacer,
							container.NewHBox(
								layout.NewSpacer(),
								line1,
								layout.NewSpacer(),
								func() fyne.CanvasObject {
									logKeyTitle := canvas.NewText(i18n.T("settings.log_viewer_section"), apptheme.C.Gray)
									logKeyTitle.TextSize = scaleFont(11)
									logKeyTitle.Alignment = fyne.TextAlignCenter
									logKeyTitle.TextStyle = fyne.TextStyle{Bold: true}
									return logKeyTitle
								}(),
								layout.NewSpacer(),
								line2,
								layout.NewSpacer(),
							),
							rectSpacer,
							func() fyne.CanvasObject {
								desc := widget.NewRichTextFromMarkdown(i18n.T("settings.log_viewer_desc"))
								desc.Wrapping = fyne.TextWrapWord
								return desc
							}(),
							rectSpacer,
							func() fyne.CanvasObject {
								// Load saved key, default to backtick
								savedKey := "`"
								if data, err := GetValue("settings", []byte("log_viewer_key")); err == nil && len(data) > 0 {
									if len(data) == 1 {
										savedKey = string(data)
									}
								}

								entry := widget.NewEntry()
								entry.SetText(savedKey)
								entry.PlaceHolder = "`"
								entry.OnChanged = func(s string) {
									// Allow max 1 character — keep only the last rune entered
									if len([]rune(s)) > 1 {
										runes := []rune(s)
										last := string(runes[len(runes)-1:])
										entry.SetText(last)
									} else if len(s) > 0 {
										StoreValue("settings", []byte("log_viewer_key"), []byte(s))
										logger.Printf("[Settings] Log viewer key changed to: %s\n", s)
									}
								}

								return container.NewCenter(entry)
							}(),
							rectSpacer,
						),
					),
					layout.NewSpacer(),
				),
			)
		}(),

		// EPOCH STATISTICS Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			func() fyne.CanvasObject {
				epochTitle := canvas.NewText(i18n.T("settings.epoch_section"), apptheme.C.Gray)
				epochTitle.TextSize = scaleFont(11)
				epochTitle.Alignment = fyne.TextAlignCenter
				epochTitle.TextStyle = fyne.TextStyle{Bold: true}
				return epochTitle
			}(),
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rectWidth90,
					container.NewVBox(
						rectSpacer,
						wEpoch,
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochHashes,
							),
							container.NewStack(
								spacerEpoch,
								textEpochHashes,
							),
						),
						container.NewHBox(
							container.NewStack(
								spacerEpoch,
								labelEpochBlocks,
							),
							container.NewStack(
								spacerEpoch,
								textEpochBlocks,
							),
						),
					),
				),
				layout.NewSpacer(),
			),
		),

		// SCANNING Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			scanningTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		scanningDescription,
		rectSpacer,
		entryTrackBlocks,

		// DATASHARD Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			labelDatashard,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		container.NewStack(
			container.NewHBox(
				layout.NewSpacer(),
				container.NewStack(
					rectWidth90,
					container.NewVBox(
						rectSpacer,
						container.NewCenter(headerDatashard),
						rectSpacer,
						textDatashard,
						rectSpacer,
						textDatashardDesc,
						rectSpacer,
						textDatashardDesc2,
						rectSpacer,
						wrapMobileButton(btnClearDatashard),
					),
				),
				layout.NewSpacer(),
			),
		),

		// MAINTENANCE Section
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			maintenanceTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		wrapMobileButton(btnClearLocalData),
		rectSpacer,
		wrapMobileButton(btnRestoreDefaults),
		rectSpacer,
		wrapMobileButton(btnExportDebugLog),
		rectSpacer,
	)

	// Create the tab container with width constraint
	tabs := container.NewAppTabs(
		container.NewTabItem(i18n.T("settings.remote"), remoteAccessContent),
		container.NewTabItem(i18n.T("settings.tela"), telaContent),
		container.NewTabItem(i18n.T("settings.advanced"), advancedContent),
	)

	// Select default tab based on how we navigated here
	if previousDomain == "app.tela.settings" || previousDomain == "app.tela.manager.settings" {
		tabs.SelectIndex(1) // TELA tab
		settingsActiveTab = 1
	} else if previousDomain == "app.appsettings" {
		tabs.SelectIndex(settingsActiveTab)
	} else {
		tabs.SelectIndex(0) // Default to Remote Access
		settingsActiveTab = 0
	}

	tabs.OnSelected = func(tab *container.TabItem) {
		for i, t := range tabs.Items {
			if t == tab {
				settingsActiveTab = i
				break
			}
		}
	}

	// Wrap tabs in a container with fixed width
	tabsContainer := container.NewStack(
		rectWidth,
		tabs,
	)

	// Back button to return to previous screen (dashboard or TELA)
	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())

		// Use the tracked caller domain to handle returning from sub-pages properly
		targetDomain := previousDomain
		if targetDomain == "app.remoteaccess.manager" || targetDomain == "app.remoteaccess.permissions" {
			targetDomain = settingsCallerDomain
		}

		// Return to TELA if user came from there, otherwise dashboard
		if targetDomain == "app.tela" || targetDomain == "app.tela.settings" {
			session.Window.SetContent(layoutTELA())
		} else if targetDomain == "app.tela.manager" || targetDomain == "app.tela.manager.settings" {
			if cachedTelaManagerContent != nil {
				session.Domain = "app.tela.manager"
				session.Window.SetContent(cachedTelaManagerContent)
			} else {
				session.Window.SetContent(layoutDashboard())
			}
		} else {
			session.Window.SetContent(layoutDashboard())
		}
		removeOverlays()
	})

	// Main content area matching layoutSettings pattern
	formSettings := container.NewVBox(
		rectSpacer,
		rectSpacer,
		tabsContainer,
	)

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectScroll,
				formSettings,
			),
			layout.NewSpacer(),
		),
	)

	top := container.NewVBox(
		rectSpacer,
		heading,
		rectSpacer,
	)

	footer := container.NewStack(
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

	c := container.NewBorder(
		top,
		footer,
		nil,
		nil,
		scrollBox,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	// Register with navigation stack (app settings allows back navigation)
	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(layout)
}

func layoutNetwork() fyne.CanvasObject {
	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	session.Domain = "app.network"

	frame := &iframe{}

	rectScroll := canvas.NewRectangle(color.Transparent)
	rectScroll.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.8))
	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	labelNode := canvas.NewText(i18n.T("settings.connection"), apptheme.C.Gray)
	labelNode.TextStyle = fyne.TextStyle{Bold: true}
	labelNode.TextSize = scaleFont(14)

	type NodeItem struct {
		Address string
		Status  string
	}

	mainnetNodes := []NodeItem{
		{Address: "127.0.0.1:10102", Status: "unknown"},
		{Address: "dero.rabidmining.com:10102", Status: "unknown"},
		{Address: "dero-node.net:10102", Status: "unknown"},
		{Address: "community-pools.mysrv.cloud:10102", Status: "unknown"},
		{Address: "node.derofoundation.org:11012", Status: "unknown"},
	}
	testnetNodes := []NodeItem{
		{Address: "69.30.234.163:40402", Status: "unknown"},
		{Address: "testnet.derofoundation.co:40402", Status: "unknown"},
		{Address: "127.0.0.1:40402", Status: "unknown"},
	}
	simulatorNodes := []NodeItem{
		{Address: "127.0.0.1:20000", Status: "unknown"},
	}

	getNodesKey := func(network string) string {
		switch network {
		case NETWORK_TESTNET:
			return "testnet_nodes"
		case NETWORK_SIMULATOR:
			return "simulator_nodes"
		default:
			return "mainnet_nodes"
		}
	}

	getDefaultNodes := func(network string) []NodeItem {
		switch network {
		case NETWORK_TESTNET:
			return testnetNodes
		case NETWORK_SIMULATOR:
			return simulatorNodes
		default:
			return mainnetNodes
		}
	}

	loadNodesForNetwork := func(network string) []NodeItem {
		nodesKey := getNodesKey(network)
		if data, err := GetValue("settings", []byte(nodesKey)); err == nil && len(data) > 0 {
			var savedNodes []NodeItem
			if err := json.Unmarshal(data, &savedNodes); err == nil && len(savedNodes) > 0 {
				return savedNodes
			}
		}
		return getDefaultNodes(network)
	}

	nodeData := loadNodesForNetwork(session.Network)
	nodeContainer := container.NewVBox()

	var updateNodeContainer func()

	updateNodeContainer = func() {
		nodeContainer.Objects = nil

		for i := range nodeData {
			i := i
			item := &nodeData[i]

			var iconResource fyne.Resource
			switch item.Status {
			case "connected":
				iconResource = theme.ConfirmIcon()
			case "failed":
				iconResource = theme.CancelIcon()
			}

			rowIcon := widget.NewIcon(iconResource)

			removeBtn := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
				if len(nodeData) <= 1 {
					return
				}

				removedAddress := item.Address
				wasConnected := item.Status == "connected" || getDaemon() == removedAddress

				nodeData = append(nodeData[:i], nodeData[i+1:]...)

				if wasConnected {
					newIndex := i - 1
					if newIndex < 0 {
						newIndex = 0
					}
					if newIndex >= len(nodeData) {
						newIndex = len(nodeData) - 1
					}
					nodeData[newIndex].Status = "connected"
					setDaemon(nodeData[newIndex].Address)

					for j := range nodeData {
						if j != newIndex {
							nodeData[j].Status = "unknown"
						}
					}
				}

				if data, err := json.Marshal(nodeData); err == nil {
					StoreValue("settings", []byte(getNodesKey(session.Network)), data)
				}

				updateNodeContainer()
			})
			removeBtn.Importance = widget.MediumImportance
			if len(nodeData) <= 1 {
				removeBtn.Disable()
			}

			addressLabel := widget.NewLabel(item.Address)
			addressLabel.Truncation = fyne.TextTruncateEllipsis

			row := container.NewBorder(
				nil, nil, nil,
				container.NewHBox(rowIcon, wrapMobileButton(removeBtn)),
				addressLabel,
			)

			tapBtn := widget.NewButton("", func() {
				if testNodeConnection(item.Address) {
					item.Status = "connected"
					setDaemon(item.Address)

					for j := range nodeData {
						if j != i {
							nodeData[j].Status = "unknown"
						}
					}

					if data, err := json.Marshal(nodeData); err == nil {
						StoreValue("settings", []byte(getNodesKey(session.Network)), data)
					}
				} else {
					item.Status = "failed"
				}
				updateNodeContainer()
			})
			tapBtn.Importance = widget.LowImportance
			tapBtn.Alignment = widget.ButtonAlignLeading
			tapBtn.Text = ""

			clickableRow := container.NewMax(
				wrapMobileButton(tapBtn),
				row,
			)

			nodeContainer.Add(clickableRow)
		}
		nodeContainer.Refresh()
	}

	currentDaemon := getDaemon()
	for i := range nodeData {
		if nodeData[i].Address == currentDaemon {
			nodeData[i].Status = "connected"
		}
	}
	updateNodeContainer()

	getNodePlaceholder := func(network string) string {
		switch network {
		case NETWORK_TESTNET:
			return "hostname:40402"
		case NETWORK_SIMULATOR:
			return "hostname:20000"
		default:
			return "hostname:10102"
		}
	}

	entryCustomNode := widget.NewEntry()
	entryCustomNode.PlaceHolder = getNodePlaceholder(session.Network)

	showNodeError := func(err error) {
		entryCustomNode.Validator = func(s string) error {
			return err
		}
		entryCustomNode.SetValidationError(err)
		entryCustomNode.FocusGained()
		entryCustomNode.FocusLost()
	}

	clearNodeError := func() {
		entryCustomNode.Validator = nil
		entryCustomNode.SetValidationError(nil)
		entryCustomNode.Refresh()
	}

	btnAddNode := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		nodeAddress := strings.TrimSpace(entryCustomNode.Text)
		nodeAddress = strings.ReplaceAll(nodeAddress, " ", "")
		if nodeAddress != "" {
			for _, node := range nodeData {
				if node.Address == nodeAddress {
					showNodeError(errors.New("node already exists"))
					return
				}
			}

			if testNodeConnectionTimeout(nodeAddress, 500*time.Millisecond) {
				clearNodeError()

				for i := range nodeData {
					nodeData[i].Status = "unknown"
				}

				nodeData = append(nodeData, NodeItem{
					Address: nodeAddress,
					Status:  "connected",
				})
				setDaemon(nodeAddress)

				if data, err := json.Marshal(nodeData); err == nil {
					StoreValue("settings", []byte(getNodesKey(session.Network)), data)
				}

				entryCustomNode.Text = ""
				entryCustomNode.Refresh()
				updateNodeContainer()
			} else {
				showNodeError(errors.New("node unreachable"))
			}
		}
	})
	btnAddNode.Importance = widget.MediumImportance
	btnAddNode.Disable()

	entryCustomNode.OnChanged = func(s string) {
		clearNodeError()
		s = strings.TrimSpace(s)
		if s != "" {
			btnAddNode.Enable()
		} else {
			btnAddNode.Disable()
		}
	}

	entrySection := container.NewBorder(nil, nil, nil, wrapMobileButton(btnAddNode), entryCustomNode)
	entryWrapper := container.NewStack(
		canvas.NewRectangle(color.Transparent),
		entrySection,
	)
	entryWrapper.Resize(fyne.NewSize(ui.Width*0.9, 35))

	sep1 := canvas.NewRectangle(apptheme.C.Gray)
	sep1.SetMinSize(fyne.NewSize(ui.Width*0.3, 2))
	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep1,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(apptheme.C.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.3, 2))
	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)

	statusAreaTitle := canvas.NewText(i18n.T("settings.status_area"), apptheme.C.Gray)
	statusAreaTitle.TextSize = scaleFont(11)
	statusAreaTitle.Alignment = fyne.TextAlignCenter
	statusAreaTitle.TextStyle = fyne.TextStyle{Bold: true}

	prioritiseDesc := widget.NewRichTextFromMarkdown(i18n.T("settings.prioritise_status_desc"))
	prioritiseDesc.Wrapping = fyne.TextWrapWord

	prioritiseCheck := widget.NewCheck(i18n.T("settings.prioritise_status"), func(b bool) {
		setPrioritiseStatus(b)
	})
	prioritiseCheck.SetChecked(getPrioritiseStatus())

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	formNetwork := container.NewVBox(
		rectSpacer,
		labelNode,
		rectSpacer,
		rectSpacer,
		entryWrapper,
		rectSpacer,
		nodeContainer,
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			statusAreaTitle,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		rectSpacer,
		prioritiseDesc,
		rectSpacer,
		prioritiseCheck,
	)

	scrollBox := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewStack(
				rectScroll,
				formNetwork,
			),
			layout.NewSpacer(),
		),
	)

	scrollBox.SetMinSize(fyne.NewSize(ui.MaxWidth, ui.MaxHeight*0.8))

	if isMobile() {
		SetCurrentScrollBox(scrollBox)
	}

	heading := canvas.NewText(strings.ToUpper(session.Network), apptheme.C.Green)
	heading.TextSize = scaleFont(22)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(heading),
		rectSpacer,
	)

	footer := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(1), btnBack),
			),
			rectSpacer,
		),
	)

	c := container.NewBorder(
		top,
		footer,
		nil,
		nil,
		scrollBox,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	if session.NavStack != nil {
		session.NavStack.Push(session.Domain, true)
	}

	return NewVScroll(layout)
}

func layoutRemoteAccess() fyne.CanvasObject {
	session.Domain = "app.remoteaccess"

	go refreshXSWDList()

	wSpacer := widget.NewLabel(" ")

	title := canvas.NewText(i18n.T("settings.remote_access_heading"), apptheme.C.Gray)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = scaleFont(16)

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width, ui.Height*0.20))

	frame := &iframe{}

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(standardSpacerSize())

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, scaleSize(0)))

	rpcLabel := canvas.NewText(i18n.T("settings.rpc_config"), apptheme.C.Gray)
	rpcLabel.TextSize = scaleFont(11)
	rpcLabel.Alignment = fyne.TextAlignCenter
	rpcLabel.TextStyle = fyne.TextStyle{Bold: true}

	wsLabel := canvas.NewText(i18n.T("settings.rpc_config"), apptheme.C.Gray)
	wsLabel.TextSize = scaleFont(11)
	wsLabel.Alignment = fyne.TextAlignCenter
	wsLabel.TextStyle = fyne.TextStyle{Bold: true}

	labelConnections := canvas.NewText(i18n.T("settings.connections"), apptheme.C.Gray)
	labelConnections.TextSize = scaleFont(11)
	labelConnections.Alignment = fyne.TextAlignCenter
	labelConnections.TextStyle = fyne.TextStyle{Bold: true}

	sep1 := canvas.NewRectangle(apptheme.C.Gray)
	sep1.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep1,
		layout.NewSpacer(),
	)

	sep2 := canvas.NewRectangle(apptheme.C.Gray)
	sep2.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep2,
		layout.NewSpacer(),
	)
	_ = line1
	_ = line2

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Window.SetContent(layoutTransition())
		if session.LastDomain != nil {
			session.Window.SetContent(session.LastDomain)
		} else {
			session.Window.SetContent(layoutDashboard())
		}
		removeOverlays()
	})

	shortShard := canvas.NewText(i18n.T("settings.app_connections"), apptheme.C.Gray)
	shortShard.TextStyle = fyne.TextStyle{Bold: true}
	shortShard.TextSize = scaleFont(12)

	linkColor := apptheme.C.Green

	if remoteAccess.RPC.server == nil {
		session.Link = i18n.T("settings.blocked")
		linkColor = apptheme.C.Gray
	}

	remoteAccess.RPC.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.RPC.status.TextSize = scaleFont(22)
	remoteAccess.RPC.status.TextStyle = fyne.TextStyle{Bold: true}

	serverStatus := canvas.NewText(i18n.T("settings.app_connections"), apptheme.C.Gray)
	serverStatus.TextSize = scaleFont(12)
	serverStatus.Alignment = fyne.TextAlignCenter
	serverStatus.TextStyle = fyne.TextStyle{Bold: true}

	linkCenter := container.NewCenter(
		remoteAccess.RPC.status,
	)

	remoteAccess.RPC.userText = widget.NewEntry()
	remoteAccess.RPC.userText.PlaceHolder = i18n.T("settings.username")
	remoteAccess.RPC.userText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.user = s
		}
	}

	remoteAccess.RPC.passText = widget.NewEntry()
	remoteAccess.RPC.passText.Password = true
	remoteAccess.RPC.passText.PlaceHolder = i18n.T("settings.password")
	remoteAccess.RPC.passText.OnChanged = func(s string) {
		if len(s) > 1 {
			remoteAccess.RPC.pass = s
			StoreValue("settings", []byte("rpc_pass"), []byte(s))
		}
	}

	remoteAccess.RPC.portText = widget.NewEntry()
	remoteAccess.RPC.portText.PlaceHolder = i18n.T("settings.rpc_placeholder")
	remoteAccess.RPC.portText.Validator = func(s string) (err error) {
		regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
		test := regexp.MustCompile(regex)
		if test.MatchString(s) {
			remoteAccess.RPC.portText.SetValidationError(nil)
		} else {
			err = errors.New("invalid host name")
			remoteAccess.RPC.portText.SetValidationError(err)
		}

		return
	}
	remoteAccess.RPC.portText.SetText(getRemoteAccess("RPC"))

	linkColor = apptheme.C.Green

	if remoteAccess.WS.server == nil {
		session.Link = i18n.T("settings.blocked")
		linkColor = apptheme.C.Gray
	}

	remoteAccess.WS.status = canvas.NewText(session.Link, linkColor)
	remoteAccess.WS.status.TextSize = scaleFont(22)
	remoteAccess.WS.status.TextStyle = fyne.TextStyle{Bold: true}

	deckChoice := widget.NewSelect([]string{i18n.T("settings.ws_label"), i18n.T("settings.rpc_label")}, nil)

	remoteAccess.RPC.toggle = widget.NewButton(i18n.T("settings.turn_on"), nil)
	remoteAccess.RPC.toggle.OnTapped = func() {
		switch session.Network {
		case NETWORK_TESTNET:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_TESTNET_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		case NETWORK_SIMULATOR:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_SIMULATOR_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		default:
			if remoteAccess.RPC.portText.Validate() != nil {
				remoteAccess.RPC.port = fmt.Sprintf("127.0.0.1:%d", DEFAULT_WALLET_PORT)
				remoteAccess.RPC.portText.SetText(remoteAccess.RPC.port)
			} else {
				remoteAccess.RPC.port = remoteAccess.RPC.portText.Text
			}
		}

		toggleRPCServer(remoteAccess.RPC.port)
		if remoteAccess.RPC.server != nil {
			setRemoteAccess(remoteAccess.RPC.port, "RPC")
			deckChoice.Disable()
			remoteAccess.RPC.portText.Disable()
		} else {
			deckChoice.Enable()
			remoteAccess.RPC.portText.Enable()
		}
	}

	if remoteAccess.WS.portText == nil {
		remoteAccess.WS.portText = widget.NewEntry()
		remoteAccess.WS.portText.PlaceHolder = i18n.T("settings.ws_placeholder")
		remoteAccess.WS.portText.Validator = func(s string) (err error) {
			regex := `^(?:[a-zA-Z0-9]{1,62}(?:[-\.][a-zA-Z0-9]{1,62})+)(:\d+)?$`
			test := regexp.MustCompile(regex)
			if test.MatchString(s) {
				remoteAccess.WS.portText.SetValidationError(nil)
			} else {
				err = errors.New("invalid host name")
				remoteAccess.WS.portText.SetValidationError(err)
			}

			return
		}

		remoteAccess.WS.portText.OnChanged = func(s string) {
			if remoteAccess.WS.portText.Validate() == nil {
				remoteAccess.WS.port = s
				setRemoteAccessDual(s, "WS") // Use dual storage instead of setRemoteAccess()

				// CRITICAL FIX: Save WebSocket enabled state to storage
				remoteAccess.WS.global.enabled = true
				setPermissions()
			}
		}
	}

	remoteAccess.WS.toggle = widget.NewButton(i18n.T("settings.turn_on"), nil)
	remoteAccess.WS.toggle.OnTapped = func() {
		if remoteAccess.WS.portText.Validate() != nil {
			remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
			remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
		} else {
			_, err := net.ResolveTCPAddr("tcp", remoteAccess.WS.port)
			if err != nil {
				logger.Errorf("[Remote Access] XSWD port: %s\n", err)
				remoteAccess.WS.port = fmt.Sprintf("127.0.0.1:%d", xswd.XSWD_PORT)
				remoteAccess.WS.portText.SetText(remoteAccess.WS.port)
			} else {
				remoteAccess.WS.port = remoteAccess.WS.portText.Text
			}
		}

		remoteAccess.EPOCH.err = nil
		toggleXSWD(remoteAccess.WS.port)
		if remoteAccess.WS.server != nil {
			setRemoteAccessDual(remoteAccess.WS.port, "WS") // Use dual storage for consistency
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
			if remoteAccess.EPOCH.enabled {
				/*
					if remoteAccess.EPOCH.allowWithAddress {
						// If address is defined by dApp, GetWork will be started and stopped upon each WS call
						logger.Printf("[EPOCH] dApp addresses are enabled\n")
						return
					}
				*/

				err := epoch.StartGetWork(engram.Disk.GetAddress().String(), session.Daemon)
				if err != nil {
					logger.Errorf("[EPOCH] Connecting: %s\n", err)
					remoteAccess.EPOCH.err = err
				} else {
					remoteAccess.EPOCH.err = nil
					setRemoteAccess(epoch.GetPort(), "EPOCH")
				}
			}
		} else {
			stopEPOCH()
			remoteAccess.WS.portText.Enable()
			deckChoice.Enable()
		}
	}

	if session.Offline {
		remoteAccess.RPC.toggle.Text = i18n.T("settings.disabled_offline")
		remoteAccess.RPC.toggle.Disable()
		remoteAccess.RPC.portText.Disable()
		remoteAccess.WS.toggle.Text = i18n.T("settings.disabled_offline")
		remoteAccess.WS.toggle.Disable()
		remoteAccess.WS.portText.Disable()
	} else {
		if remoteAccess.RPC.server != nil {
			remoteAccess.RPC.status.Text = i18n.T("settings.allowed")
			remoteAccess.RPC.status.Color = apptheme.C.Green
			remoteAccess.RPC.toggle.Text = i18n.T("settings.turn_off")
			remoteAccess.RPC.userText.Disable()
			remoteAccess.RPC.passText.Disable()
			remoteAccess.RPC.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.RPC.status.Text = i18n.T("settings.blocked")
			remoteAccess.RPC.status.Color = apptheme.C.Gray
			remoteAccess.RPC.toggle.Text = i18n.T("settings.turn_on")
			remoteAccess.RPC.userText.Enable()
			remoteAccess.RPC.passText.Enable()
			remoteAccess.RPC.portText.Enable()
		}

		if remoteAccess.WS.server != nil {
			remoteAccess.WS.status.Text = i18n.T("settings.allowed")
			remoteAccess.WS.status.Color = apptheme.C.Green
			remoteAccess.WS.toggle.Text = i18n.T("settings.turn_off")
			remoteAccess.WS.portText.Disable()
			deckChoice.Disable()
		} else {
			remoteAccess.WS.status.Text = i18n.T("settings.blocked")
			remoteAccess.WS.status.Color = apptheme.C.Gray
			remoteAccess.WS.toggle.Text = i18n.T("settings.turn_on")
			remoteAccess.WS.portText.Enable()
		}
	}

	remoteAccess.RPC.userText.SetText(remoteAccess.RPC.user)
	remoteAccess.RPC.passText.SetText(remoteAccess.RPC.pass)

	linkCopy := widget.NewHyperlinkWithStyle(i18n.T("settings.copy_creds"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkCopy.OnTapped = func() {
		a.Clipboard().SetContent(remoteAccess.RPC.user + ":" + remoteAccess.RPC.pass)
	}

	linkPermissions := widget.NewHyperlinkWithStyle(i18n.T("settings.title"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkPermissions.OnTapped = func() {
		//if remoteAccess.WS.server != nil {
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDPermissions())
		removeOverlays()
		//}
	}

	/*
		linkApps := widget.NewHyperlinkWithStyle("View Connections", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		linkApps.OnTapped = func() {
			if remoteAccess.WS.server != nil {
				session.LastDomain = session.Window.Content()
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutXSWDConnections())
				removeOverlays()
			}
		}
	*/

	remoteAccess.WS.list = widget.NewList(
		func() int {
			return len(remoteAccess.WS.apps)
		},
		func() fyne.CanvasObject {
			return container.NewVBox(
				widget.NewLabel(""),
				//widget.NewLabel(""),
			)
		},
		func(li widget.ListItemID, co fyne.CanvasObject) {
			app := remoteAccess.WS.apps[li]

			fyne.Do(func() {
				co.(*fyne.Container).Objects[0].(*widget.Label).SetText(app.Name)
				//co.(*fyne.Container).Objects[1].(*widget.Label).SetText(app.Id)
			})
		},
	)

	remoteAccess.WS.list.OnSelected = func(id widget.ListItemID) {
		remoteAccess.WS.list.UnselectAll()
		remoteAccess.WS.list.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutXSWDAppManager(&remoteAccess.WS.apps[id]))
		removeOverlays()
	}

	xswdForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			wsLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					container.NewCenter(
						remoteAccess.WS.status,
					),
					rectSpacer,
					serverStatus,
					wSpacer,
					remoteAccess.WS.toggle,
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkPermissions,
						layout.NewSpacer(),
					),
				),
			),
		),
		container.NewStack(
			rectWidth90,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					labelConnections,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewStack(
						rect,
						remoteAccess.WS.list,
					),
				),
			),
		),
		layout.NewSpacer(),
	)

	rpcForm := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			line1,
			layout.NewSpacer(),
			rpcLabel,
			layout.NewSpacer(),
			line2,
			layout.NewSpacer(),
		),
		container.NewCenter(
			layout.NewSpacer(),
			container.NewCenter(
				container.NewVBox(
					rectWidth90,
					rectSpacer,
					linkCenter,
					rectSpacer,
					serverStatus,
					wSpacer,
					remoteAccess.RPC.toggle,
					wSpacer,
					remoteAccess.RPC.portText,
					rectSpacer,
					remoteAccess.RPC.userText,
					rectSpacer,
					remoteAccess.RPC.passText,
					wSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkCopy,
						layout.NewSpacer(),
					),
				),
			),
			layout.NewSpacer(),
		),
	)

	deckFeatures := container.NewStack()
	if remoteAccess.RPC.server != nil {
		deckFeatures.Add(rpcForm)
		deckChoice.SetSelectedIndex(1)
	} else {
		deckFeatures.Add(xswdForm)
		deckChoice.SetSelectedIndex(0)
	}

	deckChoice.OnChanged = func(s string) {
		if s == i18n.T("settings.rpc_label") {
			deckFeatures.Objects[0] = rpcForm
		} else {
			deckFeatures.Objects[0] = xswdForm
		}
	}

	deckForm := container.NewVScroll(
		container.NewStack(
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewVBox(
						title,
					),
				),
				rectSpacer,
				rectSpacer,
				container.NewCenter(
					container.NewStack(
						rectWidth90,
						deckChoice,
					),
				),
				container.NewBorder(
					deckFeatures,
					nil,
					nil,
					nil,
				),
			),
		),
	)

	deckForm.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.80))

	session.Window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyLeft {
			session.Dashboard = "main"
			session.Window.SetContent(layoutTransition())
			session.Window.SetContent(layoutDashboard())
			removeOverlays()
		}
	})

	subContainer := container.NewStack(
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

	c := container.NewBorder(
		deckForm,
		subContainer,
		nil,
		nil,
	)

	layout := container.NewStack(
		frame,
		c,
	)

	return NewVScroll(layout)
}

// Layout details of an app connected through web socket
