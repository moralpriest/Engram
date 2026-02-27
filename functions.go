// Copyright 2023-2024 DERO Foundation. All rights reserved.
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
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	x "fyne.io/x/fyne/widget"
	"github.com/civilware/Gnomon/indexer"
	"github.com/civilware/Gnomon/rwc"
	"github.com/civilware/Gnomon/structures"
	"github.com/civilware/epoch"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
	"github.com/civilware/tela/shards"
	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	"github.com/gorilla/websocket"
	"mvdan.cc/xurls/v2"

	"github.com/civilware/Gnomon/storage"
	"github.com/deroproject/derohe/config"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/dvm"
	"github.com/deroproject/derohe/globals"

	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/transaction"

	"github.com/deroproject/derohe/walletapi"
	"github.com/deroproject/derohe/walletapi/mnemonics"
	"github.com/deroproject/derohe/walletapi/rpcserver"
	"github.com/deroproject/derohe/walletapi/xswd"
)

type App struct {
	App    fyne.App
	Window fyne.Window
	Focus  bool
}

type UI struct {
	Padding   float32
	MaxWidth  float32
	Width     float32
	MaxHeight float32
	Height    float32
}

type Colors struct {
	Network    color.Color
	Account    color.Color
	Blue       color.Color
	Red        color.Color
	DarkGreen  color.Color
	Green      color.Color
	Gray       color.Color
	Yellow     color.Color
	DarkMatter color.Color
	Cold       color.Color
	Flint      color.Color
}

type Navigation struct {
	PosX float32
	PosY float32
	CurX float32
	CurY float32
}

type Session struct {
	Window            fyne.Window
	DesktopMode       bool
	Domain            string
	LastDomain        fyne.CanvasObject
	Network           string
	Offline           bool
	Language          int
	ID                string
	Link              string
	Type              string
	Daemon            string
	WalletOpen        bool
	Username          string
	Datapad           string
	DatapadChanged    bool
	LastBalance       uint64
	Balance           uint64
	BalanceHidden     bool
	AddressHidden     bool
	BalanceUSD        string
	BalanceText       *canvas.Text
	BalanceUSDText    *canvas.Text
	ModeText          *canvas.Text
	IDText            *canvas.Text
	LinkText          *canvas.Text
	StatusText        *canvas.Text
	Path              string
	Name              string
	Password          string
	PasswordConfirm   string
	DaemonHeight      uint64
	WalletHeight      uint64
	RPCServer         *rpcserver.RPCServer
	Verified          bool
	Dashboard         string
	Error             string
	NewUser           string
	Gif               *x.AnimatedGif
	RegHashes         int64
	LimitMessages     bool
	TrackRecentBlocks int64
}

type Cyberdeck struct {
	RPC struct {
		user     string
		pass     string
		port     string
		userText *widget.Entry
		passText *widget.Entry
		portText *widget.Entry
		toggle   *widget.Button
		status   *canvas.Text
		server   *rpcserver.RPCServer
	}
	WS struct {
		sync.RWMutex
		port     string
		portText *widget.Entry
		list     *widget.List
		toggle   *widget.Button
		status   *canvas.Text
		server   *xswd.XSWD
		apps     []xswd.ApplicationData
		advanced bool
		global   struct {
			connect     bool
			enabled     bool
			status      *canvas.Text
			permissions map[string]xswd.Permission
		}
	}
	EPOCH struct {
		enabled          bool
		allowWithAddress bool
		err              error
		total            epoch.GetSessionEPOCH_Result
	}
}

type INDEXwithRatings struct {
	ratings tela.Rating_Result
	tela.INDEX
}

type Engram struct {
	Disk *walletapi.Wallet_Disk
}

type Theme struct {
	main eTheme
	alt  eTheme2
}

type Gnomon struct {
	Active   int
	Index    *indexer.Indexer
	BBolt    *storage.BboltStore
	Graviton *storage.GravitonStore
	Path     string
}

type ProofData struct {
	Receivers []string
	Amounts   []uint64
	Payloads  []string
}

type Status struct {
	Canvas     *canvas.Text
	Message    string
	Network    *canvas.Text
	Connection *canvas.Circle
	Sync       *canvas.Circle
	Cyberdeck  *canvas.Circle
	Gnomon     *canvas.Circle
	EPOCH      *canvas.Circle
}

type Transfers struct {
	Address    *rpc.Address
	PaymentID  uint64
	Amount     uint64
	Comment    string
	GasStorage uint64
	Fees       uint64
	Pending    []rpc.Transfer
	TX         *transaction.Transaction
	TXID       crypto.Hash
	Proof      string
	Ringsize   uint64
	SendAll    bool
	Size       float32
	Status     string
	OfflineTX  bool
	Filename   string
}

type TELAFavoriteData struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	IconURL     string  `json:"icon_url"`
	Rating      float64 `json:"rating"`
	LastUpdated int64   `json:"last_updated"`
}

type MessageBox struct {
	List *widget.List
	Data binding.ExternalStringList
}

type Messages struct {
	Contact string
	Address string
	Data    []string
	Height  uint64
	Message string
}

// PermissionGroup represents a grouped set of permission methods for Simple Mode
type PermissionGroup struct {
	Name        string
	Description string
	Methods     []string
	SimpleMode  bool   // Show in simple mode
	Category    string // "transactions", "readonly", "sensitive", "mining", "tela", "utility"
}

// PermissionPresets defines default behaviors for Simple and Advanced modes
type PermissionPresets struct {
	Simple   map[string]xswd.Permission // Simple mode defaults
	Advanced map[string]xswd.Permission // Advanced mode defaults (current behavior)
}

// PermissionGroups defines the 6 user-friendly groups for Simple Mode
var permissionGroups = []PermissionGroup{
	{
		Name:        "Send Transactions",
		Description: "Allow apps to send DERO/assets from your wallet",
		Methods:     []string{"Transfer", "transfer_split", "scinvoke"},
		SimpleMode:  true,
		Category:    "transactions",
	},
	{
		Name:        "View Wallet Info",
		Description: "Allow apps to see your address and balance",
		Methods: []string{
			"GetAddress", "GetAddressEPOCH", "getaddress",
			"GetBalance", "getbalance",
			"GetHeight", "getheight",
			"MakeIntegratedAddress", "make_integrated_address",
			"SplitIntegratedAddress", "split_integrated_address",
		},
		SimpleMode: true,
		Category:   "readonly",
	},
	{
		Name:        "View Transaction History",
		Description: "Allow apps to see your transaction records",
		Methods: []string{
			"GetTransfers", "get_transfers",
			"GetTransferbyTXID", "get_transfer_by_txid",
		},
		SimpleMode: true,
		Category:   "sensitive",
	},
	{
		Name:        "Sign Messages",
		Description: "Allow apps to request cryptographic signatures",
		Methods: []string{
			"SignData", "CheckSignature",
			"QueryKey", "query_key",
		},
		SimpleMode: true,
		Category:   "sensitive",
	},
	{
		Name:        "EPOCH Mining",
		Description: "Allow apps to use your device for mining",
		Methods: []string{
			"AttemptEPOCH", "AttemptEPOCHWithAddr",
			"SubmitEPOCH", "GetSessionEPOCH", "GetMaxHashesEPOCH",
		},
		SimpleMode: true,
		Category:   "mining",
	},
	{
		Name:        "TELA & Links",
		Description: "Allow apps to open TELA links and content",
		Methods: []string{
			"HandleTELALinks", "Subscribe", "Unsubscribe",
		},
		SimpleMode: true,
		Category:   "tela",
	},
	// Hidden groups (SimpleMode: false) - auto-handled
	{
		Name:        "Utility Methods",
		Description: "Read-only utility methods (auto-allowed)",
		Methods:     []string{"Echo", "HasMethod", "GetDaemon", "GetPrimaryUsername"},
		SimpleMode:  false,
		Category:    "utility",
	},
}

type InstallContract struct {
	TXID string
}

type Client struct {
	WS  *websocket.Conn
	RPC *jrpc2.Client
}

type ScanProgress struct {
	Position  int    `json:"position"`
	Total     int    `json:"total"`
	LastSCID  string `json:"last_scid"`
	State     string `json:"state"` // "syncing", "scanning", "completed", "interrupted"
	Timestamp int64  `json:"timestamp"`
}

// Get the Engram settings from the local Graviton tree
func initSettings() {
	logger.Printf("[Engram] initSettings() called - starting initialization")
	getNetwork()
	getMode()
	getDaemon()
	getGnomon()
	getRPCCredentials()

	// Load and apply TELA settings on startup
	initTELAPreferences()

	// CRITICAL FIX: Initialize WebSocket state FIRST, independent of Gnomon
	// This ensures WebSocket settings load even if Gnomon fails
	logger.Printf("[Engram] Initializing WebSocket state (independent of Gnomon)")
	initWebSocketState()
	logger.Printf("[Engram] WebSocket state initialization completed")

	// NOW initialize Gnomon with error handling so failure doesn't prevent other features
	logger.Printf("[Engram] Initializing Gnomon database...")
	getGnomon()
	logger.Printf("[Engram] initSettings() completed")

	if a.Driver().Device().IsMobile() {
		err := tela.SetShardPath(filepath.Join(AppPath(), filepath.Dir(shards.GetPath())))
		if err != nil {
			logger.Errorf("[Engram] Setting TELA shard: %s\n", err)
			return
		}

		os.RemoveAll(tela.GetPath())
	}
}

// Store TELA setting with dual storage (encrypted + unencrypted fallback)
func setTELADual(key string, value []byte) {
	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err := StoreEncryptedValue("TELA Settings", []byte(key), value)
		if err != nil {
			logger.Debugf("[Engram] setTELADual encrypted storage failed for %s: %s\n", key, err)
		} else {
			logger.Printf("[Engram] setTELADual: Successfully saved %s to encrypted storage", key)
		}
	}

	// Always save to unencrypted storage as fallback
	err := StoreValue("TELASettingsUnencrypted", []byte(key), value)
	if err != nil {
		logger.Debugf("[Engram] setTELADual unencrypted storage failed for %s: %s\n", key, err)
	} else {
		logger.Printf("[Engram] setTELADual: Successfully saved %s to fallback storage", key)
	}
}

// Get TELA setting with dual storage (try encrypted first, then fallback)
func getTELADual(key string) (value string, found bool) {
	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		stored, err := GetEncryptedValue("TELA Settings", []byte(key))
		if err == nil && stored != nil {
			logger.Printf("[Engram] getTELADual: Successfully loaded %s from encrypted storage", key)
			return string(stored), true
		} else if err != nil {
			logger.Debugf("[Engram] getTELADual encrypted storage failed for %s: %s\n", key, err)
		}
	}

	// Fallback to unencrypted storage
	stored, err := GetValue("TELASettingsUnencrypted", []byte(key))
	if err == nil && stored != nil {
		logger.Printf("[Engram] getTELADual: Successfully loaded %s from fallback storage", key)
		return string(stored), true
	} else if err != nil {
		logger.Debugf("[Engram] getTELADual fallback storage failed for %s: %s\n", key, err)
	}

	logger.Printf("[Engram] getTELADual: No stored value found for %s", key)
	return "", false
}

// Initialize TELA preferences from dual storage (works with or without wallet)
func initTELAPreferences() {
	logger.Printf("[Engram] initTELAPreferences() called - wallet available: %v", engram.Disk != nil)

	// Load and apply TELA Port Start using dual storage
	if portStart, found := getTELADual("Port Start"); found {
		if p, err := strconv.Atoi(portStart); err == nil {
			if err := tela.SetPortStart(p); err != nil {
				logger.Errorf("[Engram] Failed to set TELA port start: %s\n", err)
			} else {
				logger.Printf("[Engram] TELA Port Start applied: %d", p)
			}
		}
	} else {
		logger.Printf("[Engram] TELA Port Start not found, using default")
	}

	// Load and apply Allow Updates setting using dual storage
	if allowUpdates, found := getTELADual("Allow Updates"); found {
		tela.AllowUpdates(allowUpdates == "Allow")
		logger.Printf("[Engram] TELA Allow Updates applied: %s", allowUpdates)
	} else {
		logger.Printf("[Engram] TELA Allow Updates not found, using default")
	}

	// Load and apply Restrictive Mode setting using dual storage
	if restrictiveMode, found := getTELADual("Restrictive Mode"); found {
		// TODO: Apply restrictive mode to tela package when function is available
		logger.Printf("[Engram] TELA Restrictive Mode setting loaded: %s", restrictiveMode)
	} else {
		logger.Printf("[Engram] TELA Restrictive Mode not found, using default (Disabled)")
	}
}

// Initialize WebSocket state from dual storage (works with or without wallet)
func initWebSocketState() {
	logger.Printf("[Engram] initWebSocketState() called - wallet available: %v", engram.Disk != nil)

	// Load stored WebSocket port using dual storage (works without wallet)
	if wsPort := getCyberdeckDual("WS"); wsPort != "" {
		cyberdeck.WS.port = wsPort
		if cyberdeck.WS.portText != nil {
			cyberdeck.WS.portText.SetText(wsPort)
		}
		logger.Printf("[Engram] WebSocket port loaded from dual storage: %s", wsPort)
	} else {
		logger.Printf("[Engram] No WebSocket port found in storage")
	}

	// Load global permissions (includes WebSocket enabled state)
	getPermissions()

	// Load RPC credentials from regular settings (works without wallet)
	getRPCCredentials()

	// Update UI based on loaded enabled state
	if cyberdeck.WS.toggle != nil && !session.Offline {
		logger.Printf("[Engram] Updating WebSocket UI based on loaded state: enabled=%v", cyberdeck.WS.global.enabled)
		if cyberdeck.WS.global.enabled {
			// If it should be enabled but server is nil, show "Turn On" (ready to start)
			cyberdeck.WS.toggle.Text = "Turn On"
			logger.Printf("[Engram] Set toggle text to 'Turn On' for enabled=true")
		} else {
			// Show disabled state
			cyberdeck.WS.status.Text = "Blocked"
			cyberdeck.WS.status.Color = colors.Gray
			cyberdeck.WS.toggle.Text = "Turn On"
			logger.Printf("[Engram] Set toggle text to 'Turn On' for enabled=false")
		}
	}

	// If WebSocket was previously enabled, restart it
	if cyberdeck.WS.global.enabled && !session.Offline {
		logger.Printf("[Engram] Attempting to restart WebSocket (was previously enabled)")

		// Validate port without requiring UI (for auto-start after login)
		wsEndpoint := cyberdeck.WS.port
		if wsEndpoint == "" {
			logger.Printf("[Engram] No WebSocket port configured, cannot auto-start")
		} else {
			logger.Printf("[Engram] Calling toggleXSWD with: %s", wsEndpoint)
			toggleXSWD(wsEndpoint)

			// Update UI to reflect server started (if UI is available)
			if cyberdeck.WS.server != nil {
				logger.Printf("[Engram] WebSocket server started successfully")
				if cyberdeck.WS.status != nil {
					cyberdeck.WS.status.Text = "Allowed"
					cyberdeck.WS.status.Color = colors.Green
				}
				if cyberdeck.WS.toggle != nil {
					cyberdeck.WS.toggle.Text = "Turn Off"
					logger.Printf("[Engram] Set toggle text to 'Turn Off' - server started")
				}
				if cyberdeck.WS.portText != nil {
					cyberdeck.WS.portText.Disable()
				}
			} else {
				logger.Printf("[Engram] toggleXSWD failed to start server")
			}
		}
	} else {
		logger.Printf("[Engram] Not restarting WebSocket - enabled=%v or offline=%v", cyberdeck.WS.global.enabled, session.Offline)
	}
}

// Go routine to update the latest information from the connected daemon (Online Mode only)
func StartPulse() {
	if !walletapi.Connected && engram.Disk != nil {
		logger.Printf("[Network] Attempting network connection to: %s\n", walletapi.Daemon_Endpoint)
		err := walletapi.Connect(session.Daemon)
		if err != nil {
			logger.Errorf("[Network] Failed to connect to: %s\n", walletapi.Daemon_Endpoint)
			walletapi.Connected = false
			closeWallet()
			session.Window.SetContent(layoutAlert(1))
			removeOverlays()
			return
		} else {
			sentNotifications := false
			walletapi.Connected = true
			engram.Disk.SetOnlineMode()
			status.Connection.FillColor = colors.Gray
			status.Sync.FillColor = colors.Gray

			// Refresh permissions after wallet connects to ensure all methods are loaded
			go func() {
				time.Sleep(time.Second * 2) // Allow UI to stabilize
				fyne.Do(func() {
					_, _ = getPermissions()
				})
			}()

			go func() {
				count := 0
				for engram.Disk != nil {
					if walletapi.Get_Daemon_Height() < 1 || !walletapi.Connected {
						logger.Printf("[Network] Attempting network connection to: %s\n", walletapi.Daemon_Endpoint)
						err := walletapi.Connect(session.Daemon)
						if err != nil {
							// If we fail DEFAULT_DAEMON_RECONNECT_TIMEOUT+ times, display node communication layout err
							if count >= DEFAULT_DAEMON_RECONNECT_TIMEOUT {
								walletapi.Connected = false
								closeWallet()
								session.Window.SetContent(layoutAlert(1))
								removeOverlays()
								break
							}
							count++
							logger.Errorf("[Network] Failed to connect to: %s (%d / %d)\n", walletapi.Daemon_Endpoint, count, DEFAULT_DAEMON_RECONNECT_TIMEOUT)
							walletapi.Connected = false
							status.Connection.FillColor = colors.Red
							status.Sync.FillColor = colors.Red
							status.Gnomon.FillColor = colors.Red
							status.EPOCH.FillColor = colors.Red

							time.Sleep(time.Second)
							continue
						} else {
							count = 0
							time.Sleep(time.Second)
							session.Offline = false
						}
					}

					if !engram.Disk.IsRegistered() {
						if !walletapi.Connected {
							logger.Errorf("[Network] Could not connect to daemon...%d\n", engram.Disk.Get_Daemon_TopoHeight())
							status.Connection.FillColor = colors.Red
							status.Connection.Refresh()
							status.Sync.FillColor = colors.Red
						}

						time.Sleep(time.Second)
					} else {
						if session.WalletHeight != engram.Disk.Get_Height() {
							sentNotifications = false
						}

						session.Balance, _ = engram.Disk.Get_Balance()
						session.WalletHeight = engram.Disk.Get_Height()
						session.DaemonHeight = engram.Disk.Get_Daemon_Height()

						session.LastBalance = session.Balance

						if walletapi.IsDaemonOnline() {
							status.Connection.FillColor = colors.Green
							if session.DaemonHeight > 0 && session.DaemonHeight-session.WalletHeight < 2 {
								status.Connection.FillColor = colors.Green
								status.Sync.FillColor = colors.Green
							} else if session.DaemonHeight == 0 {
								status.Sync.FillColor = colors.Red
							} else {
								status.Sync.FillColor = color.Transparent
							}

							if gnomon.Index != nil {
								if gnomon.Index.Status == "indexed" {
									status.Gnomon.FillColor = colors.Green
								} else {
									if uint64(gnomon.Index.LastIndexedHeight) < session.WalletHeight-15 {
										status.Gnomon.FillColor = colors.Red
									} else {
										status.Gnomon.FillColor = color.Transparent
									}
								}
							} else {
								status.Gnomon.FillColor = colors.Gray

								if gnomon.Index == nil && engram.Disk != nil {
									enableGnomon, _ := getGnomon()
									if enableGnomon == "1" {
										startGnomon()
									}
								}
							}

							if epoch.IsActive() {
								if epoch.IsProcessing() {
									status.EPOCH.FillColor = color.Transparent
								} else {
									status.EPOCH.FillColor = colors.Green
								}
							} else {
								if cyberdeck.EPOCH.err != nil {
									status.EPOCH.FillColor = colors.Red
								} else {
									status.EPOCH.FillColor = colors.Gray
								}
							}
						} else {
							status.Connection.FillColor = colors.Gray
							status.Sync.FillColor = colors.Gray
							status.Cyberdeck.FillColor = colors.Gray
							status.Gnomon.FillColor = colors.Gray
							status.EPOCH.FillColor = colors.Gray
							logger.Printf("[Network] Offline › Last Height: %d / %d\n", session.WalletHeight, session.DaemonHeight)
						}

						// Check for updates and send appropriate notifications
						var zeroscid crypto.Hash

						// Query incoming messages (with nil check to prevent panic on wallet close)
						if engram.Disk == nil {
							break
						}
						entries := engram.Disk.Show_Transfers(zeroscid, false, true, false, session.WalletHeight-1, session.WalletHeight-1, "", "", uint64(1337), 0)

						for e := range entries {
							if entries[e].Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) && !sentNotifications {
								sender := entries[e].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string)

								notification := fyne.NewNotification(sender, "New message was received (Height: "+fmt.Sprintf("%d", entries[e].Height)+")")
								fyne.CurrentApp().SendNotification(notification)

								sentNotifications = true
							}
						}

						fyne.Do(func() {
							if session.BalanceText != nil && !session.BalanceHidden {
								session.BalanceText.Text = globals.FormatMoney(session.Balance)
								session.BalanceText.Refresh()
							}
							if session.StatusText != nil {
								session.StatusText.Text = fmt.Sprintf("%d", session.WalletHeight)
								session.StatusText.Refresh()
							}
							status.Connection.Refresh()
							status.Sync.Refresh()
							status.Cyberdeck.Refresh()
							status.Gnomon.Refresh()
							status.EPOCH.Refresh()
						})

						time.Sleep(time.Second)
					}
				}

				if walletapi.Connected {
					walletapi.Connected = false
				}
			}()
		}
	}
}

// Get Network setting from the local Graviton tree (Ex: Mainnet, Testnet, Simulator)
func getNetwork() (network string) {
	result, err := GetValue("settings", []byte("network"))
	if err != nil {
		network = NETWORK_MAINNET
		session.Network = network
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
		setNetwork(network)
		return
	} else {
		if string(result) == NETWORK_TESTNET {
			network = NETWORK_TESTNET
			session.Network = network
			globals.Arguments["--testnet"] = true
			globals.Arguments["--simulator"] = false
			return
		} else if string(result) == NETWORK_SIMULATOR {
			network = NETWORK_SIMULATOR
			session.Network = network
			globals.Arguments["--testnet"] = true
			globals.Arguments["--simulator"] = true
			return
		} else {
			network = NETWORK_MAINNET
			session.Network = network
			globals.Arguments["--testnet"] = false
			globals.Arguments["--simulator"] = false
			return
		}
	}
}

// Set Network setting to the local Graviton tree (Ex: Mainnet, Testnet, Simulator)
func setNetwork(network string) (err error) {
	s := ""
	if network == NETWORK_MAINNET {
		s = network
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
	} else if network == NETWORK_SIMULATOR {
		s = network
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
	} else {
		s = NETWORK_TESTNET
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = false
	}

	session.Network = s

	StoreValue("settings", []byte("network"), []byte(s))

	return
}

// Get daemon endpoint setting from the local Graviton tree
func getDaemon() (r string) {
	result, err := GetValue("settings", []byte("endpoint"))
	if err != nil {
		if checkLocalNode() {
			r = "127.0.0.1:10102"
		} else {
			r = DEFAULT_REMOTE_DAEMON
		}
		setDaemon(r)
		session.Daemon = r
		globals.Arguments["--daemon-address"] = r
		return
	}

	r = string(result)
	session.Daemon = r
	globals.Arguments["--daemon-address"] = r
	return
}

func checkLocalNode() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:10102", 500*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func testNodeConnection(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func testNodeConnectionTimeout(address string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// Set the daemon endpoint setting to local Graviton tree
func setDaemon(s string) (err error) {
	StoreValue("settings", []byte("endpoint"), []byte(s))
	globals.Arguments["--daemon-address"] = s
	session.Daemon = s
	return
}

// Get Cyberdeck endpoint setting from the local Graviton tree
func getCyberdeck(key string) (r string) {
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

	stored, err := GetEncryptedValue("Cyberdeck", []byte(key))
	if err != nil {
		logger.Debugf("[Engram] getCyberdeck %s: %s\n", key, err)
		return
	}

	return string(stored)
}

// Get Cyberdeck endpoint setting with dual storage (try encrypted first, then fallback)
func getCyberdeckDual(key string) (r string) {
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
		stored, err := GetEncryptedValue("Cyberdeck", []byte(key))
		if err == nil && stored != nil {
			logger.Printf("[Engram] getCyberdeckDual: Successfully loaded %s from encrypted storage", key)
			return string(stored)
		} else if err != nil {
			logger.Debugf("[Engram] getCyberdeckDual encrypted storage failed: %s\n", err)
		}
	}

	// Fallback to unencrypted storage
	stored, err := GetValue("CyberdeckUnencrypted", []byte(key))
	if err == nil && stored != nil {
		logger.Printf("[Engram] getCyberdeckDual: Successfully loaded %s from fallback storage", key)
		return string(stored)
	} else if err != nil {
		logger.Debugf("[Engram] getCyberdeckDual fallback storage failed: %s\n", err)
	}

	logger.Printf("[Engram] getCyberdeckDual: No stored value found for %s", key)
	return ""
}

// Set Cyberdeck endpoint setting to the local Graviton tree
func setCyberdeck(port, key string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		logger.Debugf("[Engram] setCyberdeck: invalid key\n")
		return
	}

	err := StoreEncryptedValue("Cyberdeck", []byte(key), []byte(port))
	if err != nil {
		logger.Debugf("[Engram] setCyberdeck %s: %s\n", key, err)
	}
}

// Set Cyberdeck endpoint setting with dual storage (encrypted + unencrypted fallback)
func setCyberdeckDual(port, key string) {
	switch key {
	case "RPC":
		key = "port.RPC"
	case "WS":
		key = "port.WS"
	case "EPOCH":
		key = "port.EPOCH"
	default:
		logger.Debugf("[Engram] setCyberdeckDual: invalid key\n")
		return
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err := StoreEncryptedValue("Cyberdeck", []byte(key), []byte(port))
		if err != nil {
			logger.Debugf("[Engram] setCyberdeckDual encrypted storage failed: %s\n", err)
		} else {
			logger.Printf("[Engram] setCyberdeckDual: Successfully saved %s to encrypted storage", key)
		}
	}

	// Always save to unencrypted storage as fallback
	err := StoreValue("CyberdeckUnencrypted", []byte(key), []byte(port))
	if err != nil {
		logger.Debugf("[Engram] setCyberdeckDual unencrypted storage failed: %s\n", err)
	} else {
		logger.Printf("[Engram] setCyberdeckDual: Successfully saved %s to fallback storage", key)
	}
}

// Get mode (online, offline) setting from local Graviton tree
func getMode() {

	/*
		if globals.Arguments["--offline"].(bool) == true {
			session.Mode = "Offline"
			return
		}

		s := "mode"
		t := "settings"
		key := []byte(s)
		result, err := GetValue(t, key)
		if err != nil {
			session.Mode = "Online"
			err := setMode("Online")
			globals.Arguments["--offline"] = false
			if err != nil {
				fmt.Printf("[Engram] Error: %s\n", err)
				return
			}
		} else {
			if result == nil {
				session.Mode = "Online"
				err := setMode("Online")
				globals.Arguments["--offline"] = false
				if err != nil {
					fmt.Printf("[Engram] Error: %s\n", err)
					return
				}
			} else {
				if string(result) == "Offline" {
					globals.Arguments["--offline"] = true
					session.Mode = "Offline"
				} else {
					globals.Arguments["--offline"] = false
					session.Mode = "Online"
				}
			}
		}
	*/
}

// Set the default Offline Mode settings to the local Graviton tree
/*
func setMode(s string) (err error) {
	err = StoreValue("settings", []byte("mode"), []byte(s))
	if s == "Offline" {
		globals.Arguments["--offline"] = true
	} else {
		globals.Arguments["--offline"] = false
	}
	return
}
*/

// Get the default Gnomon settings from local Graviton tree
func getGnomon() (r string, err error) {
	v, err := GetValue("settings", []byte("gnomon"))
	if err != nil {
		gnomon.Active = 1
		if gnomon.Index != nil {
			gnomon.Index.Endpoint = getDaemon()
		}
		StoreValue("settings", []byte("gnomon"), []byte("1"))
	}

	if string(v) == "1" {
		gnomon.Active = 1
		if gnomon.Index != nil {
			gnomon.Index.Endpoint = getDaemon()
		}
	} else {
		gnomon.Active = 0
	}

	r = string(v)
	return
}

// Set the default Gnomon settings to the local Graviton tree
func setGnomon(s string) (err error) {
	if s == "1" {
		err = StoreValue("settings", []byte("gnomon"), []byte("1"))
		gnomon.Active = 1
		if gnomon.Index != nil {
			gnomon.Index.Endpoint = getDaemon()
		}
	} else {
		err = StoreValue("settings", []byte("gnomon"), []byte("0"))
		gnomon.Active = 0
	}
	return
}

// Get the RPC credentials from local Graviton tree
func getRPCCredentials() {
	if user, err := GetValue("settings", []byte("rpc_user")); err == nil && len(user) > 0 {
		cyberdeck.RPC.user = string(user)
		// Update UI field when settings are loaded at startup
		if cyberdeck.RPC.userText != nil {
			cyberdeck.RPC.userText.SetText(string(user))
		}
	}
	if pass, err := GetValue("settings", []byte("rpc_pass")); err == nil && len(pass) > 0 {
		cyberdeck.RPC.pass = string(pass)
		// Update UI field when settings are loaded at startup
		if cyberdeck.RPC.passText != nil {
			cyberdeck.RPC.passText.SetText(string(pass))
		}
	}
}

/*
func getAuthMode() (result string, err error) {
	r, err := GetValue("settings", []byte("auth_mode"))
	if err != nil {
		StoreValue("settings", []byte("auth_mode"), []byte("true"))
		cyberdeck.mode = 1
		result = "true"
	} else {
		result = string(r)
		if string(result) == "true" {
			cyberdeck.mode = 1
			result = "true"
		} else {
			cyberdeck.mode = 0
			result = "false"
		}
	}
	return
}
*/

// Get the auth_mode settings from local Graviton tree
func setAuthMode(s string) {
	if s == "true" {
		StoreValue("settings", []byte("auth_mode"), []byte("true"))
	} else {
		StoreValue("settings", []byte("auth_mode"), []byte("false"))
	}
}

// Check if a URL exists in the string
func getTextURL(s string) (result []string) {
	return xurls.Relaxed().FindAllString(s, -1)
}

// Set the window size from provided height and width
func resizeWindow(width float32, height float32) {
	s := fyne.NewSize(width, height)
	session.Window.Resize(s)
}

// Close the active wallet
func closeWallet() {
	showLoadingOverlay()

	if engram.Disk != nil {
		logger.Printf("[Engram] Shutting down wallet services...\n")

		// CRITICAL FIX: Stop Gnomon FIRST to release database lock before closing wallet
		// This prevents "database timeout" errors when reopening the wallet
		if gnomon.Index != nil {
			logger.Printf("[Gnomon] Shutting down indexers...\n")
			stopGnomon()
			// Increased delay to ensure database file is fully released
			time.Sleep(500 * time.Millisecond)
		}

		stopEPOCH()
		engram.Disk.SetOfflineMode()
		engram.Disk.Save_Wallet()

		globals.Exit_In_Progress = true
		engram.Disk.Close_Encrypted_Wallet()
		session.WalletOpen = false
		session.Domain = "app.main"
		session.BalanceUSD = ""
		session.LastBalance = 0
		engram.Disk = nil
		tx = Transfers{}

		if cyberdeck.RPC.server != nil {
			cyberdeck.RPC.server.RPCServer_Stop()
			cyberdeck.RPC.server = nil
			logger.Printf("[Engram] Cyberdeck RPC closed.\n")
		}

		if cyberdeck.WS.server != nil {
			cyberdeck.WS.server.Stop()
			cyberdeck.WS.server = nil
			cyberdeck.WS.apps = nil
			cyberdeck.WS.list = nil
			logger.Printf("[Engram] Cyberdeck XSWD closed.\n")
		}
		// CRITICAL FIX: Don't reset enabled state here - it should persist across wallet sessions
		// The enabled state is saved to encrypted storage and should be restored on wallet open
		cyberdeck.WS.advanced = false
		cyberdeck.WS.global.connect = false

		tela.ShutdownTELA()

		if rpc_client.WS != nil {
			rpc_client.WS.Close()
			rpc_client.WS = nil
			logger.Printf("[Engram] Websocket client closed.\n")
		}

		if rpc_client.RPC != nil {
			rpc_client.RPC.Close()
			rpc_client.RPC = nil
			logger.Printf("[Engram] RPC client closed.\n")
		}

		session.Path = ""
		session.Name = ""

		session.LastDomain = layoutMain()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		removeOverlays()
		//session.Window.CenterOnScreen()
		logger.Printf("[Engram] Wallet saved and closed successfully.\n")
		return
	}
}

// Create a new account and wallet file
func create() (address string, seed string, err error) {
	check := findAccount()

	if session.Path == "" {
		session.Error = "Please enter an account name."
	} else if session.Language == -1 {
		session.Error = "Please select a language."
	} else if session.Password == "" {
		session.Error = "Please enter a password."
	} else if session.PasswordConfirm == "" {
		session.Error = "Please confirm your password."
	} else if session.PasswordConfirm != session.Password {
		session.Error = "Passwords do not match."
	} else if check {
		session.Error = "Account name already exists."
	} else {
		engram.Disk, err = walletapi.Create_Encrypted_Wallet_Random(session.Path, session.Password)

		if err != nil {
			session.Language = -1
			session.Name = ""
			session.Path = ""
			session.Password = ""
			session.PasswordConfirm = ""
			session.Error = "Account could not be created."
		} else {
			switch session.Network {
			case NETWORK_TESTNET:
				engram.Disk.SetNetwork(false)
				globals.Arguments["--testnet"] = true
				globals.Arguments["--simulator"] = false
			case NETWORK_SIMULATOR:
				engram.Disk.SetNetwork(false)
				globals.Arguments["--testnet"] = true
				globals.Arguments["--simulator"] = true
			default:
				engram.Disk.SetNetwork(true)
				globals.Arguments["--testnet"] = false
				globals.Arguments["--simulator"] = false
			}

			languages := mnemonics.Language_List()

			if session.Language < 0 || session.Language > len(languages)-1 {
				session.Language = 0 // English
			}

			engram.Disk.SetSeedLanguage(languages[session.Language])
			address = engram.Disk.GetAddress().String()
			seed = engram.Disk.GetSeed()
			engram.Disk.Close_Encrypted_Wallet()
			engram.Disk = nil
			session.Error = "Account successfully created."
			session.Language = -1
			session.Name = ""
			session.Path = ""
			session.Password = ""
			session.PasswordConfirm = ""
			session.Domain = "app.main"
		}
	}
	return
}

// The main login routine - optimized for fast dashboard display
func login() {
	showLoadingOverlay()

	if engram.Disk == nil {
		temp, err := walletapi.Open_Encrypted_Wallet(session.Path, session.Password)
		if err != nil {
			session.Domain = "app.main"
			session.Error = err.Error()
			if len(session.Error) > 40 {
				session.Error = fmt.Sprintf("%s...", session.Error[0:40])
			}
			session.Window.Canvas().Content().Refresh()
			removeOverlays()
			return
		}

		engram.Disk = temp
		session.Password = ""

		logger.Printf("[Engram] Wallet opened - loading encrypted settings...")
		initSettings()
		logger.Printf("[Engram] Encrypted settings loaded successfully")
	}

	switch session.Network {
	case NETWORK_TESTNET:
		engram.Disk.SetNetwork(false)
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = false
	case NETWORK_SIMULATOR:
		engram.Disk.SetNetwork(false)
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
	default:
		engram.Disk.SetNetwork(true)
		globals.Arguments["--testnet"] = false
		globals.Arguments["--simulator"] = false
	}

	session.WalletOpen = true
	session.BalanceUSD = ""
	session.LastBalance = 0

	if !session.Offline {
		walletapi.SetDaemonAddress(session.Daemon)
		engram.Disk.SetDaemonAddress(session.Daemon)

		if session.TrackRecentBlocks > 0 {
			logger.Printf("[Engram] Scan tracking enabled, only scanning the last %d blocks...\n", session.TrackRecentBlocks)
			engram.Disk.SetTrackRecentBlocks(session.TrackRecentBlocks)
		}

		if s, err := strconv.Atoi(getCyberdeck("EPOCH")); err == nil {
			if err := epoch.SetPort(s); err != nil {
				logger.Errorf("[Engram] Setting EPOCH port: %s\n", err)
			}
		}

		cyberdeck.EPOCH.total.Hashes = 0
		cyberdeck.EPOCH.total.MiniBlocks = 0
		if epochData, err := GetEncryptedValue("Cyberdeck", []byte("EPOCH")); err == nil {
			if err := json.Unmarshal(epochData, &cyberdeck.EPOCH.total); err != nil {
				logger.Errorf("[Engram] Setting EPOCH total: %s\n", err)
			}
		}

		go StartPulse()
	} else {
		engram.Disk.SetOfflineMode()
		status.Connection.FillColor = colors.Gray
		status.Sync.FillColor = colors.Gray
	}

	setRingSize(engram.Disk, 16)
	session.Verified = false

	if a.Driver().Device().IsMobile() {
		session.Domain = "app.wallet"
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
	}

	session.Window.SetContent(layoutDashboard())
	removeOverlays()

	if !session.Offline {
		status.Connection.FillColor = colors.Yellow
		status.Connection.Refresh()
		status.Sync.FillColor = colors.Yellow
		status.Sync.Refresh()
		session.Balance = 0

		go func() {
			connected := waitForConnectionWithTimeout(5 * time.Second)
			if !connected {
				fyne.Do(func() {
					closeWallet()
					session.Window.SetContent(layoutAlert(1))
				})
				return
			}

			fyne.Do(func() {
				status.Connection.FillColor = colors.Green
				status.Connection.Refresh()
			})

			waitForWalletSync(3 * time.Second)

			needsReg, regDone := checkRegistrationWithTimeout(10 * time.Second)
			if needsReg {
				fyne.Do(func() {
					registerAccount()
					session.Verified = true
				})
				logger.Printf("[Registration] Account registration PoW started...\n")
				logger.Printf("[Registration] Registering your account. This can take up to 120 minutes (one time). Please wait...\n")
				return
			}

			if regDone {
				go startGnomon()
			}

			fyne.Do(func() {
				updateDashboardAfterLogin()
			})
		}()
	} else {
		fyne.Do(func() {
			updateDashboardAfterLogin()
		})
	}

	address := engram.Disk.GetAddress().String()
	shard := fmt.Sprintf("%x", sha1.Sum([]byte(address)))
	session.ID = shard
	session.LimitMessages = true
}

// waitForConnectionWithTimeout waits for daemon connection with a timeout
func waitForConnectionWithTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if walletapi.Connected {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return walletapi.Connected
}

// waitForWalletSync waits briefly for wallet height to catch up
func waitForWalletSync(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	daemonHeight := engram.Disk.Get_Daemon_Height()
	for time.Now().Before(deadline) {
		if engram.Disk.Get_Height() >= daemonHeight {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// checkRegistrationWithTimeout checks if account registration is needed
func checkRegistrationWithTimeout(timeout time.Duration) (needsRegistration bool, canProceed bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		height := engram.Disk.Get_Registration_TopoHeight()
		if height >= 1 {
			return false, true
		}
		time.Sleep(500 * time.Millisecond)
	}

	height := engram.Disk.Get_Registration_TopoHeight()
	if height < 1 {
		return true, false
	}
	return false, true
}

// updateDashboardAfterLogin updates dashboard UI elements after background operations complete
func updateDashboardAfterLogin() {
	session.Balance, _ = engram.Disk.Get_Balance()
	fyne.Do(func() {
		if session.BalanceText != nil {
			if session.BalanceHidden {
				session.BalanceText.Text = "••••••"
			} else {
				session.BalanceText.Text = globals.FormatMoney(session.Balance)
			}
			session.BalanceText.Refresh()
		}
	})

	session.WalletHeight = engram.Disk.Wallet_Memory.Get_Height()
	session.DaemonHeight = engram.Disk.Get_Daemon_Height()
	fyne.Do(func() {
		if session.StatusText != nil {
			session.StatusText.Text = fmt.Sprintf("%d", session.WalletHeight)
			session.StatusText.Refresh()
		}

		if session.WalletHeight == session.DaemonHeight && !session.Offline {
			status.Sync.FillColor = colors.Green
			status.Sync.Refresh()
		}
	})
}

// Remove all overlays
func removeOverlays() {
	overlays := session.Window.Canvas().Overlays()
	list := overlays.List()

	for o := range list {
		overlays.Remove(list[o])
	}

	if res.loading != nil {
		res.loading.Hide()
		res.loading.Stop()
		res.loading = nil
	}
}

// Add an overlay with the loading animation
func showLoadingOverlay() {
	frame := &iframe{}

	if res.loading == nil {
		res.loading, _ = x.NewAnimatedGifFromResource(resourceLoadingGif)
		res.loading.SetMinSize(fyne.NewSize(ui.Width*0.45, ui.Width*0.45))
	}

	rect := canvas.NewRectangle(colors.DarkMatter)
	rect.SetMinSize(frame.Size())

	background := container.NewStack(
		rect,
		container.NewCenter(
			res.loading,
		),
	)

	res.loading.Start()

	layout := container.NewStack(
		frame,
		background,
	)

	overlays := session.Window.Canvas().Overlays()
	overlays.Add(layout)
}

// Load embedded resources
func loadResources() {
	res.bg = canvas.NewImageFromResource(resourceBgPng)
	res.bg.FillMode = canvas.ImageFillContain

	res.bg2 = canvas.NewImageFromResource(resourceBg2Png)
	res.bg2.FillMode = canvas.ImageFillContain

	res.icon = canvas.NewImageFromResource(resourceIconPng)
	res.icon.FillMode = canvas.ImageFillContain

	res.header = canvas.NewImageFromResource(resourceBackground1Png)
	res.header.FillMode = canvas.ImageFillContain

	res.load = canvas.NewImageFromResource(resourceLoadPng)
	res.load.FillMode = canvas.ImageFillStretch

	res.dero = canvas.NewImageFromResource(resourceDeroPng)
	res.dero.FillMode = canvas.ImageFillContain

	res.gram = canvas.NewImageFromResource(resourceDEROLogoPng)
	res.gram.FillMode = canvas.ImageFillContain

	res.block = canvas.NewImageFromResource(resourceBlankPng)
	res.block.FillMode = canvas.ImageFillContain

	res.red_alert = canvas.NewImageFromResource(resourceRedAlertPng)
	res.red_alert.FillMode = canvas.ImageFillContain

	res.green_alert = canvas.NewImageFromResource(resourceGreenAlertPng)
	res.green_alert.FillMode = canvas.ImageFillContain

	res.mainBg = canvas.NewImageFromResource(resourceEngramMainPng)
	res.mainBg.FillMode = canvas.ImageFillContain

	res.telaBg = canvas.NewImageFromResource(resourceTelaPng)
	res.telaBg.FillMode = canvas.ImageFillContain

}

// Validate if the provided word is a seed word
func checkSeedWord(w string) (check bool) {
	split := strings.Split(w, " ")

	if len(split) > 1 {
		return
	}
	_, _, _, check = mnemonics.Find_indices([]string{w})

	return
}

// Add a DERO transfer to the batch
func addTransfer() error {
	var arguments = rpc.Arguments{}
	var err error

	logger.Printf("[Send] Starting tx...\n")
	if tx.Address.IsIntegratedAddress() {
		if tx.Address.Arguments.Validate_Arguments() != nil {
			logger.Errorf("[Service] Integrated Address arguments could not be validated\n")
			err = errors.New("integrated address arguments could not be validated")
			return err
		}

		logger.Printf("[Send] Not Integrated..\n")
		if !tx.Address.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
			logger.Errorf("[Service] Integrated Address does not contain destination port\n")
			err = errors.New("integrated address does not contain destination port")
			return err
		}

		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: tx.Address.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)})
		logger.Printf("[Send] Added arguments..\n")

		if tx.Address.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) {

			if tx.Address.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime).(time.Time).Before(time.Now().UTC()) {
				logger.Errorf("[Service] This address has expired: %s\n", tx.Address.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
				err = errors.New("this address has expired")
				return err
			} else {
				logger.Warnf("[Service] This address will expire: %s\n", tx.Address.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
			}
		}

		logger.Printf("[Service] Destination port is integrated in address: %d\n", tx.Address.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64))

		if tx.Address.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) {
			logger.Printf("[Service] Integrated Message: %s\n", tx.Address.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString))
			arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: tx.Address.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString)})
		}
	}

	logger.Printf("[Send] Checking arguments..\n")

	for _, arg := range tx.Address.Arguments {
		if !(arg.Name == rpc.RPC_COMMENT || arg.Name == rpc.RPC_EXPIRY || arg.Name == rpc.RPC_DESTINATION_PORT || arg.Name == rpc.RPC_SOURCE_PORT || arg.Name == rpc.RPC_VALUE_TRANSFER || arg.Name == rpc.RPC_NEEDS_REPLYBACK_ADDRESS) {
			switch arg.DataType {
			case rpc.DataString:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataInt64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataUint64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataFloat64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataTime:
				logger.Warnf("[Service] Time currently not supported.\n")
			}
		}
	}

	logger.Printf("[Send] Checking Amount..\n")

	if tx.Address.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
		logger.Printf("[Service] Transaction amount: %s\n", globals.FormatMoney(tx.Address.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)))
		tx.Amount = tx.Address.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
	} else {
		balance, _ := engram.Disk.Get_Balance()
		logger.Printf("[Send] Balance: %d\n", balance)
		logger.Printf("[Send] Amount: %d\n", tx.Amount)

		if tx.Amount > balance {
			logger.Errorf("[Send] Error: Insufficient funds\n")
			err = errors.New("insufficient funds")
			return err
		} else if tx.Amount == balance {
			tx.SendAll = true
		} else {
			tx.SendAll = false
		}
	}

	logger.Printf("[Send] Checking services..\n")

	if tx.Address.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataUint64) {
		logger.Printf("[Service] Reply Address required, sending: %s\n", engram.Disk.GetAddress().String())
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_REPLYBACK_ADDRESS, DataType: rpc.DataAddress, Value: engram.Disk.GetAddress()})
	}

	logger.Printf("[Send] Checking payment ID/destination port..\n")

	if len(arguments) == 0 {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: tx.PaymentID})
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: tx.Comment})
	}

	logger.Printf("[Send] Checking Pack..\n")

	if _, err := arguments.CheckPack(transaction.PAYLOAD0_LIMIT); err != nil {
		logger.Errorf("[Send] Arguments packing err: %s\n", err)
		return err
	}

	if tx.Ringsize == 0 {
		tx.Ringsize = 2
	} else if tx.Ringsize > 128 {
		tx.Ringsize = 128
	} else if !crypto.IsPowerOf2(int(tx.Ringsize)) {
		tx.Ringsize = 2
		logger.Errorf("[Send] Error: Invalid ringsize - New ringsize = %d\n", tx.Ringsize)
		err = errors.New("invalid ringsize")
		return err
	}

	tx.Status = "Unsent"

	logger.Printf("[Send] Ringsize: %d\n", tx.Ringsize)

	tx.Pending = append(tx.Pending, rpc.Transfer{Amount: tx.Amount, Destination: tx.Address.String(), Payload_RPC: arguments})
	logger.Printf("[Send] Added transfer to the pending list.\n")

	return nil
}

// Send all batched transfers (TODO: export offline transactions to file in Offline mode)
func sendTransfers() (txid crypto.Hash, err error) {
	if session.Offline {
		return
	}

	fees := ((tx.Ringsize + 1) * config.FEE_PER_KB) / 4
	if fees < 85 {
		fees = 85
	}

	logger.Printf("[Send] Calculated Fees: %d\n", fees*uint64(len(tx.Pending)))

	tx.TX, err = engram.Disk.TransferPayload0(tx.Pending, tx.Ringsize, false, rpc.Arguments{}, fees, false)
	if err != nil {
		logger.Errorf("[Send] Error while building transaction: %s\n", err)
		return
	}

	if err = engram.Disk.SendTransaction(tx.TX); err != nil {
		logger.Errorf("[Send] Error while dispatching transaction: %s\n", err)
		return
	}

	tx.Fees = tx.TX.Fees()
	tx.TXID = tx.TX.GetHash()

	logger.Printf("[Send] Dispatched transaction: %s\n", tx.TXID)

	txid = tx.TX.GetHash()

	tx = Transfers{}

	return
}

// Go Routine for account registration
func registerAccount() {
	session.Domain = "app.register"
	if engram.Disk == nil {
		resizeWindow(ui.MaxWidth, ui.MaxHeight)
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutMain())
		session.Domain = "app.main"
		return
	}

	link := widget.NewHyperlinkWithStyle("Cancel", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	link.OnTapped = func() {
		session.Gif.Stop()
		session.Gif = nil
		closeWallet()
	}

	title := canvas.NewText("R E G I S T R A T I O N", colors.Green)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 16

	heading := canvas.NewText("Please wait...", colors.Gray)
	heading.TextSize = 22
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	sub := canvas.NewText("This one-time process can take a while.", colors.Gray)
	sub.TextSize = 14
	sub.Alignment = fyne.TextAlignCenter
	sub.TextStyle = fyne.TextStyle{Bold: true}

	resizeWindow(ui.MaxWidth, ui.MaxHeight)
	session.Window.SetContent(layoutTransition())
	session.Window.SetContent(layoutWaiting(title, heading, sub, link))

	// Registration PoW
	go func() {
		var reg_tx *transaction.Transaction
		successful_regs := make(chan *transaction.Transaction)
		counter := 0
		session.RegHashes = 0

		for i := 0; i < runtime.GOMAXPROCS(0)-1; i++ {
			go func() {
				for counter == 0 {
					if engram.Disk == nil {
						break
					} else if engram.Disk.IsRegistered() {
						break
					}

					lreg_tx := engram.Disk.GetRegistrationTX()
					hash := lreg_tx.GetHash()
					session.RegHashes++

					if hash[0] == 0 && hash[1] == 0 && hash[2] == 0 {
						successful_regs <- lreg_tx
						counter++
						break
					}
				}
			}()
		}

		if engram.Disk == nil {
			session.Gif.Stop()
			session.Gif = nil

			fyne.Do(func() {
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutMain())
			})

			session.Domain = "app.main"
			return
		}

		reg_tx = <-successful_regs

		logger.Printf("[Registration] Registration TXID: %s\n", reg_tx.GetHash())
		err := engram.Disk.SendTransaction(reg_tx)
		if err != nil {
			session.Gif.Stop()
			session.Gif = nil
			logger.Errorf("[Registration] Error: %s\n", err)

			fyne.Do(func() {
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutMain())
			})

			session.Domain = "app.main"
		} else {
			session.Gif.Stop()
			session.Gif = nil
			logger.Printf("[Registration] Registration transaction dispatched successfully.\n")
			session.Domain = "app.wallet"

			fyne.Do(func() {
				resizeWindow(ui.MaxWidth, ui.MaxHeight)
				session.Window.SetContent(layoutTransition())
				session.Window.SetContent(layoutDashboard())
			})
		}
	}()
}

// Set the ring size for transactions
func setRingSize(wallet *walletapi.Wallet_Disk, s int) bool {
	if wallet == nil {
		logger.Errorf("[Engram] No wallet found.\n")
		return false
	}

	// Minimum ring size is 2, only accept powers of 2.
	if s < 2 {
		wallet.SetRingSize(2)
		logger.Printf("[Engram] Set minimum ring size: 2\n")
	} else {
		wallet.SetRingSize(s)
		logger.Printf("[Engram] Set default ring size: %d\n", s)
	}

	return true
}

// Check if a username exists, return the registered address if so
func checkUsername(s string, h int64) (address string, err error) {
	if session.Offline {
		return
	}

	if h < 0 {
		address, err = engram.Disk.NameToAddress(s)
	} else {
		var params rpc.NameToAddress_Params
		var response *jrpc2.Response
		var result rpc.NameToAddress_Result

		rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
		if err != nil {
			return
		}

		input_output := rwc.New(rpc_client.WS)
		rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

		if rpc_client.RPC != nil {
			params.Name = s
			params.TopoHeight = h

			address = ""
			response, err = rpc_client.RPC.Call(context.Background(), "DERO.NameToAddress", params)

			rpc_client.WS.Close()
			rpc_client.RPC.Close()

			if err != nil {
				return
			}

			err = response.UnmarshalResult(&result)
			if err != nil {
				return
			}

			if result.Status != "OK" {
				err = errors.New("username does not exist")
				return
			}

			address = result.Address
		}
	}

	return
}

// Get the transaction fees to be paid
func getGasEstimate(gp rpc.GasEstimate_Params) (gas uint64, err error) {
	var result rpc.GasEstimate_Result

	rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
	if err != nil {
		return
	}

	input_output := rwc.New(rpc_client.WS)
	rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

	if err = rpc_client.RPC.CallResult(context.Background(), "DERO.GetGasEstimate", gp, &result); err != nil {
		return
	}

	if result.Status != "OK" {
		return
	}

	gas = result.GasStorage

	return
}

// Register a new DERO username
func registerUsername(s string) (storage uint64, err error) {
	// Check first if the name is taken
	valid, _ := checkUsername(s, -1)
	if valid != "" {
		logger.Errorf("[Username] Error: skipping registration - username exists.\n")
		err = errors.New("username already exists")
		return
	}

	scid := crypto.HashHexToHash("0000000000000000000000000000000000000000000000000000000000000001")

	var args = rpc.Arguments{}
	args = append(args, rpc.Argument{Name: "entrypoint", DataType: "S", Value: "Register"})
	args = append(args, rpc.Argument{Name: "SC_ID", DataType: "H", Value: scid})
	args = append(args, rpc.Argument{Name: "SC_ACTION", DataType: "U", Value: uint64(rpc.SC_CALL)})
	args = append(args, rpc.Argument{Name: "name", DataType: "S", Value: s})

	var p rpc.Transfer_Params
	var dest string

	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	default:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}
	p.Transfers = append(p.Transfers, rpc.Transfer{
		Destination: dest,
		Amount:      0,
		Burn:        0,
	})

	gp := rpc.GasEstimate_Params{SC_RPC: args, Ringsize: 2, Signer: engram.Disk.GetAddress().String(), Transfers: p.Transfers}

	storage, err = getGasEstimate(gp)
	if err != nil {
		logger.Errorf("[Username] Error estimating fees: %s\n", err)
		return
	}

	tx, err := engram.Disk.TransferPayload0(p.Transfers, 2, false, args, storage, false)
	if err != nil {
		logger.Errorf("[Username] Error while building transaction: %s\n", err)
		return
	}

	err = engram.Disk.SendTransaction(tx)
	if err != nil {
		logger.Errorf("[Username] Error while dispatching transaction: %s\n", err)
		return
	}

	logger.Printf("[Username] Username Registration TXID:  %s\n", tx.GetHash().String())

	return
}

// Check to make sure the message transaction meets criteria
func checkMessagePack(m string, s string, r string) (err error) {
	if m == "" {
		return
	}

	mapAddress := ""
	a, err := globals.ParseValidateAddress(r)
	if err != nil {
		//mapAddress, err = engram.Disk.NameToAddress(r)
		mapAddress, err = checkUsername(r, -1)
		if err != nil {
			return
		}
		a, err = globals.ParseValidateAddress(mapAddress)
		if err != nil {
			return
		}
	}

	if s == "" {
		s = engram.Disk.GetAddress().String()
	}

	var amount uint64

	if a.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) { // but only it is present
		//logger.Info("Transaction", "Value", globals.FormatMoney(a.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)))
		amount = a.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
	} else {
		amount, err = globals.ParseAmount("0.00001")
		if err != nil {
			//logger.Error(err, "Err parsing amount\n")
			return
		}
	}

	var arguments = rpc.Arguments{
		{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
		{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: amount},
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m},
		{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s},
	}

	if a.IsIntegratedAddress() { // read everything from the address
		if a.Arguments.Validate_Arguments() != nil {
			//fmt.Printf(err, "Integrated Address  arguments could not be validated.\n")
			return
		}

		if !a.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) { // but only it is present
			//fmt.Printf(fmt.Errorf("Integrated Address does not contain destination port.\n"), "")
			return
		}

		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)})

		if a.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) { // but only it is present
			if a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime).(time.Time).Before(time.Now().UTC()) {
				//fmt.Printf(nil, "This address has expired.", "expiry time", a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
				return
			}
		}

		logger.Printf("Destination port is integrated in address. %d\n", a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64))

		if a.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) { // but only it is present
			logger.Printf("Integrated Message: %s\n", a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString))
			arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString)})
		}
	}

	for _, arg := range arguments {
		if !(arg.Name == rpc.RPC_COMMENT || arg.Name == rpc.RPC_EXPIRY || arg.Name == rpc.RPC_DESTINATION_PORT || arg.Name == rpc.RPC_SOURCE_PORT || arg.Name == rpc.RPC_VALUE_TRANSFER || arg.Name == rpc.RPC_NEEDS_REPLYBACK_ADDRESS) {
			switch arg.DataType {
			case rpc.DataString:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataInt64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataUint64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataFloat64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataTime:
				logger.Warnf("[Service] Time currently not supported.\n")
			}
		}
	}

	if a.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s})
	}

	// if no arguments, use space by embedding a small comment
	if len(arguments) == 0 { // allow user to enter Comment
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)})
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m})
	}

	if _, err = arguments.CheckPack(transaction.PAYLOAD0_LIMIT); err != nil {
		logger.Errorf("[Message] Arguments packing err: %s\n", err)
		return
	}

	return
}

// Send a private message to another account
func sendMessage(m string, s string, r string) (txid crypto.Hash, err error) {
	if m == "" {
		return
	}

	mapAddress := ""
	a, err := globals.ParseValidateAddress(r)
	if err != nil {
		//mapAddress, err = engram.Disk.NameToAddress(r)
		mapAddress, err = checkUsername(r, -1)
		if err != nil {
			return
		}
		a, err = globals.ParseValidateAddress(mapAddress)
		if err != nil {
			return
		}
	}

	if s == "" {
		s = engram.Disk.GetAddress().String()
	}

	amount, err := globals.ParseAmount("0.00001")
	if err != nil {
		//logger.Error(err, "Err parsing amount\n")
		return
	}

	var arguments = rpc.Arguments{
		{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
		{Name: rpc.RPC_VALUE_TRANSFER, DataType: rpc.DataUint64, Value: amount},
		{Name: rpc.RPC_EXPIRY, DataType: rpc.DataTime, Value: time.Now().Add(time.Hour).UTC()},
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m},
		{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s},
	}

	if a.IsIntegratedAddress() {
		if a.Arguments.Validate_Arguments() != nil {
			return
		}

		if !a.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
			logger.Errorf("[Send Message] Integrated Address does not contain destination port.\n")
			return
		}

		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)})

		if a.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) {
			if a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime).(time.Time).Before(time.Now().UTC()) {
				logger.Errorf("[Send Message] This address has expired on %x\n", a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
				return
			} else {
				logger.Warnf("[Send Message] This address will expire on %x\n", a.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime))
			}
		}

		logger.Printf("[Send Message] Destination port is integrated in address. %x\n", a.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64))

		if a.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) {
			logger.Printf("[Send Message] Integrated Message: %s\n", a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString))
			arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: a.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString)})
		}
	}

	for _, arg := range arguments {
		if !(arg.Name == rpc.RPC_COMMENT || arg.Name == rpc.RPC_EXPIRY || arg.Name == rpc.RPC_DESTINATION_PORT || arg.Name == rpc.RPC_SOURCE_PORT || arg.Name == rpc.RPC_VALUE_TRANSFER || arg.Name == rpc.RPC_NEEDS_REPLYBACK_ADDRESS) {
			switch arg.DataType {
			case rpc.DataString:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataInt64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataUint64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataFloat64:
				arguments = append(arguments, rpc.Argument{Name: arg.Name, DataType: arg.DataType, Value: arg.Value.(string)})
			case rpc.DataTime:
				logger.Warnf("[Service] Time currently not supported.\n")
			}
		}
	}

	if a.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
		amount = a.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
	} else {
		amount, err = globals.ParseAmount("0.00001")
		if err != nil {
			logger.Errorf("[Send Message] Failed parsing transfer amount: %s\n", err)
			return
		}
	}

	if a.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: s})
	}

	if len(arguments) == 0 {
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)})
		arguments = append(arguments, rpc.Argument{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: m})
	}

	if _, err = arguments.CheckPack(transaction.PAYLOAD0_LIMIT); err != nil {
		logger.Errorf("[Message] Arguments packing err: %s\n", err)
		return
	}

	fees := ((uint64(engram.Disk.GetRingSize()) + 1) * config.FEE_PER_KB) / 4

	logger.Printf("[Message] Calculated Fees: %d\n", fees)

	transfer := rpc.Transfer{Amount: amount, Destination: a.String(), Payload_RPC: arguments}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{transfer}, 0, false, rpc.Arguments{}, fees, false)
	if err != nil {
		logger.Errorf("[Message] Error while building transaction: %s\n", err)
		return
	}

	if err = engram.Disk.SendTransaction(tx); err != nil {
		logger.Errorf("[Message] Error while dispatching transaction: %s\n", err)
		return
	}

	txid = tx.GetHash()

	logger.Printf("[Message] Dispatched transaction: %s\n", txid)

	return
}

// Get a list of message transactions from an address
func getMessagesFromUser(s string, h uint64) (result []rpc.Entry) {
	var zeroscid crypto.Hash
	if s == "" {
		return
	}

	messages := engram.Disk.Get_Payments_DestinationPort(zeroscid, uint64(1337), h)

	for m := range messages {
		var username bool
		var username2 bool

		txid := messages[m].TXID
		_, tx := engram.Disk.Get_Payments_TXID(zeroscid, txid)

		//check, err := engram.Disk.NameToAddress(s)
		check, err := checkUsername(s, -1)
		if err != nil {
			username = false
		} else {
			username = true
		}

		if tx.Incoming {
			if tx.Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
				height := int64(tx.Height)
				check2, err := checkUsername(tx.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string), height)
				if err != nil {
					username2 = false
					addr, err := globals.ParseValidateAddress(tx.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string))
					if err != nil {
						check2 = ""
					} else {
						check2 = addr.String()
					}
				} else {
					username2 = true
				}

				// Check for spoofing
				//if ring_member_exists(txid, check2) {

				if username && username2 {
					if check == check2 {
						result = append(result, messages[m])
					}
				} else if !username && !username2 {
					if s == tx.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string) {
						result = append(result, messages[m])
					}
				} else if check == tx.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string) {
					result = append(result, messages[m])
				} else if s == check2 {
					result = append(result, messages[m])
				}
				//}
			}
		} else {
			//addr, err := engram.Disk.NameToAddress(s)
			addr, err := checkUsername(s, -1)
			if err != nil {
				if tx.Destination == s {
					result = append(result, messages[m])
				}
			} else {
				if tx.Destination == addr {
					result = append(result, messages[m])
				}
			}
		}
	}

	return
}

// Get a list of all message transactions and sort them by address
func getMessages(h uint64) (result []string) {
	var zeroscid crypto.Hash
	messages := engram.Disk.Get_Payments_DestinationPort(zeroscid, uint64(1337), h)

	for m := range messages {
		if messages[m].Incoming {
			if messages[m].Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
				if messages[m].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string) == "" {

				} else {
					height := int64(messages[m].Height)
					sender, _ := checkUsername(messages[m].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string), height)
					if sender == "" {
						addr, err := globals.ParseValidateAddress(messages[m].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string))
						if err != nil {

						} else {
							sender = addr.String()
							for r := range result {
								if r > -1 && r < len(result) {
									if strings.Contains(result[r], sender+"~~~") {
										copy(result[r:], result[r+1:])
										result[len(result)-1] = ""
										result = result[:len(result)-1]
									}
								}
							}
							result = append(result, sender+"~~~")
						}
					} else {
						// Check for spoofing
						//if ring_member_exists(messages[m].TXID, sender) {
						for r := range result {
							if r > -1 && r < len(result) {
								//if strings.Contains(result[r], sender+"```"+messages[m].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string)) {
								if strings.Contains(result[r], sender+"~~~") {
									copy(result[r:], result[r+1:])
									result[len(result)-1] = ""
									result = result[:len(result)-1]
								}
							}
						}
						result = append(result, sender+"~~~"+messages[m].Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string))
						//} else {
						// TODO: Add spoofing address to the ban list?
						//}
					}
				}
			}
		} else {
			if messages[m].Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
				uname := ""
				for r := range result {
					if r > -1 && r < len(result) {
						if strings.Contains(result[r], messages[m].Destination+"~~~") {
							split := strings.Split(result[r], "~~~")
							uname = split[1]
							copy(result[r:], result[r+1:])
							result[len(result)-1] = ""
							result = result[:len(result)-1]
						}
					}
				}
				result = append(result, messages[m].Destination+"~~~"+uname)
			}
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(result)))
	return
}

// Returns a list of registered usernames from Gnomon
func queryUsernames(address string) (result []string, err error) {
	if gnomon.Index != nil && engram.Disk != nil {
		result, _ = gnomon.Graviton.GetSCIDKeysByValue("0000000000000000000000000000000000000000000000000000000000000001", address, engram.Disk.Get_Daemon_TopoHeight(), false)
		if len(result) <= 0 {
			result, _, err = gnomon.Index.GetSCIDKeysByValue(nil, "0000000000000000000000000000000000000000000000000000000000000001", address, engram.Disk.Get_Daemon_TopoHeight())
			if err != nil {
				logger.Errorf("[Gnomon] Querying usernames failed: %s\n", err)
				return
			}
		}

		sort.Strings(result)
	}

	return
}

// Get the local list of registered usernames saved from previous Gnomon scans
func getUsernames() (result []string, err error) {
	usernames, err := GetEncryptedValue("Usernames", []byte("usernames"))
	if err != nil {
		return
	}

	result = strings.Split(string(usernames), ",")
	return
}

// Set the Primary Username saved to a wallet's datashard
func setPrimaryUsername(s string) (err error) {
	err = StoreEncryptedValue("settings", []byte("username"), []byte(s))
	return
}

// Get the Primary Username saved to a wallet's datashard
func getPrimaryUsername() (err error) {
	u, err := GetEncryptedValue("settings", []byte("username"))
	if err != nil {
		session.Username = ""
		return
	}
	session.Username = string(u)
	return
}

// Start the Gnomon indexer
func startGnomon() {
	if walletapi.Connected {
		if gnomon.Index == nil && gnomon.Active == 1 {
			path := filepath.Join(AppPath(), "datashards", "gnomon")
			switch session.Network {
			case NETWORK_TESTNET:
				path = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
			case NETWORK_SIMULATOR:
				path = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
			}

			// Check if database exists and might be corrupted
			if _, err := os.Stat(path); err == nil {
				// Database directory exists, validate it
				if isDatabaseCorrupted(path) {
					logger.Printf("[Gnomon] Database appears corrupted, attempting recovery...\n")

					// Backup corrupted database
					backupPath := path + "_corrupted_" + time.Now().Format("20060102_150405")
					if err := os.Rename(path, backupPath); err == nil {
						logger.Printf("[Gnomon] Backed up corrupted database to: %s\n", backupPath)

						// Show recovery message to user
						fyne.Do(func() {
							if session.Window != nil {
								dialog.ShowInformation("Database Recovery",
									fmt.Sprintf("Corrupted Gnomon database detected and backed up.\nDatabase will be recreated with fresh sync.\nBackup location: %s", backupPath),
									session.Window)
							}
						})
					}
				}
			}

			// Initialize fresh databases with retry logic and configurable timeout
			var err error
			maxRetries := 3
			baseTimeout := 5 * time.Second // Start with 5 seconds, increase on retry

			// Try to load timeout from settings
			timeoutStr, err := GetValue("settings", []byte("db_timeout"))
			if err == nil {
				if timeout, err := time.ParseDuration(string(timeoutStr)); err == nil {
					baseTimeout = timeout
					logger.Printf("[Gnomon] Using database timeout from settings: %v", baseTimeout)
				}
			} else {
				logger.Printf("[Gnomon] Using default database timeout: %v", baseTimeout)
			}

			for attempt := 0; attempt < maxRetries; attempt++ {
				gnomon.BBolt, err = storage.NewBBoltDB(path, "gnomon")
				if err != nil {
					logger.Printf("[Gmonon] Error creating BBoltDB on attempt %d: %v\n", attempt+1, err)
					if attempt == maxRetries-1 && strings.Contains(err.Error(), "timeout") {
						// Last attempt with timeout, try with increased timeout
						increasedTimeout := time.Duration(attempt+1) * baseTimeout
						logger.Printf("[Gmonon] Final attempt with increased timeout: %v\n", increasedTimeout)
						gnomon.BBolt, err = storage.NewBBoltDB(path, "gnomon")
						if err != nil {
							logger.Printf("[Gmonon] Successfully created BBoltDB with increased timeout: %v\n", increasedTimeout)
							break
						}
					} else {
						// Non-timeout error, don't retry
						logger.Printf("[Gmonon] Non-timeout error on attempt %d: %v\n", attempt+1, err)
						break
					}
				} else {
					logger.Printf("[Gmonon] Successfully created BBoltDB on attempt %d\n", attempt+1)
					break
				}
			}
			gnomon.Graviton, err = storage.NewGravDB(path, "25ms")
			if err != nil {
				logger.Printf("[Gmonon] Error creating GravDB: %v\n", err)
				return
			}
			gnomon.Graviton, err = storage.NewGravDB(path, "25ms")
			if err != nil {
				logger.Printf("[Gnomon] Error creating GravDB: %v\n", err)
				return
			}

			term := []string(nil)
			term = append(term, "Function Initialize")
			height, err := gnomon.Graviton.GetLastIndexHeight()
			if err != nil {
				height = 0
			}

			// Fastsync Config
			config := &structures.FastSyncConfig{
				Enabled:           true,
				SkipFSRecheck:     true,
				ForceFastSync:     true,
				ForceFastSyncDiff: 20,
				NoCode:            true,
			}

			// exclude the Gnomon SC, etc. to keep faster sync times
			var exclusions []string

			gnomon.Index = indexer.NewIndexer(gnomon.Graviton, gnomon.BBolt, "gravdb", term, height, session.Daemon, "daemon", false, false, config, exclusions)
			indexer.InitLog(globals.Arguments, os.Stdout)

			// We can allow parallel processing of x blocks at a time
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Printf("[Gnomon] Critical error in StartDaemonMode: %v\n", r)
						logger.Printf("[Gnomon] This usually indicates corrupted database. Please use 'Clear Local Data' in Advanced settings.\n")

						// Try to gracefully handle the panic by stopping gnomon
						stopGnomon()

						// Show recovery dialog on main thread
						fyne.Do(func() {
							if session.Window != nil {
								dialog.ShowError(fmt.Errorf("Gnomon database corrupted. Please use 'Clear Local Data' in Advanced settings to recover."), session.Window)
							}
						})
					}
				}()
				gnomon.Index.StartDaemonMode(1)
			}()

			logger.Printf("[Gnomon] Scan Status: [%d / %d]\n", height, gnomon.Index.LastIndexedHeight)
		}
	}
}

// Check if database might be corrupted
func isDatabaseCorrupted(path string) bool {
	// Enhanced validation - multiple checks
	logger.Printf("[Gnomon] Validating database at: %s\n", path)

	// Check file permissions and basic integrity
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false // No database = not corrupted
		}
		return true // Permission issues = corruption
	}

	// Try basic validation with recovery
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("[Gnomon] Database validation panic: %v\n", r)
		}
	}()

	// Try to open database safely
	grav, err := storage.NewGravDB(path, "25ms")
	if err != nil {
		logger.Printf("[Gnomon] Cannot open database: %v\n", err)
		return true
	}

	// Multiple validation attempts
	_, err = grav.GetLastIndexHeight()
	if err != nil {
		logger.Printf("[Gnomon] Database validation error: %v\n", err)
		return true
	}

	// Additional validation - try snapshot creation (often exposes corruption)
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("[Gnomon] Snapshot validation panic: %v\n", r)
		}
	}()

	logger.Printf("[Gnomon] Database appears valid, but being cautious...\n")
	return false
}

// Stop all indexers and close Gnomon
func stopGnomon() {
	if gnomon.Index != nil {
		gnomon.Index.Close()
		gnomon.Index = nil
		logger.Printf("[Gnomon] Closed all indexers.\n")
	}
	// CRITICAL FIX: Also close BBolt database to release file lock
	if gnomon.BBolt != nil && gnomon.BBolt.DB != nil {
		gnomon.BBolt.DB.Close()
		gnomon.BBolt = nil
		logger.Printf("[Gnomon] Closed BBolt database.\n")
	}
}

// Save TELA scan progress to encrypted storage
func saveScanProgress(position, total int, lastSCID, state string) {
	progress := ScanProgress{
		Position:  position,
		Total:     total,
		LastSCID:  lastSCID,
		State:     state,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(progress)
	if err != nil {
		logger.Printf("[Gnomon] Error marshaling scan progress: %s\n", err)
		return
	}

	err = StoreEncryptedValue("TELA Search", []byte("ScanProgress"), data)
	if err != nil {
		logger.Printf("[Gnomon] Error saving scan progress: %s\n", err)
	} else {
		logger.Printf("[Gnomon] Saved scan progress: %d/%d, state: %s\n", position, total, state)
	}
}

// Load TELA scan progress from encrypted storage
func loadScanProgress() ScanProgress {
	var progress ScanProgress

	stored, err := GetEncryptedValue("TELA Search", []byte("ScanProgress"))
	if err != nil || stored == nil {
		return progress
	}

	if err := json.Unmarshal(stored, &progress); err != nil {
		logger.Printf("[Gnomon] Error loading scan progress: %s\n", err)
		return ScanProgress{}
	}

	logger.Printf("[Gnomon] Loaded scan progress: %d/%d, state: %s, lastSCID: %s\n",
		progress.Position, progress.Total, progress.State, progress.LastSCID)

	return progress
}

// Clear TELA scan progress from storage
func clearScanProgress() {
	err := StoreEncryptedValue("TELA Search", []byte("ScanProgress"), nil)
	if err != nil {
		logger.Printf("[Gnomon] Error clearing scan progress: %s\n", err)
	} else {
		logger.Printf("[Gnomon] Cleared scan progress\n")
	}
}

// Clear all TELA cache from storage (comprehensive cache clear)
func clearAllTELACache() {
	keysToClear := [][]byte{
		[]byte("ScanProgress"),
		[]byte("SCIDs"),
		[]byte("Searched SCIDs"),
		[]byte("Last Scan"),
	}
	for _, key := range keysToClear {
		if err := DeleteKey("TELA Search", key); err != nil {
			logger.Printf("[TELA] Error clearing key %s: %v\n", string(key), err)
		} else {
			logger.Printf("[TELA] Cleared key: %s\n", string(key))
		}
	}
	logger.Printf("[TELA] Cleared all TELA cache from storage\n")
}

// Reset Gnomon index for a complete fresh scan
func resetGnomonIndex() error {
	if gnomon.Index != nil {
		logger.Printf("[Gnomon] Stopping indexer for full reset...\n")
		stopGnomon()
		time.Sleep(1 * time.Second)
	}

	path := filepath.Join(AppPath(), "datashards", "gnomon")
	switch session.Network {
	case NETWORK_TESTNET:
		path = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
	case NETWORK_SIMULATOR:
		path = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
	}

	if _, err := os.Stat(path); err == nil {
		backupPath := path + "_reset_" + time.Now().Format("20060102_150405")
		if err := os.Rename(path, backupPath); err != nil {
			logger.Printf("[Gnomon] Error backing up database: %v\n", err)
			return err
		}
		logger.Printf("[Gnomon] Backed up database to: %s\n", backupPath)
	}

	gnomon.Active = 1
	go startGnomon()
	logger.Printf("[Gnomon] Started fresh indexing\n")
	return nil
}

// Check if scan progress is stale (older than specified hours)
func isScanProgressStale(progress ScanProgress, hours int) bool {
	if progress.Timestamp == 0 {
		return true
	}
	age := time.Now().Unix() - progress.Timestamp
	return age > int64(hours*3600)
}

// Check if an error is a connection-related error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"timeout",
		"no route to host",
		"network is unreachable",
		"broken pipe",
		"EOF",
		"i/o timeout",
		"context deadline exceeded",
		"dial tcp",
	}
	errStrLower := strings.ToLower(errStr)
	for _, ce := range connectionErrors {
		if strings.Contains(errStrLower, ce) {
			return true
		}
	}
	return false
}

// Method of Gnomon GetAllOwnersAndSCIDs() where DB type is defined by Indexer.DBType
func (g *Gnomon) GetAllOwnersAndSCIDs() (scids map[string]string) {
	if g.Index == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetAllOwnersAndSCIDs()
	case "boltdb":
		return g.Index.BBSBackend.GetAllOwnersAndSCIDs()
	default:
		return
	}
}

// Method of Gnomon GetAllSCIDVariableDetails() where DB type is defined by Indexer.DBType
func (g *Gnomon) GetAllSCIDVariableDetails(scid string) (vars []*structures.SCIDVariable) {
	if g.Index == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetAllSCIDVariableDetails(scid)
	case "boltdb":
		return g.Index.BBSBackend.GetAllSCIDVariableDetails(scid)
	default:
		return
	}
}

// Method of Gnomon GetSCIDValuesByKey() where DB type is defined by Indexer.DBType
func (g *Gnomon) GetSCIDValuesByKey(scid string, key interface{}) (valuesstring []string, valuesuint64 []uint64) {
	if g.Index == nil {
		return
	}
	switch g.Index.DBType {
	case "gravdb":
		return g.Index.GravDBBackend.GetSCIDValuesByKey(scid, key, g.Index.ChainHeight, true)
	case "boltdb":
		return g.Index.BBSBackend.GetSCIDValuesByKey(scid, key, g.Index.ChainHeight, true)
	default:
		return
	}
}

// Add a var store only scid to Gnomon DB
func (g *Gnomon) AddSCIDToIndex(scid string) (err error) {
	if g.Index == nil {
		return fmt.Errorf("gnomon index is nil")
	}
	add := make(map[string]*structures.FastSyncImport)
	add[scid] = &structures.FastSyncImport{}

	return gnomon.Index.AddSCIDToIndex(add, false, true)
}

// preIndexFavorites indexes all favorited SCIDs in the background
func preIndexFavorites(favs map[string]*TELAFavoriteData) {
	if gnomon.Index == nil {
		return
	}

	for scid := range favs {
		if gnomon.GetAllSCIDVariableDetails(scid) == nil {
			logger.Printf("[Engram] Pre-indexing favorite SCID: %s\n", scid)
			if err := gnomon.AddSCIDToIndex(scid); err != nil {
				logger.Errorf("[Engram] Failed to pre-index %s: %v\n", scid, err)
			}
		}
	}
}

// Get the current code of a smart contract
func getContractCode(scid string) (code string, err error) {
	var params = rpc.GetSC_Params{SCID: scid, Variables: false, Code: true}
	var result rpc.GetSC_Result

	rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
	if err != nil {
		return
	}

	input_output := rwc.New(rpc_client.WS)
	rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

	err = rpc_client.RPC.CallResult(context.Background(), "DERO.GetSC", params, &result)
	if err != nil {
		logger.Errorf("[Engram] Error getting SC code: %s\n", err)
		return
	}

	code = result.Code

	return
}

// DVM starter InitializePrivate() example for smart contract builder
func dvmInitFuncExample() string {
	return `Function InitializePrivate() Uint64
10 IF EXISTS("owner") == 0 THEN GOTO 30
20 RETURN 1
30 STORE("owner", SIGNER())
31 STORE("var_header_name", "")
32 STORE("var_header_description", "")
33 STORE("var_header_icon", "")
40 RETURN 0
End Function`
}

// DVM starter function for smart contract builder
func dvmFuncExample(increment int) string {
	return `Function new` + fmt.Sprintf("%d", increment) + `() Uint64
10
20
30 RETURN 0
End Function`
}

// Verification overlay for user actions with or without password
func verificationOverlay(password bool, headerText, subText, dismiss string, callback func(bool)) {
	overlay := session.Window.Canvas().Overlays()

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 5))

	if password {
		headerText = "ACCOUNT  VERIFICATION  REQUIRED"
		dismiss = "Submit"
	}

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 14
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	btnConfirm := widget.NewButton(dismiss, nil)
	btnConfirm.Disable()

	entryPassword := NewReturnEntry()
	entryPassword.Password = true
	entryPassword.PlaceHolder = "Password"
	entryPassword.OnChanged = func(s string) {
		if s == "" {
			btnConfirm.Text = dismiss
			btnConfirm.Disable()
			btnConfirm.Refresh()
		} else {
			btnConfirm.Text = dismiss
			btnConfirm.Enable()
			btnConfirm.Refresh()
		}
	}

	subHeader := canvas.NewText(subText, colors.Account)
	if password {
		subText = "Confirm Password"
		subHeader.TextSize = 22
	} else {
		subHeader.TextSize = 18
		entryPassword.Hide()
		btnConfirm.Enable()
		btnConfirm.Refresh()
	}

	subHeader.Text = subText
	subHeader.Alignment = fyne.TextAlignCenter
	subHeader.TextStyle = fyne.TextStyle{Bold: true}
	subHeader.Refresh()

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		callback(false)
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	btnConfirm.OnTapped = func() {
		btnConfirm.Disable()
		if password {
			if engram.Disk.Check_Password(entryPassword.Text) {
				callback(true)
				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())
			} else {
				btnConfirm.Text = "Invalid Password..."
				btnConfirm.Refresh()
			}
		} else {
			callback(true)
			overlay.Top().Hide()
			overlay.Remove(overlay.Top())
			overlay.Remove(overlay.Top())
		}
	}

	entryPassword.OnReturn = btnConfirm.OnTapped

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

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
					container.NewCenter(
						container.NewStack(
							span,
							entryPassword,
						),
					),
					rectSpacer,
					rectSpacer,
					btnConfirm,
					rectSpacer,
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						btnBack,
						layout.NewSpacer(),
					),
					rectSpacer,
					rectSpacer,
				),
			),
		),
	)

	if password {
		session.Window.Canvas().Focus(entryPassword)
	}
}

// Color for the TELA likes ratio and individual rating numbers
func telaRatingColor(r uint64) color.Color {
	if r > 65 {
		return colors.Green
	} else if r > 32 {
		return colors.Yellow
	} else {
		return colors.Red
	}
}

// Color for the TELA average rating number hexagon
func telaHexagonColor(r float64) fyne.Resource {
	if r > 6.5 {
		return resourceTelaHexagonGreen
	} else if r > 3.2 {
		return resourceTelaHexagonYellow
	} else {
		return resourceTelaHexagonRed
	}
}

// Display ratings overview and the details of each rating for the TELA SCID
func viewTELARatingsOverlay(name, scid string) (err error) {
	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(fyne.NewSize(ui.Width, 10))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 5))

	header := canvas.NewText("TELA  RATINGS", colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	if len(name) > 30 {
		name = fmt.Sprintf("%s...", name[0:30])
	}

	nameHdr := canvas.NewText(name, colors.Account)
	nameHdr.Alignment = fyne.TextAlignCenter
	nameHdr.TextStyle = fyne.TextStyle{Bold: true}

	labelSCID := canvas.NewText("   SMART  CONTRACT  ID", colors.Gray)
	labelSCID.TextSize = 14
	labelSCID.Alignment = fyne.TextAlignLeading
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	textSCID := widget.NewRichTextWithText(scid)
	textSCID.Wrapping = fyne.TextWrapWord

	textLikes := widget.NewRichTextFromMarkdown("Likes:")
	textDislikes := widget.NewRichTextFromMarkdown("Dislikes:")
	textAverage := widget.NewRichTextFromMarkdown("Average:")

	ratingsBox := container.NewVBox(labelSCID, textSCID)

	ratings, err := tela.GetRating(scid, session.Daemon, 0)
	if err != nil {
		removeOverlays()
		logger.Errorf("[Engram] GetRating: %s\n", err)
		err = fmt.Errorf("error could not get ratings")
		return
	}

	removeOverlays()
	overlay := session.Window.Canvas().Overlays()

	ratingsBox.Add(container.NewHBox(textLikes, canvas.NewText(fmt.Sprintf("%d", ratings.Likes), colors.Green)))
	ratingsBox.Add(container.NewHBox(textDislikes, canvas.NewText(fmt.Sprintf("%d", ratings.Dislikes), colors.Red)))
	ratingsBox.Add(container.NewHBox(textAverage, canvas.NewText(fmt.Sprintf("%0.1f/10", ratings.Average), colors.Account)))

	linkRate := widget.NewHyperlinkWithStyle("Rate SCID", nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkRate.OnTapped = func() {
		rateTELAOverlay(name, scid)
	}
	linkRate.Hide()

	// Check if wallet has rated SCID
	if gnomon.Index != nil {
		ratingStore, _ := gnomon.GetSCIDValuesByKey(scid, engram.Disk.GetAddress().String())
		if ratingStore == nil {
			linkRate.Show()
		}
	}

	linkBack := widget.NewHyperlinkWithStyle("Back to Application", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkBack.OnTapped = func() {
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	}

	labelSeparator := widget.NewRichTextFromMarkdown("")
	labelSeparator.Wrapping = fyne.TextWrapOff
	labelSeparator.ParseMarkdown("---")
	labelSeparator2 := widget.NewRichTextFromMarkdown("")
	labelSeparator2.Wrapping = fyne.TextWrapOff
	labelSeparator2.ParseMarkdown("---")

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	userRatingsBox := container.NewVBox()

	for _, r := range ratings.Ratings {
		ratingString, err := tela.Ratings.ParseString(r.Rating)
		if err != nil {
			ratingString = fmt.Sprintf("%d", r.Rating)
		}

		labelSeparator := widget.NewRichTextFromMarkdown("")
		labelSeparator.Wrapping = fyne.TextWrapOff
		labelSeparator.ParseMarkdown("---")

		labelAddress := widget.NewRichTextFromMarkdown(r.Address)
		labelAddress.Wrapping = fyne.TextWrapWord

		userRatingsBox.Add(
			container.NewVBox(
				labelAddress,
				container.NewHBox(widget.NewRichTextFromMarkdown("Height:"), canvas.NewText(fmt.Sprintf("%d", r.Height), colors.Account)),
				container.NewHBox(widget.NewRichTextFromMarkdown("Rating:"), canvas.NewText(fmt.Sprintf("%d", r.Rating), telaRatingColor(r.Rating))),
				widget.NewRichTextFromMarkdown(ratingString),
				labelSeparator,
			),
		)
	}

	userRatingsBoxScroll := container.NewVScroll(
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				ratingsBox,
				rectWidth90,
				container.NewHBox(
					linkRate,
					layout.NewSpacer(),
				),
				rectSpacer,
				rectSpacer,
				labelSeparator2,
				rectSpacer,
				rectSpacer,
				userRatingsBox,
			),
			layout.NewSpacer(),
		),
	)
	userRatingsBoxScroll.SetMinSize(fyne.NewSize(ui.Width*0.80, ui.Height*0.50))

	overlayCont := container.NewVBox(
		span,
		container.NewCenter(
			header,
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			nameHdr,
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		labelSeparator,
		rectSpacer,
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
		),
		userRatingsBoxScroll,
		rectSpacer,
		rectSpacer,
		rectSpacer,
		rectSpacer,
		rectSpacer,
	)

	menuLabel := canvas.NewText(" ", colors.Gray)
	menuLabel.TextSize = 11
	menuLabel.Alignment = fyne.TextAlignCenter
	menuLabel.TextStyle = fyne.TextStyle{Bold: true}

	sep := canvas.NewRectangle(colors.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	line2 := container.NewVBox(
		layout.NewSpacer(),
		sep,
		layout.NewSpacer(),
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			rectSpacer,
			container.NewStack(
				container.NewHBox(
					layout.NewSpacer(),
					line1,
					layout.NewSpacer(),
					menuLabel,
					layout.NewSpacer(),
					line2,
					layout.NewSpacer(),
				),
			),
			rectSpacer,
			rectSpacer,
			container.NewCenter(
				layout.NewSpacer(),
				linkBack,
				layout.NewSpacer(),
			),
			rectSpacer,
			rectSpacer,
			rectSpacer,
			rectSpacer,
		),
	)

	overlay.Add(
		container.NewStack(
			&iframe{},
			canvas.NewRectangle(colors.DarkMatter),
		),
	)

	overlay.Add(
		container.NewStack(
			&iframe{},
			container.NewBorder(
				nil,
				bottom,
				nil,
				nil,
				overlayCont,
			),
		),
	)

	return
}

// TELA smart contract rating overlay with password confirmation
func rateTELAOverlay(name, scid string) {
	overlay := session.Window.Canvas().Overlays()

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(10, 5))

	selectSpacer := canvas.NewRectangle(color.Transparent)
	selectSpacer.SetMinSize(fyne.NewSize(80, 5))

	header := canvas.NewText("RATE  TELA  INDEX", colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	if len(name) > 30 {
		name = fmt.Sprintf("%s...", name[0:30])
	}

	nameHdr := canvas.NewText(name, colors.Account)
	nameHdr.Alignment = fyne.TextAlignCenter
	nameHdr.TextStyle = fyne.TextStyle{Bold: true}

	btnConfirm := widget.NewButton("Rate", nil)
	btnConfirm.Disable()

	var telaCategories, negativeDetails, positiveDetails []string
	for i := uint64(0); i < 10; i++ {
		telaCategories = append(telaCategories, tela.Ratings.Category(i))
	}
	categorySelect := widget.NewSelect(telaCategories, nil)

	scidLabel := canvas.NewText("   SMART  CONTRACT  ID", colors.Gray)
	scidLabel.TextSize = 14
	scidLabel.Alignment = fyne.TextAlignCenter
	scidLabel.TextStyle = fyne.TextStyle{Bold: true}

	scidText := widget.NewRichTextFromMarkdown(scid)
	scidText.Wrapping = fyne.TextWrapWord

	errorText := canvas.NewText(" ", colors.Green)
	errorText.TextSize = 12
	errorText.Alignment = fyne.TextAlignCenter

	for i := uint64(0); i < 10; i++ {
		negativeDetails = append(negativeDetails, tela.Ratings.Detail(i, false))
		positiveDetails = append(positiveDetails, tela.Ratings.Detail(i, true))
	}
	negativeSelect := widget.NewSelect(negativeDetails, nil)
	positiveSelect := widget.NewSelect(positiveDetails, nil)

	categoryHeader := canvas.NewText("Category", colors.Account)
	categoryHeader.Alignment = fyne.TextAlignLeading
	categoryHeader.TextStyle = fyne.TextStyle{Bold: true}
	categoryHeader.Refresh()

	detailHeader := canvas.NewText("Detail", colors.Account)
	detailHeader.Alignment = fyne.TextAlignLeading
	detailHeader.TextStyle = fyne.TextStyle{Bold: true}
	detailHeader.Refresh()

	ratingHeader := canvas.NewText("Rating", colors.Account)
	ratingHeader.Alignment = fyne.TextAlignLeading
	ratingHeader.TextStyle = fyne.TextStyle{Bold: true}
	ratingHeader.Refresh()

	ratingText := widget.NewRichTextFromMarkdown("")
	ratingText.Wrapping = fyne.TextWrapWord

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		overlay.Top().Hide()
		overlay.Remove(overlay.Top())
		overlay.Remove(overlay.Top())
	})

	btnConfirm.OnTapped = func() {
		errorText.Text = ""
		errorText.Refresh()
		btnConfirm.Disable()
		if gnomon.Index != nil {
			var ratingStore []string
			switch gnomon.Index.DBType {
			case "gravdb":
				ratingStore, _ = gnomon.Index.GravDBBackend.GetSCIDValuesByKey(scid, engram.Disk.GetAddress().String(), gnomon.Index.LastIndexedHeight, false)
			case "boltdb":
				ratingStore, _ = gnomon.Index.BBSBackend.GetSCIDValuesByKey(scid, engram.Disk.GetAddress().String(), gnomon.Index.LastIndexedHeight, false)
			}
			if ratingStore != nil {
				errorText.Text = "already rated this contract"
				errorText.Color = colors.Red
				errorText.Refresh()
				return
			}
		}

		category := categorySelect.SelectedIndex()
		if category < 0 {
			errorText.Text = "select a category"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		var detail int
		if category > 4 {
			detail = positiveSelect.SelectedIndex()
		} else {
			detail = negativeSelect.SelectedIndex()
		}

		if detail < 0 {
			errorText.Text = "select a detail"
			errorText.Color = colors.Red
			errorText.Refresh()
			return
		}

		rating := (category * 10) + detail

		verificationOverlay(
			true,
			"",
			"",
			"",
			func(b bool) {
				if !b {
					btnConfirm.Enable()
					return
				}

				overlay.Top().Hide()
				overlay.Remove(overlay.Top())
				overlay.Remove(overlay.Top())

				txid, err := tela.Rate(engram.Disk, scid, uint64(rating))
				if err != nil {
					logger.Errorf("[Engram] Rate TX: %s\n", err)
					return
				}

				logger.Printf("[Engram] Rate TXID: %s\n", txid)
			},
		)
	}

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

	overlayCont := container.NewVBox(
		span,
		container.NewCenter(
			header,
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			nameHdr,
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		scidLabel,
		scidText,
		widget.NewLabel(""),
		container.NewCenter(
			container.NewStack(
				span,
				container.NewBorder(
					nil,
					nil,
					container.NewStack(
						selectSpacer,
						categoryHeader,
					),
					nil,
					categorySelect,
				),
			),
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewStack(
				span,
				container.NewBorder(
					nil,
					nil,
					container.NewStack(
						selectSpacer,
						detailHeader,
					),
					nil,
					positiveSelect,
				),
			),
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			container.NewStack(
				span,
				container.NewBorder(
					nil,
					nil,
					container.NewStack(
						selectSpacer,
						ratingHeader,
					),
					nil,
					ratingText,
				),
			),
		),
		rectSpacer,
		rectSpacer,
		rectSpacer,
		rectSpacer,
		errorText,
		rectSpacer,
		rectSpacer,
		btnConfirm,
		rectSpacer,
		rectSpacer,
		container.NewHBox(
			layout.NewSpacer(),
			btnBack,
			layout.NewSpacer(),
		),
		rectSpacer,
		rectSpacer,
	)

	categorySelect.OnChanged = func(s string) {
		if positiveSelect.SelectedIndex() > -1 && negativeSelect.SelectedIndex() > -1 {
			btnConfirm.Enable()
		}

		if categorySelect.SelectedIndex() > 4 {
			overlayCont.Objects[17] = container.NewCenter(
				container.NewStack(
					span,
					container.NewBorder(
						nil,
						nil,
						container.NewStack(
							selectSpacer,
							detailHeader,
						),
						nil,

						positiveSelect,
					),
				),
			)
			positiveSelect.SetSelectedIndex(0)
			ratingText.ParseMarkdown(fmt.Sprintf("%d", (categorySelect.SelectedIndex()*10)+positiveSelect.SelectedIndex()))
		} else {
			overlayCont.Objects[17] = container.NewCenter(
				container.NewStack(
					span,
					container.NewBorder(
						nil,
						nil,
						container.NewStack(
							selectSpacer,
							detailHeader,
						),
						nil,
						negativeSelect,
					),
				),
			)
			negativeSelect.SetSelectedIndex(0)
			ratingText.ParseMarkdown(fmt.Sprintf("%d", (categorySelect.SelectedIndex()*10)+negativeSelect.SelectedIndex()))
		}
	}

	positiveSelect.OnChanged = func(s string) {
		if categorySelect.SelectedIndex() > -1 {
			btnConfirm.Enable()
			ratingText.ParseMarkdown(fmt.Sprintf("%d", (categorySelect.SelectedIndex()*10)+positiveSelect.SelectedIndex()))
		}
	}

	negativeSelect.OnChanged = func(s string) {
		if categorySelect.SelectedIndex() > -1 {
			btnConfirm.Enable()
			ratingText.ParseMarkdown(fmt.Sprintf("%d", (categorySelect.SelectedIndex()*10)+negativeSelect.SelectedIndex()))
		}
	}

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
				overlayCont,
			),
		),
	)
}

// Install a new smart contract
func installSC(code string, args []rpc.Argument) (txid string, err error) {
	var dest string
	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	case NETWORK_TESTNET:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}

	transfer := rpc.Transfer{
		Destination: dest,
		Amount:      0,
		Burn:        0,
	}

	_, err = transfer.Payload_RPC.CheckPack(transaction.PAYLOAD0_LIMIT)
	if err != nil {
		logger.Errorf("[Engram] Install arguments packing err: %s\n", err)
		err = fmt.Errorf("contract install pack error")
		return
	}

	// decode SC from base64 if possible
	if sc, err := base64.StdEncoding.DecodeString(code); err == nil {
		code = string(sc)
	}

	args = append(args, rpc.Argument{Name: rpc.SCACTION, DataType: rpc.DataUint64, Value: uint64(rpc.SC_INSTALL)})
	args = append(args, rpc.Argument{Name: rpc.SCCODE, DataType: rpc.DataString, Value: code})

	fees := uint64(0)
	gasParams := rpc.GasEstimate_Params{
		Transfers: []rpc.Transfer{transfer},
		SC_Code:   code,
		SC_Value:  0,
		SC_RPC:    args,
		Ringsize:  2,
		Signer:    engram.Disk.GetAddress().String(),
	}

	if gas, err := getGasEstimate(gasParams); err == nil {
		fees = gas
		logger.Printf("[Engram] SC install fees: %d\n", fees)
	} else {
		// uses default fees
		logger.Errorf("[Engram] Error estimating fees: %s\n", err)
	}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{transfer}, 2, false, args, fees, false)
	if err != nil {
		logger.Errorf("[Engram] Error while building install transaction: %s\n", err)
		err = fmt.Errorf("contract install build error")
		return
	}

	if err = engram.Disk.SendTransaction(tx); err != nil {
		logger.Errorf("[Engram] Error while dispatching install transaction: %s\n", err)
		err = fmt.Errorf("contract install dispatch error")
		return
	}

	txid = tx.GetHash().String()

	logger.Printf("[Engram] SC Installed: %s\n", txid)

	return
}

// Set the Cyberdeck password
func newRPCPassword() (s string) {
	r := make([]byte, 20)
	_, err := rand.Read(r)
	if err != nil {
		panic(err)
	}

	s = base64.URLEncoding.EncodeToString(r)
	cyberdeck.RPC.pass = s
	return
}

// Set the Cyberdeck username
func newRPCUsername() (s string) {
	r, _ := rand.Int(rand.Reader, big.NewInt(1600))
	w := mnemonics.Key_To_Words(r, "english")
	l := strings.Split(string(w), " ")
	s = l[len(l)-2]
	cyberdeck.RPC.user = s
	return
}

// Start an RPC server to allow decentralized application communication
func toggleRPCServer(port string) {
	var err error
	if engram.Disk == nil {
		return
	}

	if cyberdeck.RPC.server != nil {
		cyberdeck.RPC.server.RPCServer_Stop()
		cyberdeck.RPC.server = nil
		cyberdeck.RPC.status.Text = "Blocked"
		cyberdeck.RPC.status.Color = colors.Gray
		cyberdeck.RPC.status.Refresh()
		cyberdeck.RPC.toggle.Text = "Turn On"
		cyberdeck.RPC.toggle.Refresh()
		status.Cyberdeck.FillColor = colors.Gray
		status.Cyberdeck.StrokeColor = colors.Gray
		status.Cyberdeck.Refresh()
		cyberdeck.RPC.userText.Text = cyberdeck.RPC.user
		cyberdeck.RPC.passText.Text = cyberdeck.RPC.pass
		cyberdeck.RPC.userText.Enable()
		cyberdeck.RPC.passText.Enable()
		logger.Printf("[Engram] RPC server closed\n")
	} else {
		logger.Printf("[Engram] Starting RPC server %s\n", port)

		globals.Arguments["--rpc-bind"] = port

		if cyberdeck.RPC.user == "" {
			cyberdeck.RPC.user = newRPCUsername()
		}

		if cyberdeck.RPC.pass == "" {
			cyberdeck.RPC.pass = newRPCPassword()
		}

		globals.Arguments["--rpc-login"] = cyberdeck.RPC.user + ":" + cyberdeck.RPC.pass

		cyberdeck.RPC.server, err = rpcserver.RPCServer_Start(engram.Disk, "Cyberdeck")
		if err != nil {
			cyberdeck.RPC.server = nil
			cyberdeck.RPC.status.Text = "Blocked"
			cyberdeck.RPC.status.Color = colors.Gray
			cyberdeck.RPC.status.Refresh()
			cyberdeck.RPC.toggle.Text = "Turn On"
			cyberdeck.RPC.toggle.Refresh()
			status.Cyberdeck.FillColor = colors.Gray
			status.Cyberdeck.StrokeColor = colors.Gray
			status.Cyberdeck.Refresh()
			cyberdeck.RPC.userText.Text = cyberdeck.RPC.user
			cyberdeck.RPC.passText.Text = cyberdeck.RPC.pass
			cyberdeck.RPC.userText.Enable()
			cyberdeck.RPC.passText.Enable()
		} else {
			cyberdeck.RPC.status.Text = "Allowed"
			cyberdeck.RPC.status.Color = colors.Green
			cyberdeck.RPC.status.Refresh()
			cyberdeck.RPC.toggle.Text = "Turn Off"
			cyberdeck.RPC.toggle.Refresh()
			status.Cyberdeck.FillColor = colors.Green
			status.Cyberdeck.StrokeColor = colors.Green
			status.Cyberdeck.Refresh()
			cyberdeck.RPC.userText.Text = cyberdeck.RPC.user
			cyberdeck.RPC.passText.Text = cyberdeck.RPC.pass
			cyberdeck.RPC.userText.Disable()
			cyberdeck.RPC.passText.Disable()
		}
	}
}

// Get the latest smart contract header data (must follow the standard here: https://github.com/civilware/artificer-nfa-standard/blob/main/Headers/README.md)
func getContractHeader(scid crypto.Hash) (name string, desc string, icon string, owner string, code string) {
	var headerData []*structures.SCIDVariable
	var found bool

	switch gnomon.Index.DBType {
	case "gravdb":
		headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(scid.String())
	case "boltdb":
		headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(scid.String())
	}
	if headerData == nil {
		addIndex := make(map[string]*structures.FastSyncImport)
		addIndex[scid.String()] = &structures.FastSyncImport{}
		gnomon.Index.AddSCIDToIndex(addIndex, false, true)
		switch gnomon.Index.DBType {
		case "gravdb":
			headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(scid.String())
		case "boltdb":
			headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(scid.String())
		}
	}

	for _, h := range headerData {
		switch key := h.Key.(type) {
		case string:
			if key == "var_header_name" {
				found = true
				name = h.Value.(string)
			} else if name == "" && key == "nameHdr" {
				found = true
				name = h.Value.(string)
			}

			if key == "var_header_description" {
				found = true
				desc = h.Value.(string)
			} else if desc == "" && key == "descrHdr" {
				found = true
				desc = h.Value.(string)
			}

			if key == "var_header_icon" {
				found = true
				icon = h.Value.(string)
			} else if icon == "" && key == "iconURLHdr" {
				found = true
				icon = h.Value.(string)
			}

			if key == "owner" {
				owner = h.Value.(string)
			}

			if key == "C" {
				code = h.Value.(string)
			}
		}
	}

	// Secondary check for headers in Gnomon SC
	if !found {
		switch gnomon.Index.DBType {
		case "gravdb":
			headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
		case "boltdb":
			headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
		}
		if headerData == nil {
			addIndex := make(map[string]*structures.FastSyncImport)
			addIndex[structures.MAINNET_GNOMON_SCID] = &structures.FastSyncImport{}
			gnomon.Index.AddSCIDToIndex(addIndex, false, true)
			switch gnomon.Index.DBType {
			case "gravdb":
				headerData = gnomon.Index.GravDBBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
			case "boltdb":
				headerData = gnomon.Index.BBSBackend.GetAllSCIDVariableDetails(structures.MAINNET_GNOMON_SCID)
			}
		}

		for _, h := range headerData {
			if strings.Contains(h.Key.(string), scid.String()) {
				switch key := h.Key.(type) {
				case string:
					if key == scid.String() {
						query := h.Value.(string)
						header := strings.Split(query, ";")

						if len(header) > 2 {
							name = header[0]
							desc = header[1]
							icon = header[2]
						}
					}

					if key == scid.String()+"owner" {
						owner = h.Value.(string)
					}
				}
			}
		}
	}

	return
}

// Send an asset from one account to another
func transferAsset(scid crypto.Hash, ringsize uint64, address string, amount string) (txid crypto.Hash, err error) {
	var amount_to_transfer uint64

	if amount == "" {
		amount = ".00001"
	}

	amount_to_transfer, err = globals.ParseAmount(amount)
	if err != nil {
		logger.Errorf("[Transfer] Failed parsing transfer amount: %s\n", err)
		return
	}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{{SCID: scid, Amount: amount_to_transfer, Destination: address}}, ringsize, false, rpc.Arguments{}, 0, false)
	if err != nil {
		logger.Errorf("[Transfer] Failed to build transaction: %s\n", err)
		return
	}

	if err = engram.Disk.SendTransaction(tx); err != nil {
		logger.Errorf("[Transfer] Failed to send asset: %s - %s\n", scid, err)
		return
	}

	txid = tx.GetHash()

	logger.Printf("[Transfer] Successfully sent asset: %s - TXID: %s\n", scid, tx.GetHash().String())
	return
}

// Transfer a username to another account
func transferUsername(username string, address string) (storage uint64, err error) {
	var args = rpc.Arguments{}
	var dest string

	scid := crypto.HashHexToHash("0000000000000000000000000000000000000000000000000000000000000001")

	args = append(args, rpc.Argument{Name: "entrypoint", DataType: "S", Value: "TransferOwnership"})
	args = append(args, rpc.Argument{Name: "SC_ID", DataType: "H", Value: scid})
	args = append(args, rpc.Argument{Name: "SC_ACTION", DataType: "U", Value: uint64(rpc.SC_CALL)})
	args = append(args, rpc.Argument{Name: "name", DataType: "S", Value: username})
	args = append(args, rpc.Argument{Name: "newowner", DataType: "S", Value: address})

	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	default:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}

	transfer := rpc.Transfer{
		Destination: dest,
		Amount:      0,
		Burn:        0,
	}

	gasParams := rpc.GasEstimate_Params{
		SC_RPC:    args,
		SC_Value:  0,
		Ringsize:  2,
		Signer:    engram.Disk.GetAddress().String(),
		Transfers: []rpc.Transfer{transfer},
	}

	storage, err = getGasEstimate(gasParams)
	if err != nil {
		logger.Errorf("[%s] Error estimating fees: %s\n", "TransferOwnership", err)
		return
	}

	tx, err := engram.Disk.TransferPayload0([]rpc.Transfer{transfer}, 2, false, args, storage, false)
	if err != nil {
		logger.Errorf("[%s] Error while building transaction: %s\n", "TransferOwnership", err)
		return
	}

	txid := tx.GetHash().String()

	err = engram.Disk.SendTransaction(tx)
	if err != nil {
		logger.Errorf("[%s] Error while dispatching transaction: %s\n", "TransferOwnership", err)
		return
	}

	walletapi.WaitNewHeightBlock()
	logger.Printf("[%s] Username transfer successful - TXID:  %s\n", "TransferOwnership", txid)
	_ = tx

	return
}

// Execute arbitrary exportable smart contract functions
func executeContractFunction(scid crypto.Hash, ringsize uint64, dero_amount uint64, asset_amount uint64, funcName string, params []dvm.Variable) (storage uint64, err error) {
	var args = rpc.Arguments{}
	var zero uint64
	var dest string

	args = append(args, rpc.Argument{Name: "entrypoint", DataType: "S", Value: funcName})
	args = append(args, rpc.Argument{Name: "SC_ID", DataType: "H", Value: scid})
	args = append(args, rpc.Argument{Name: "SC_ACTION", DataType: "U", Value: uint64(rpc.SC_CALL)})

	for p := range params {
		if params[p].Type == 0x4 {
			args = append(args, rpc.Argument{Name: params[p].Name, DataType: "U", Value: params[p].ValueUint64})
		} else {
			args = append(args, rpc.Argument{Name: params[p].Name, DataType: "S", Value: params[p].ValueString})
		}
	}

	switch session.Network {
	case NETWORK_MAINNET:
		dest = "dero1qykyta6ntpd27nl0yq4xtzaf4ls6p5e9pqu0k2x4x3pqq5xavjsdxqgny8270"
	case NETWORK_SIMULATOR:
		dest = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
	default:
		dest = "deto1qy0ehnqjpr0wxqnknyc66du2fsxyktppkr8m8e6jvplp954klfjz2qqdzcd8p"
	}

	var transfers []rpc.Transfer

	if dero_amount != zero {
		burn := dero_amount

		transfer := rpc.Transfer{
			Destination: dest,
			Amount:      0,
			Burn:        burn,
		}

		transfers = append(transfers, transfer)
	}
	if asset_amount != zero {
		burn := asset_amount

		transfer := rpc.Transfer{
			SCID:        scid,
			Destination: dest,
			Amount:      0,
			Burn:        burn,
		}

		transfers = append(transfers, transfer)
	}

	if len(transfers) < 1 {
		transfer := rpc.Transfer{
			Destination: dest,
			Amount:      0,
			Burn:        0,
		}

		transfers = append(transfers, transfer)
	}

	gasParams := rpc.GasEstimate_Params{
		SC_RPC:    args,
		SC_Value:  0,
		Ringsize:  ringsize,
		Signer:    engram.Disk.GetAddress().String(),
		Transfers: transfers,
	}

	storage, err = getGasEstimate(gasParams)
	if err != nil {
		logger.Errorf("[%s] Error estimating fees: %s\n", funcName, err)
		return
	}

	tx, err := engram.Disk.TransferPayload0(transfers, ringsize, false, args, storage, false)
	if err != nil {
		logger.Errorf("[%s] Error while building transaction: %s\n", funcName, err)
		return
	}

	err = engram.Disk.SendTransaction(tx)
	if err != nil {
		logger.Errorf("[%s] Error while dispatching transaction: %s\n", funcName, err)
		return
	}

	walletapi.WaitNewHeightBlock()
	logger.Printf("[%s] Function execution successful - TXID:  %s\n", funcName, tx.GetHash().String())
	_ = tx

	return
}

// Delete the Gnomon directory
func cleanGnomonData() error {
	path := filepath.Join(AppPath(), "datashards", "gnomon")
	switch session.Network {
	case NETWORK_TESTNET:
		path = filepath.Join(AppPath(), "datashards", "gnomon_testnet")
	case NETWORK_SIMULATOR:
		path = filepath.Join(AppPath(), "datashards", "gnomon_simulator")
	}

	dir, err := os.ReadDir(path)
	if err != nil {
		logger.Errorf("[Gnomon] Error purging local Gnomon data: %s\n", err)
		return err
	}

	for _, d := range dir {
		os.RemoveAll(filepath.Join([]string{path, d.Name()}...))
		logger.Printf("[Gnomon] Local Gnomon data has been purged successfully\n")
	}

	return nil
}

// Delete the datashard directory for the active wallet
func cleanWalletData() (err error) {
	path, err := GetShard()
	if err != nil {
		return
	}

	dir, err := os.ReadDir(path)
	if err != nil {
		logger.Errorf("[Engram] Error purging local datashard data: %s\n", err)
		return err
	}

	for _, d := range dir {
		os.RemoveAll(filepath.Join([]string{path, d.Name()}...))
		logger.Printf("[Engram] Local datashard data has been purged successfully\n")
	}

	return nil
}

// Get transaction data for any TXID from the daemon
func getTxData(txid string) (result rpc.GetTransaction_Result, err error) {
	if engram.Disk == nil || session.Offline {
		return
	}

	var params rpc.GetTransaction_Params

	params.Tx_Hashes = append(params.Tx_Hashes, txid)

	rpc_client.WS, _, err = websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
	if err != nil {
		return
	}

	input_output := rwc.New(rpc_client.WS)
	rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)

	if err = rpc_client.RPC.CallResult(context.Background(), "DERO.GetTransaction", params, &result); err != nil {
		logger.Errorf("[Engram] getTxData TXID: %s (Failed: %s)\n", txid, err)
		return
	}

	rpc_client.WS.Close()
	rpc_client.RPC.Close()

	if result.Status != "OK" {
		logger.Errorf("[Engram] getTxData TXID: %s (Failed: %s)\n", txid, result.Status)
		return
	}

	if len(result.Txs_as_hex[0]) < 50 {
		return
	}

	return
}

// Methods Engram will use as XSWD noStore
func engramNoStoreMethods() []string {
	return []string{
		"Subscribe",
		"SignData",
		"CheckSignature",
		"GetDaemon",
		"GetPrimaryUsername",
		"query_key",
		"QueryKey",
		"HandleTELALinks"}
}

// Check if method is in Engram's noStore list
func engramCanStoreMethod(method string) bool {
	noStoreMethods := engramNoStoreMethods()
	for _, m := range noStoreMethods {
		if m == method {
			return false
		}
	}

	return true
}

// Set XSWD permissions to the local Graviton tree
func setPermissions() {
	// Save permissions with dual storage
	data, err := json.Marshal(&cyberdeck.WS.global.permissions)
	if err != nil {
		logger.Errorf("[Engram] setPermissions: %s\n", err)
	} else {
		// Try encrypted storage first (when wallet available)
		if engram.Disk != nil {
			err = StoreEncryptedValue("XSWD", []byte("Globals"), data)
			if err != nil {
				logger.Debugf("[Engram] setPermissions (encrypted): %s\n", err)
			} else {
				logger.Printf("[Engram] Permissions saved to encrypted storage: %d bytes", len(data))
			}
		}

		// Always save to unencrypted storage as fallback
		err = StoreValue("XSWDUnencrypted", []byte("Globals"), data)
		if err != nil {
			logger.Debugf("[Engram] setPermissions (fallback): %s\n", err)
		} else {
			logger.Printf("[Engram] Permissions saved to fallback storage: %d bytes", len(data))
		}
	}

	// Force immediate save to ensure data is written
	logger.Printf("[Engram] setPermissions completed - permissions saved to dual storage")

	// Save enabled state separately with dual storage
	enabledValue := "0"
	if cyberdeck.WS.global.enabled {
		enabledValue = "1"
	}

	// Try encrypted storage first (when wallet available)
	if engram.Disk != nil {
		err = StoreEncryptedValue("XSWD", []byte("Enabled"), []byte(enabledValue))
		if err != nil {
			logger.Debugf("[Engram] setPermissions enabled (encrypted): %s\n", err)
		} else {
			logger.Printf("[Engram] WebSocket enabled state saved to encrypted storage: %s\n", enabledValue)
		}
	}

	// Always save to unencrypted storage as fallback
	err = StoreValue("XSWDUnencrypted", []byte("Enabled"), []byte(enabledValue))
	if err != nil {
		logger.Debugf("[Engram] setPermissions enabled (fallback): %s\n", err)
	} else {
		logger.Printf("[Engram] WebSocket enabled state saved to fallback storage: %s\n", enabledValue)
	}
}

// GetPermissionGroup returns the group a method belongs to
func GetPermissionGroup(method string) *PermissionGroup {
	for i := range permissionGroups {
		for _, m := range permissionGroups[i].Methods {
			if m == method {
				return &permissionGroups[i]
			}
		}
	}
	return nil
}

// getSimpleDefault returns the default permission for a category in Simple Mode
func getSimpleDefault(category string) xswd.Permission {
	switch category {
	case "readonly", "utility":
		return xswd.AlwaysAllow
	case "mining":
		return xswd.AlwaysDeny
	default:
		return xswd.Ask
	}
}

// IsSimpleMode checks if user is in simple mode
func IsSimpleMode() bool {
	// Skip encrypted storage if wallet not available yet
	if engram.Disk != nil {
		data, err := GetEncryptedValue("XSWD", []byte("SimpleMode"))
		if err == nil {
			return string(data) == "true"
		}
	}

	// Check unencrypted fallback
	data, err := GetValue("XSWDUnencrypted", []byte("SimpleMode"))
	if err != nil {
		return true // Default to simple mode
	}
	return string(data) == "true"
}

// SetSimpleMode toggles between simple and advanced mode
func SetSimpleMode(simple bool) {
	val := "false"
	if simple {
		val = "true"
	}

	// Try encrypted storage first
	if engram.Disk != nil {
		err := StoreEncryptedValue("XSWD", []byte("SimpleMode"), []byte(val))
		if err != nil {
			logger.Debugf("[Engram] SetSimpleMode (encrypted): %s\n", err)
		}
	}

	// Always save to unencrypted as fallback
	err := StoreValue("XSWDUnencrypted", []byte("SimpleMode"), []byte(val))
	if err != nil {
		logger.Debugf("[Engram] SetSimpleMode (fallback): %s\n", err)
	}
}

// GetGroupPermission returns the current permission for a group from storage
func GetGroupPermission(groupName string) xswd.Permission {
	// Skip encrypted storage if wallet not available yet
	if engram.Disk != nil {
		data, err := GetEncryptedValue("XSWD", []byte("Group:"+groupName))
		if err == nil {
			switch string(data) {
			case "AlwaysAllow":
				return xswd.AlwaysAllow
			case "AlwaysDeny":
				return xswd.AlwaysDeny
			default:
				return xswd.Ask
			}
		}
	}

	// Check unencrypted fallback
	data, err := GetValue("XSWDUnencrypted", []byte("Group:"+groupName))
	if err != nil {
		// Return default based on group category
		for _, group := range permissionGroups {
			if group.Name == groupName {
				return getSimpleDefault(group.Category)
			}
		}
		return xswd.Ask
	}

	switch string(data) {
	case "AlwaysAllow":
		return xswd.AlwaysAllow
	case "AlwaysDeny":
		return xswd.AlwaysDeny
	default:
		return xswd.Ask
	}
}

// SetGroupPermission sets permission for all methods in a group
func SetGroupPermission(groupName string, perm xswd.Permission) {
	permStr := perm.String()

	// Store group-level permission
	if engram.Disk != nil {
		err := StoreEncryptedValue("XSWD", []byte("Group:"+groupName), []byte(permStr))
		if err != nil {
			logger.Debugf("[Engram] SetGroupPermission (encrypted): %s\n", err)
		}
	}

	err := StoreValue("XSWDUnencrypted", []byte("Group:"+groupName), []byte(permStr))
	if err != nil {
		logger.Debugf("[Engram] SetGroupPermission (fallback): %s\n", err)
	}

	// Apply to all methods in the group
	for _, group := range permissionGroups {
		if group.Name == groupName {
			for _, method := range group.Methods {
				if cyberdeck.WS.global.permissions != nil {
					cyberdeck.WS.global.permissions[method] = perm
				}
			}
			break
		}
	}

	// Save updated permissions
	setPermissions()
	logger.Printf("[Engram] Set group '%s' permission to %s", groupName, permStr)
}

// ApplySimpleModeDefaults applies group permissions to individual methods
func ApplySimpleModeDefaults() {
	if !IsSimpleMode() {
		return
	}

	for _, group := range permissionGroups {
		groupPerm := GetGroupPermission(group.Name)
		for _, method := range group.Methods {
			if cyberdeck.WS.global.permissions != nil {
				cyberdeck.WS.global.permissions[method] = groupPerm
			}
		}
	}

	logger.Printf("[Engram] Applied Simple Mode defaults to %d permission groups", len(permissionGroups))
}

// Set all noStore methods to XSWD Ask permission
func SetDefaultPermissions() (defaults map[string]xswd.Permission) {
	defaults = make(map[string]xswd.Permission)
	for method := range rpcserver.WalletHandler {
		defaults[method] = xswd.Ask
	}

	// XSWD methods
	defaults["Subscribe"] = xswd.Ask
	defaults["HasMethod"] = xswd.Ask
	defaults["Unsubscribe"] = xswd.Ask
	defaults["GetDaemon"] = xswd.Ask
	defaults["SignData"] = xswd.Ask
	defaults["CheckSignature"] = xswd.Ask

	// Engram methods
	defaults["GetPrimaryUsername"] = xswd.Ask
	defaults["HandleTELALinks"] = xswd.Ask

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

	// Add specific methods that might be missing from your list
	defaults["Echo"] = xswd.Ask
	// Explicitly add methods that were missing from your list
	defaults["AttemptEPOCH"] = xswd.Ask
	defaults["CheckSignature"] = xswd.Ask
	defaults["GetAddress"] = xswd.Ask
	defaults["GetAddressEPOCH"] = xswd.Ask
	defaults["GetBalance"] = xswd.Ask
	defaults["GetDaemon"] = xswd.Ask
	defaults["GetHeight"] = xswd.Ask
	defaults["GetMaxHashesEPOCH"] = xswd.Ask
	defaults["GetPrimaryUsername"] = xswd.Ask
	defaults["GetSessionEPOCH"] = xswd.Ask
	defaults["GetTransferbyTXID"] = xswd.Ask
	defaults["GetTransfers"] = xswd.Ask
	defaults["MakeIntegratedAddress"] = xswd.Ask
	defaults["SplitIntegratedAddress"] = xswd.Ask
	defaults["SubmitEPOCH"] = xswd.Ask
	defaults["Transfer"] = xswd.Ask
	defaults["get_transfer_by_txid"] = xswd.Ask
	defaults["get_transfers"] = xswd.Ask
	defaults["getaddress"] = xswd.Ask
	defaults["getbalance"] = xswd.Ask
	defaults["getheight"] = xswd.Ask
	defaults["make_integrated_address"] = xswd.Ask
	defaults["query_key"] = xswd.Ask
	defaults["split_integrated_address"] = xswd.Ask
	defaults["transfer_split"] = xswd.Ask
	defaults["scinvoke"] = xswd.Ask

	// Debug: Print a few key methods to verify they're in the defaults
	logger.Printf("[Engram] DEBUG - Key methods in defaults: AttemptEPOCH=%v, GetAddress=%v, Transfer=%v",
		defaults["AttemptEPOCH"], defaults["GetAddress"], defaults["Transfer"])

	// Apply Simple Mode grouped defaults if enabled
	if IsSimpleMode() {
		logger.Printf("[Engram] Applying Simple Mode permission defaults")
		for _, group := range permissionGroups {
			groupPerm := GetGroupPermission(group.Name)
			// If no stored permission, use category default
			if groupPerm == xswd.Ask {
				groupPerm = getSimpleDefault(group.Category)
			}
			for _, method := range group.Methods {
				defaults[method] = groupPerm
			}
		}
		logger.Printf("[Engram] Applied Simple Mode defaults to %d permission groups", len(permissionGroups))
	}

	return
}

// Get XSWD permissions from local Graviton tree and sorted wallet methods
func getPermissions() (handler map[string]xswd.Permission, methods []string) {
	cyberdeck.WS.Lock()
	defer cyberdeck.WS.Unlock()

	logger.Printf("[Engram] getPermissions() called - wallet available: %v", engram.Disk != nil)
	cyberdeck.WS.global.permissions = SetDefaultPermissions()
	logger.Printf("[Engram] SetDefaultPermissions created %d methods", len(cyberdeck.WS.global.permissions))

	// Debug: Print all methods that should be available
	logger.Printf("[Engram] DEBUG - All methods in cyberdeck.WS.global.permissions:")
	for methodName := range cyberdeck.WS.global.permissions {
		logger.Printf("[Engram]   - %s", methodName)
	}

	// Load permissions (only if wallet is available for encrypted storage)
	if engram.Disk != nil {
		stored, err := GetEncryptedValue("XSWD", []byte("Globals"))
		if err != nil {
			logger.Printf("[Engram] getPermissions: stored permissions not found: %s (using defaults)\n", err)
		} else {
			// Load stored permissions into a temporary map
			var storedPermissions map[string]xswd.Permission
			if err := json.Unmarshal(stored, &storedPermissions); err != nil {
				logger.Errorf("[Engram] getPermissions: JSON unmarshal error: %s (corrupted data, deleting and using defaults)\n", err)
				// Delete corrupted data so it will be recreated on next save
				DeleteKey("XSWD", []byte("Globals"))
			} else {
				logger.Printf("[Engram] getPermissions: Successfully loaded %d stored permissions\n", len(storedPermissions))
				// Merge stored permissions with defaults (stored takes precedence)
				for method, permission := range storedPermissions {
					if _, exists := cyberdeck.WS.global.permissions[method]; exists {
						cyberdeck.WS.global.permissions[method] = permission
						logger.Printf("[Engram] getPermissions: Merged stored permission for %s: %s", method, permission)
					}
				}
				logger.Printf("[Engram] getPermissions: After merge, total permissions: %d", len(cyberdeck.WS.global.permissions))
			}
		}

		// Load enabled state
		storedEnabled, err := GetEncryptedValue("XSWD", []byte("Enabled"))
		if err != nil {
			logger.Printf("[Engram] WebSocket enabled state NOT FOUND (error: %v), defaulting to false", err)
			cyberdeck.WS.global.enabled = false // Default to disabled
		} else {
			enabledStr := string(storedEnabled)
			cyberdeck.WS.global.enabled = enabledStr == "1"
			logger.Printf("[Engram] WebSocket enabled state loaded: '%s' -> %v", enabledStr, cyberdeck.WS.global.enabled)
		}
	} else {
		// Try to load permissions from unencrypted fallback storage
		stored, err := GetValue("XSWDUnencrypted", []byte("Globals"))
		if err != nil {
			logger.Printf("[Engram] getPermissions: fallback permissions not found: %s (using defaults)\n", err)
		} else {
			// Load fallback permissions into a temporary map
			var storedPermissions map[string]xswd.Permission
			if err := json.Unmarshal(stored, &storedPermissions); err != nil {
				logger.Errorf("[Engram] getPermissions: fallback JSON unmarshal error: %s (corrupted data, deleting and using defaults)\n", err)
				// Delete corrupted data so it will be recreated on next save
				DeleteKey("XSWDUnencrypted", []byte("Globals"))
			} else {
				logger.Printf("[Engram] getPermissions: Successfully loaded %d fallback permissions\n", len(storedPermissions))
				// Merge fallback permissions with defaults (stored takes precedence)
				for method, permission := range storedPermissions {
					if _, exists := cyberdeck.WS.global.permissions[method]; exists {
						cyberdeck.WS.global.permissions[method] = permission
						logger.Printf("[Engram] getPermissions: Merged fallback permission for %s: %s", method, permission)
					}
				}
				logger.Printf("[Engram] getPermissions: After fallback merge, total permissions: %d", len(cyberdeck.WS.global.permissions))
			}
		}

		// Try to load enabled state from unencrypted fallback storage
		storedEnabled, err := GetValue("XSWDUnencrypted", []byte("Enabled"))
		if err != nil {
			logger.Printf("[Engram] WebSocket enabled state NOT FOUND in fallback (error: %v), defaulting to false", err)
			cyberdeck.WS.global.enabled = false // Default to disabled
		} else {
			enabledStr := string(storedEnabled)
			cyberdeck.WS.global.enabled = enabledStr == "1"
			logger.Printf("[Engram] WebSocket enabled state loaded from fallback: '%s' -> %v", enabledStr, cyberdeck.WS.global.enabled)
		}
		logger.Printf("[Engram] getPermissions: Wallet not available, using fallback permissions and enabled state")
	}

	for k := range cyberdeck.WS.global.permissions {
		methods = append(methods, k)
		logger.Printf("[Engram] Found method in permissions: %s", k)
	}

	sort.Strings(methods)
	logger.Printf("[Engram] Total methods for UI: %d", len(methods))

	return cyberdeck.WS.global.permissions, methods
}

// Start a permissioned web socket server to allow decentralized application communication
func toggleXSWD(endpoint string) {
	if engram.Disk == nil {
		return
	}

	if cyberdeck.WS.server != nil {
		cyberdeck.WS.server.Stop()
		cyberdeck.WS.server = nil
		cyberdeck.WS.status.Text = "Blocked"
		cyberdeck.WS.status.Color = colors.Gray
		cyberdeck.WS.status.Refresh()
		cyberdeck.WS.toggle.Text = "Turn On"
		cyberdeck.WS.toggle.Refresh()
		status.Cyberdeck.FillColor = colors.Gray
		status.Cyberdeck.StrokeColor = colors.Gray
		status.Cyberdeck.Refresh()
		cyberdeck.WS.advanced = false
		cyberdeck.WS.global.enabled = false
		cyberdeck.WS.global.connect = false

		// Save WebSocket disabled state to storage
		setPermissions()
		cyberdeck.WS.apps = []xswd.ApplicationData{}
		if cyberdeck.WS.list != nil {
			cyberdeck.WS.list.Refresh()
		}
		logger.Printf("[Engram] XSWD server closed\n")
	} else {
		_, portNum, err := net.SplitHostPort(endpoint)
		if err != nil {
			logger.Errorf("[Engram] Invalid XSWD server endpoint: %s\n", err)
			return
		}

		portInt, err := strconv.Atoi(portNum)
		if err != nil {
			logger.Errorf("[Engram] Invalid XSWD server port: %s\n", err)
			return
		}

		logger.Printf("[Engram] Starting XSWD server %s\n", endpoint)

		noStoreMethods := engramNoStoreMethods()

		cyberdeck.WS.server = xswd.NewXSWDServerWithPort(portInt, engram.Disk, false, noStoreMethods, func(ad *xswd.ApplicationData) bool {
			return XSWDPrompt(ad)
		}, func(ad *xswd.ApplicationData, r *jrpc2.Request) xswd.Permission {
			return AskPermissionForRequest(ad, r)
		})

		// Only update UI if it exists (may be nil during auto-start at login)
		if cyberdeck.WS.toggle != nil {
			cyberdeck.WS.toggle.Disable()
			cyberdeck.WS.toggle.Text = "Initializing"
			cyberdeck.WS.toggle.Refresh()
		}
		time.Sleep(time.Second)
		if !cyberdeck.WS.server.IsRunning() {
			cyberdeck.WS.server = nil
			logger.Errorf("[Engram] Error starting XSWD server\n")
			if cyberdeck.WS.toggle != nil {
				cyberdeck.WS.toggle.Text = "Error starting web sockets"
				cyberdeck.WS.toggle.Refresh()
				go func() {
					time.Sleep(time.Second * 2)
					fyne.Do(func() {
						fyne.Do(func() {
							cyberdeck.WS.toggle.Text = "Turn On"
							cyberdeck.WS.toggle.Refresh()
							cyberdeck.WS.toggle.Enable()
						})
					})
				}()
			}
			return
		}
		if cyberdeck.WS.toggle != nil {
			cyberdeck.WS.toggle.Enable()
		}

		if cyberdeck.WS.server == nil {
			if cyberdeck.WS.status != nil {
				cyberdeck.WS.status.Text = "Blocked"
				cyberdeck.WS.status.Color = colors.Gray
				cyberdeck.WS.status.Refresh()
			}
			if cyberdeck.WS.toggle != nil {
				cyberdeck.WS.toggle.Text = "Turn On"
				cyberdeck.WS.toggle.Refresh()
			}
			if status.Cyberdeck != nil {
				status.Cyberdeck.FillColor = colors.Gray
				status.Cyberdeck.StrokeColor = colors.Gray
				status.Cyberdeck.Refresh()
			}
		} else {
			for method, h := range EngramHandler {
				cyberdeck.WS.server.SetCustomMethod(method, h)
			}

			cyberdeck.WS.server.SetCustomMethod("HandleTELALinks", handler.New(HandleTELALinks))

			cyberdeck.WS.server.SetCustomMethod("AttemptEPOCHWithAddr", handler.New(AttemptEPOCHWithAddr))

			for method, h := range epoch.GetHandler() {
				cyberdeck.WS.server.SetCustomMethod(method, h)
			}

			if cyberdeck.WS.status != nil {
				cyberdeck.WS.status.Text = "Allowed"
				cyberdeck.WS.status.Color = colors.Green
				cyberdeck.WS.status.Refresh()
			}
			if cyberdeck.WS.toggle != nil {
				cyberdeck.WS.toggle.Text = "Turn Off"
				cyberdeck.WS.toggle.Refresh()
			}
			if status.Cyberdeck != nil {
				status.Cyberdeck.FillColor = colors.Green
				status.Cyberdeck.StrokeColor = colors.Green
				status.Cyberdeck.Refresh()
			}

			// CRITICAL FIX: Save WebSocket enabled state to storage
			cyberdeck.WS.global.enabled = true
			cyberdeck.WS.advanced = true
			setPermissions()
			logger.Printf("[Engram] WebSocket enabled state saved to storage")
		}
	}
}

// Prompt when an application submits request to connect to wallet with XSWD
func XSWDPrompt(ad *xswd.ApplicationData) (confirmed bool) {
	if cyberdeck.WS.advanced {
		// If global permissions enabled set them here
		if cyberdeck.WS.global.enabled {
			logger.Printf("[Engram] Applied global XSWD permissions to %s\n", ad.Name)
			// Initialize ad.Permissions if nil to avoid panic
			if ad.Permissions == nil {
				ad.Permissions = make(map[string]xswd.Permission)
			}
			cyberdeck.WS.RLock()
			for k, v := range cyberdeck.WS.global.permissions {
				ad.Permissions[k] = v
			}
			cyberdeck.WS.RUnlock()
		}

		// If wallet is set to connect to all requests, connect to app
		if cyberdeck.WS.global.connect {
			logger.Printf("[Engram] Applied automatic XSWD connection to %s\n", ad.Name)
			fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: "A new connection request has been approved"})
			go refreshXSWDList()
			return true
		}
	} else {
		// Restrictive mode overwrites any requested permissions to default Ask, and sets certain methods to AlwaysDeny
		ad.Permissions = map[string]xswd.Permission{}
		ad.Permissions["QueryKey"] = xswd.AlwaysDeny
		ad.Permissions["query_key"] = xswd.AlwaysDeny
	}

	overlay := session.Window.Canvas().Overlays()

	headerText := "NEW  CONNECTION  REQUEST"

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText("APP  NAME", colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### " + ad.Name)
	textApp.Wrapping = fyne.TextWrapWord

	labelID := canvas.NewText("APP  ID", colors.Gray)
	labelID.TextSize = 14
	labelID.Alignment = fyne.TextAlignLeading
	labelID.TextStyle = fyne.TextStyle{Bold: true}

	textID := widget.NewRichTextFromMarkdown(ad.Id)
	textID.Wrapping = fyne.TextWrapWord

	labelURL := canvas.NewText("URL", colors.Gray)
	labelURL.TextSize = 14
	labelURL.Alignment = fyne.TextAlignLeading
	labelURL.TextStyle = fyne.TextStyle{Bold: true}

	textURL := widget.NewRichTextFromMarkdown(ad.Url)
	textURL.Wrapping = fyne.TextWrapWord

	labelPermissions := canvas.NewText("PERMISSIONS", colors.Gray)
	labelPermissions.TextSize = 14
	labelPermissions.Alignment = fyne.TextAlignLeading
	labelPermissions.TextStyle = fyne.TextStyle{Bold: true}

	// Get permissioned methods from xswd.ApplicationData and create permission objects
	var methods []string
	for k := range ad.Permissions {
		methods = append(methods, k)
	}

	sort.Strings(methods)

	permForm := container.NewVBox()

	textSpacer := canvas.NewRectangle(color.Transparent)
	textSpacer.SetMinSize(fyne.NewSize(10, 3))

	for _, k := range methods {
		perm := ad.Permissions[k]
		permColor := colors.Account
		switch perm {
		case xswd.AlwaysAllow:
			permColor = colors.Green
		case xswd.AlwaysDeny:
			permColor = colors.Red
		}

		textMethod := widget.NewRichTextFromMarkdown("### " + k)
		textMethod.Wrapping = fyne.TextWrapWord

		sep := canvas.NewRectangle(colors.Gray)
		sep.SetMinSize(fyne.NewSize(ui.Width*0.5, 2))

		add := container.NewVBox(
			textMethod,
			container.NewHBox(
				textSpacer,
				canvas.NewText(perm.String(), permColor),
			),
			textSpacer,
			container.NewHBox(
				sep,
			),
		)

		permForm.Add(add)
	}

	if len(permForm.Objects) == 0 {
		permForm.Add(
			container.NewVBox(
				widget.NewRichTextFromMarkdown("No permissions"),
			),
		)
	} else {
		permForm.Add(textSpacer)
	}

	labelEvents := canvas.NewText("EVENTS", colors.Gray)
	labelEvents.TextSize = 14
	labelEvents.Alignment = fyne.TextAlignLeading
	labelEvents.TextStyle = fyne.TextStyle{Bold: true}

	eventsForm := container.NewVBox()

	// Get registered events from xswd.ApplicationData and create event objects
	for name, b := range ad.RegisteredEvents {
		eventColor := colors.Red
		if b {
			eventColor = colors.Green
		}

		textEvent := widget.NewRichTextFromMarkdown(fmt.Sprintf("### %s", name))
		textEvent.Wrapping = fyne.TextWrapWord

		sep := canvas.NewRectangle(colors.Gray)
		sep.SetMinSize(fyne.NewSize(ui.Width*0.5, 2))

		add := container.NewVBox(
			textEvent,
			container.NewHBox(
				textSpacer,
				canvas.NewText(strconv.FormatBool(b), eventColor),
			),
			textSpacer,
			container.NewHBox(
				sep,
			),
		)

		eventsForm.Add(add)
	}

	if len(eventsForm.Objects) == 0 {
		eventsForm.Add(
			container.NewVBox(
				widget.NewRichTextFromMarkdown("No events"),
			),
		)
	}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	options := widget.NewSelect([]string{xswd.Allow.String(), xswd.Deny.String()}, nil)

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				options,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						rectSpacer,
						rectSpacer,
						labelApp,
						textApp,
						rectSpacer,
						labelID,
						textID,
						rectSpacer,
						labelURL,
						textURL,
						rectSpacer,
						labelPermissions,
						permForm,
						rectSpacer,
						labelEvents,
						eventsForm,
						rectSpacer,
					),
				),
			),
		),
	)

	// Create and show connection prompt
	done := make(chan struct{})
	btnDismiss := widget.NewButton("Deny", nil)
	btnDismiss.OnTapped = func() {
		if options.Selected == xswd.Allow.String() {
			confirmed = true
		}
		done <- struct{}{}
	}

	options.OnChanged = func(s string) {
		if s == xswd.Deny.String() {
			btnDismiss.Importance = widget.MediumImportance
		} else {
			btnDismiss.Importance = widget.HighImportance
		}
		btnDismiss.SetText(s)
		btnDismiss.Refresh()
	}

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

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
					container.NewCenter(
						container.NewStack(
							span,
						),
					),
					rectSpacer,
					rectSpacer,
					content,
					btnDismiss,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
				),
			),
		),
	)

	if a.Driver().Device().IsMobile() {
		fyne.Do(func() {
			fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: "A new connection request has been received"})
		})
	} else {
		fyne.Do(func() {
			session.Window.RequestFocus()
		})
	}

	// Wait for user input or socket close
	select {
	case <-done:

	case <-ad.OnClose:

	}

	overlay.Top().Hide()
	overlay.Remove(overlay.Top())
	overlay.Remove(overlay.Top())

	go refreshXSWDList()

	return
}

// Handle incoming TELA link requests and return params to be displayed in approval prompt
func handleTELALinkRequest(linkParams TELALink_Params) (params string, err error) {
	var args []string
	var target string
	target, args, err = tela.ParseTELALink(linkParams.TelaLink)
	if err != nil {
		return
	}

	switch target {
	case "tela":
		switch args[0] {
		case "open": // open TELA content similar to a hyperlink
			if len(args) < 2 || len(args[1]) != 64 {
				err = fmt.Errorf("/open/ request has invalid scid argument")
				return
			}

			// Engram will check content rating and show it in prompt
			var rating tela.Rating_Result
			rating, err = tela.GetRating(args[1], session.Daemon, 0)
			if err != nil {
				return
			}

			var index tela.INDEX
			index, err = tela.GetINDEXInfo(args[1], session.Daemon)
			if err != nil {
				return
			}

			var linkDisplay TELALink_Display
			linkDisplay.Name = index.NameHdr
			linkDisplay.Descr = index.DescrHdr
			linkDisplay.DURL = index.DURL
			linkDisplay.TelaLink = linkParams.TelaLink
			rating.Ratings = nil // don't need to show each individual rating in prompt
			linkDisplay.Rating = &rating

			params = fmt.Sprintf("%+v", linkDisplay)
			if indentParams, err := json.MarshalIndent(linkDisplay, "", " "); err == nil {
				params = string(indentParams)
			}
		default:
			err = fmt.Errorf("invalid argument: %s", args[0])
			return
		}
	case "engram":
		if len(args) < 3 {
			err = fmt.Errorf("invalid engram link format")
			return
		}

		switch args[0] {
		case "asset":
			switch args[1] {
			case "manager": // open asset manager module with scid data
				if len(args[2]) != 64 {
					err = fmt.Errorf("/manager/ request has invalid scid argument")
					return
				}
			default:
				err = fmt.Errorf("invalid argument: %s", args[1])
				return
			}
		default:
			err = fmt.Errorf("invalid argument: %s", args[0])
			return
		}

		params = fmt.Sprintf("%+v", linkParams)
		if indentParams, err := json.MarshalIndent(linkParams, "", " "); err == nil {
			params = string(indentParams) // indent params if able
		}
	default:
		err = fmt.Errorf("invalid target: %s", target)
		return
	}

	return
}

// Ask permission to complete a specific request from a connected application,
// can choose to Allow, Always Allow, Deny, Always Deny the request
func AskPermissionForRequest(ad *xswd.ApplicationData, request *jrpc2.Request) (choice xswd.Permission) {
	method := request.Method()
	// Gnomon methods behave as AlwaysAllow
	if strings.HasPrefix(method, "Gnomon.") {
		return xswd.Allow
	}

	// All other methods require approval
	choice = xswd.Deny

	// EPOCH is not online or permissioned so Deny request
	if strings.HasSuffix(method, "EPOCH") && !epoch.IsActive() {
		return
	}

	overlay := session.Window.Canvas().Overlays()

	headerText := "NEW  PERMISSION  REQUEST"

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText("FROM", colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### " + ad.Name)
	textApp.Wrapping = fyne.TextWrapWord

	labelRequest := canvas.NewText("REQUESTING", colors.Gray)
	labelRequest.TextSize = 14
	labelRequest.Alignment = fyne.TextAlignLeading
	labelRequest.TextStyle = fyne.TextStyle{Bold: true}

	textRequest := widget.NewRichTextFromMarkdown("### " + method)
	textRequest.Wrapping = fyne.TextWrapWord

	labelParams := canvas.NewText("PARAMETERS", colors.Gray)
	labelParams.TextSize = 14
	labelParams.Alignment = fyne.TextAlignLeading
	labelParams.TextStyle = fyne.TextStyle{Bold: true}

	params := "None"
	if method == "HandleTELALinks" {
		var linkParams TELALink_Params
		err := request.UnmarshalParams(&linkParams)
		if err != nil {
			logger.Errorf("[Engram] Denied TELA link request %s from %s: %s\n", request.ParamString(), ad.Name, err)
			return
		}

		params, err = handleTELALinkRequest(linkParams)
		if err != nil {
			logger.Errorf("[Engram] Denied TELA link request %q from %s: %s\n", linkParams.TelaLink, ad.Name, err)
			return
		}
	} else if request.ParamString() != "" {
		params = strings.ReplaceAll(strings.Join(strings.Fields(request.ParamString()), " "), "\n", " ")

		// Unmarshall and indent params if able
		var buffer interface{}
		if request.UnmarshalParams(&buffer) == nil {
			if indentParams, err := json.MarshalIndent(buffer, "", " "); err == nil {
				params = string(indentParams)
			}
		}
	}

	textParams := widget.NewLabel(params)
	textParams.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	permissions := []string{
		xswd.Allow.String(),
		xswd.Deny.String(),
	}

	// Add AlwaysAllow option if method is !noStore
	if cyberdeck.WS.server.CanStorePermission(method) {
		permissions = append(permissions, xswd.AlwaysAllow.String())
	}

	permissions = append(permissions, xswd.AlwaysDeny.String())

	options := widget.NewSelect(permissions, nil)

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				options,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						labelApp,
						textApp,
						rectSpacer,
						labelRequest,
						textRequest,
						rectSpacer,
						labelParams,
						textParams,
					),
				),
			),
		),
	)

	// Create and show request prompt
	done := make(chan struct{})
	btnDismiss := widget.NewButton("Deny", nil)
	btnDismiss.OnTapped = func() {
		switch options.Selected {
		case xswd.Allow.String():
			choice = xswd.Allow
		case xswd.Deny.String():
			choice = xswd.Deny
		case xswd.AlwaysAllow.String():
			choice = xswd.AlwaysAllow
		case xswd.AlwaysDeny.String():
			choice = xswd.AlwaysDeny
		}
		done <- struct{}{}
	}

	options.OnChanged = func(s string) {
		switch s {
		case xswd.Allow.String(), xswd.AlwaysAllow.String():
			btnDismiss.Importance = widget.HighImportance
		case xswd.Deny.String(), xswd.AlwaysDeny.String():
			btnDismiss.Importance = widget.MediumImportance
		}
		btnDismiss.SetText(s)
		btnDismiss.Refresh()
	}

	linkRemove := widget.NewHyperlinkWithStyle("Remove Application", nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkRemove.OnTapped = func() {
		verificationOverlay(
			false,
			ad.Name,
			"Remove this application?",
			"Remove",
			func(b bool) {
				if b {
					cyberdeck.WS.server.RemoveApplication(ad)
				}
			},
		)
	}

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

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
					container.NewCenter(
						container.NewStack(
							span,
						),
					),
					rectSpacer,
					rectSpacer,
					content,
					btnDismiss,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					container.NewHBox(
						layout.NewSpacer(),
						linkRemove,
						layout.NewSpacer(),
					),
					rectSpacer,
				),
			),
		),
	)

	if a.Driver().Device().IsMobile() {
		fyne.CurrentApp().SendNotification(&fyne.Notification{Title: ad.Name, Content: "A new permission request has been received"})
	} else {
		session.Window.RequestFocus()
	}

	// Wait for user input or socket close
	select {
	case <-done:

	case <-ad.OnClose:

	}

	overlay.Top().Hide()
	overlay.Remove(overlay.Top())
	overlay.Remove(overlay.Top())

	go refreshXSWDList()

	return choice
}

// Refresh list of connected XSWD apps
func refreshXSWDList() {
	time.Sleep(time.Second)
	if cyberdeck.WS.server != nil {
		cyberdeck.WS.apps = cyberdeck.WS.server.GetApplications()
		sort.Slice(cyberdeck.WS.apps, func(i, j int) bool { return cyberdeck.WS.apps[i].Name < cyberdeck.WS.apps[j].Name })
		if cyberdeck.WS.list != nil {
			fyne.Do(func() {
				cyberdeck.WS.list.UnselectAll()
				cyberdeck.WS.list.FocusLost()
				cyberdeck.WS.list.Refresh()
			})
		}
	}
}

// Ask permission to complete a specific Engram action, using xswd permissions to match existing requests that have params to display
func AskPermissionForRequestE(headerText string, params interface{}) (choice xswd.Permission, err error) {
	choice = xswd.Deny

	var paramString string

	switch p := params.(type) {
	case TELALink_Params:
		paramString, err = handleTELALinkRequest(p)
		if err != nil {
			err = fmt.Errorf("denied TELA link request %s: %s", p.TelaLink, err)
			return
		}
	case string:
		switch p {
		case "TELA R OFF":
			paramString = "You will be viewing all TELA content as per your TELA settings.\n\n"
			paramString += "Min Likes will omit results if they are below the set likes ratio.\n\n"
			paramString += "Search exclusions will omit results that include the set exclusion text in their dURL."
		default:
			err = fmt.Errorf("unknown Engram request param string: %s", p)
			return
		}
	default:
		err = fmt.Errorf("unknown Engram request params: %T", p)
		return
	}

	overlay := session.Window.Canvas().Overlays()

	header := canvas.NewText(headerText, colors.Gray)
	header.TextSize = 16
	header.Alignment = fyne.TextAlignCenter
	header.TextStyle = fyne.TextStyle{Bold: true}

	labelApp := canvas.NewText("FROM", colors.Gray)
	labelApp.TextSize = 14
	labelApp.Alignment = fyne.TextAlignLeading
	labelApp.TextStyle = fyne.TextStyle{Bold: true}

	textApp := widget.NewRichTextFromMarkdown("### Engram")
	textApp.Wrapping = fyne.TextWrapWord

	labelParams := canvas.NewText("PARAMETERS", colors.Gray)
	labelParams.TextSize = 14
	labelParams.Alignment = fyne.TextAlignLeading
	labelParams.TextStyle = fyne.TextStyle{Bold: true}

	textParams := widget.NewLabel(paramString)
	textParams.Wrapping = fyne.TextWrapWord

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.90, ui.MaxHeight*0.48))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(fyne.NewSize(0, 10))

	permissions := []string{
		xswd.Allow.String(),
		xswd.Deny.String(),
	}

	options := widget.NewSelect(permissions, nil)

	content := container.NewStack(
		container.NewBorder(
			nil,
			container.NewVBox(
				rectSpacer,
				rectSpacer,
				options,
				rectSpacer,
				rectSpacer,
			),
			nil,
			nil,
			container.NewStack(
				rectBox,
				container.NewVScroll(
					container.NewVBox(
						labelApp,
						textApp,
						rectSpacer,
						labelParams,
						textParams,
						rectSpacer,
					),
				),
			),
		),
	)

	// Create and show request prompt
	done := make(chan struct{})
	btnDismiss := widget.NewButton("Deny", nil)
	btnDismiss.OnTapped = func() {
		switch options.Selected {
		case xswd.Allow.String():
			choice = xswd.Allow
		default:
			choice = xswd.Deny
		}
		done <- struct{}{}
	}

	options.OnChanged = func(s string) {
		switch s {
		case xswd.Allow.String():
			btnDismiss.Importance = widget.HighImportance
		default:
			btnDismiss.Importance = widget.MediumImportance
		}
		btnDismiss.SetText(s)
		btnDismiss.Refresh()
	}

	span := canvas.NewRectangle(color.Transparent)
	span.SetMinSize(fyne.NewSize(ui.Width, 10))

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
					container.NewCenter(
						container.NewStack(
							span,
						),
					),
					rectSpacer,
					rectSpacer,
					rectSpacer,
					content,
					btnDismiss,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
					rectSpacer,
				),
			),
		),
	)

	// Wait for user input
	<-done

	overlay.Top().Hide()
	overlay.Remove(overlay.Top())
	overlay.Remove(overlay.Top())

	return
}

func isASCII(s string) bool {
	for _, c := range s {
		if c > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// Wrapper for serving TELA content toggling tela.updates if disabled, updated content should be checked for and presented to the user before calling serveTELAUpdates
func serveTELAUpdates(scid string) (link string, err error) {
	var toggledUpdates bool
	if !tela.UpdatesAllowed() {
		tela.AllowUpdates(true)
		toggledUpdates = true
	}

	link, err = tela.ServeTELA(scid, session.Daemon)
	if toggledUpdates {
		tela.AllowUpdates(false)
	}

	return
}

// Convert TELA error to shortened string for display
func telaErrorToString(err error) string {
	str := "serving TELA"
	if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
		str = "content has been updated"
	} else if strings.Contains(err.Error(), "already exists") {
		str = "content already exists"
	}

	return fmt.Sprintf("%s %s", "error", str)
}

// Get the ratio of likes for a TELA SCID, if ratio < minLines an error will be returned
func getLikesRatio(scid, dURL, searchExclusions string, minLikes float64) (ratio float64, ratings tela.Rating_Result, err error) {
	if gnomon.Index == nil {
		err = fmt.Errorf("gnomon is not online")
		return
	}

	err = telaFilterSearchExclusions(dURL, searchExclusions)
	if err != nil {
		return
	}

	_, up := gnomon.GetSCIDValuesByKey(scid, "likes")
	if up == nil {
		err = fmt.Errorf("could not get %s likes", scid)
		return
	}

	_, down := gnomon.GetSCIDValuesByKey(scid, "dislikes")
	if down == nil {
		err = fmt.Errorf("could not get %s dislikes", scid)
		return
	}

	ratings.Likes = up[0]
	ratings.Dislikes = down[0]

	total := float64(up[0] + down[0])
	if total == 0 {
		ratio = 50
	} else {
		ratio = (float64(up[0]) / total) * 100
	}

	if ratio < minLikes {
		err = fmt.Errorf("%s is below min rating setting", scid)
	}

	return
}

// Check if a search exclusion is found in a TELA dURL
func telaFilterSearchExclusions(dURL, searchExclusions string) (err error) {
	for _, split := range strings.Split(searchExclusions, ",") {
		exclude := strings.TrimSpace(split)
		if exclude != "" && strings.Contains(dURL, exclude) {
			err = fmt.Errorf("found search exclusion %q in dURL %s", exclude, dURL)
			return
		}
	}

	return
}

// Sort and return search display strings for list widget
func telaSearchDisplayAll(telaSearch []INDEXwithRatings, sortBy string) (display []string) {
	switch sortBy {
	case "Z-A":
		sort.Slice(telaSearch, func(i, j int) bool {
			return telaSearch[i].NameHdr > telaSearch[j].NameHdr
		})
	case "A-Z":
		sort.Slice(telaSearch, func(i, j int) bool {
			return telaSearch[i].NameHdr < telaSearch[j].NameHdr
		})
	default: // Ratings
		sort.Slice(telaSearch, func(i, j int) bool {
			if telaSearch[i].ratings.Likes != telaSearch[j].ratings.Likes {
				return telaSearch[i].ratings.Likes > telaSearch[j].ratings.Likes
			}

			return telaSearch[i].ratings.Dislikes < telaSearch[j].ratings.Dislikes
		})
	}

	for _, ind := range telaSearch {
		display = append(display, ind.NameHdr+";;;"+ind.SCID)
	}

	return
}

// Validate the URL as URI or SC image and return it as a canvas.Image
func handleImageURL(nameHdr, imageURL string, size fyne.Size) (image *canvas.Image, err error) {
	scImage, err := tela.ValidateImageURL(imageURL, session.Daemon)
	if err != nil {
		return
	}

	var resource fyne.Resource
	image = canvas.NewImageFromResource(nil)

	if scImage != "" {
		resource = fyne.NewStaticResource(nameHdr, []byte(scImage))
	} else {
		resource, err = fyne.LoadResourceFromURLString(imageURL)
		if err != nil {
			return
		}
	}

	image.Resource = resource
	image.SetMinSize(size)
	image.FillMode = canvas.ImageFillContain
	image.Refresh()

	return
}

// Convert session.Domain to string for display
func sessionDomainToString(domain string) string {
	str := strings.TrimPrefix(domain, "app.")
	switch str {
	// case "main":
	// case "create":
	// case "restore":
	// case "settings":
	case "wallet":
		return "Dashboard"
	// case "register":
	case "explorer":
		return "Asset Explorer"
	case "manager":
		return "Asset Manager"
	case "send", "transfers", "messages", "cyberdeck", "Identity", "datapad":
		return fmt.Sprintf("%s%s", strings.ToUpper(str[0:1]), str[1:])
	case "tela", "tela.manager":
		return "TELA"
	case "service":
		return "Services"
	case "sign", "verify":
		return "File Manager"
	case "messages.contact":
		return "Message Contact"
	case "cyberdeck.manager":
		return "Cyberdeck Manager"
	case "cyberdeck.permissions":
		return "Cyberdeck Settings"
	case "sc.builder":
		return "Contract Builder"
	case "sc.editor":
		return "Contract Editor"
	default:
		return ""
	}
}

// Add EPOCH session values to the account total stores
func storeEPOCHTotal(timeout time.Duration) {
	epochSession, err := epoch.GetSession(timeout)
	if err == nil {
		cyberdeck.EPOCH.total.Hashes += epochSession.Hashes
		cyberdeck.EPOCH.total.MiniBlocks += epochSession.MiniBlocks

		var eMar []byte
		if eMar, err = json.Marshal(cyberdeck.EPOCH.total); err == nil {
			err = StoreEncryptedValue("Cyberdeck", []byte("EPOCH"), eMar)
		}
	}

	if err != nil {
		logger.Errorf("[EPOCH] Storing total: %s\n", err)
	}
}

// Store account EPOCH session and stop EPOCH
func stopEPOCH() {
	if cyberdeck.EPOCH.enabled {
		storeEPOCHTotal(time.Second * 4)
	}

	epoch.StopGetWork()
	cyberdeck.EPOCH.enabled = false
	//cyberdeck.EPOCH.allowWithAddress = false
}

// Check if value exists within a string array/slice
func scidExist(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// Recovery form constants
const (
	MaxAccountNameLength = 25
	HexKeyLength         = 64
	SeedWordCount24      = 24
	SeedWordCount25      = 25
	MaxDisplayFileLen    = 50
)

// Database configuration
const (
	DefaultDBTimeout = 5 * time.Second
	MaxDBRetries     = 3
)

// showFormError displays an error message on a canvas.Text element
func showFormError(text *canvas.Text, msg string) {
	text.Text = msg
	text.Color = colors.Red
	text.Refresh()
}

// showFormSuccess displays a success message on a canvas.Text element
func showFormSuccess(text *canvas.Text, msg string) {
	text.Text = msg
	text.Color = colors.Green
	text.Refresh()
}

// clearFormText clears a canvas.Text element
func clearFormText(text *canvas.Text) {
	text.Text = ""
	text.Refresh()
}

// validateRecoveryForm checks if the recovery form has valid account name and matching passwords
func validateRecoveryForm(name, password, passwordConfirm string) bool {
	return len(password) > 0 && password == passwordConfirm && name != ""
}

// PasswordStrength represents the strength level of a password
type PasswordStrength int

const (
	PasswordWeak PasswordStrength = iota
	PasswordFair
	PasswordGood
	PasswordStrong
)

// getPasswordStrength evaluates password strength based on length and character variety
func getPasswordStrength(password string) PasswordStrength {
	if len(password) == 0 {
		return PasswordWeak
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSpecial = true
		}
	}

	score := 0
	if len(password) >= 8 {
		score++
	}
	if len(password) >= 12 {
		score++
	}
	if hasUpper && hasLower {
		score++
	}
	if hasDigit {
		score++
	}
	if hasSpecial {
		score++
	}

	switch {
	case score >= 4:
		return PasswordStrong
	case score >= 3:
		return PasswordGood
	case score >= 2:
		return PasswordFair
	default:
		return PasswordWeak
	}
}

// getPasswordStrengthText returns a human-readable description of password strength
func getPasswordStrengthText(strength PasswordStrength) string {
	switch strength {
	case PasswordStrong:
		return "Strong"
	case PasswordGood:
		return "Good"
	case PasswordFair:
		return "Fair"
	default:
		return "Weak"
	}
}

// getPasswordStrengthColor returns the color associated with password strength
func getPasswordStrengthColor(strength PasswordStrength) color.Color {
	switch strength {
	case PasswordStrong:
		return colors.Green
	case PasswordGood:
		return colors.Green
	case PasswordFair:
		return colors.Yellow
	default:
		return colors.Red
	}
}

// Debouncer provides debounced function execution
type Debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	duration time.Duration
}

// NewDebouncer creates a new Debouncer with the specified duration
func NewDebouncer(duration time.Duration) *Debouncer {
	return &Debouncer{duration: duration}
}

// Debounce executes the function after the debounce duration, canceling any pending execution
func (d *Debouncer) Debounce(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.duration, fn)
}
