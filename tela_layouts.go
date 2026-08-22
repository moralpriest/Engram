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
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DEROFDN/engram/i18n"
	apptheme "github.com/DEROFDN/engram/internal/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	x "fyne.io/x/fyne/widget"
	"github.com/civilware/tela"
	"github.com/civilware/tela/logger"
	"github.com/creachadair/jrpc2"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi/xswd"
	"github.com/deroproject/graviton"
	"github.com/hypergnomon/hypergnomon/pkg/gnomes/structures"
)

func layoutTELA() fyne.CanvasObject {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[TELA-LAYOUT] PANIC recovered in layoutTELA(): %v\n", r)
			session.Domain = "app.wallet"
			if session.Window != nil {
				session.Window.SetContent(layoutDashboard())
			}
		}
	}()

	logger.Printf("[TELA-LAYOUT] layoutTELA() starting...\n")
	session.Domain = "app.tela"

	var history []string
	var historyData binding.StringList
	var historyList *widget.List

	var searching []string
	var searchData binding.StringList
	var searchList *widget.List

	var serving []string
	var servingData binding.StringList
	var servingList *widget.List

	var favorites []string
	var favoritesData binding.StringList
	var favoritesList *widget.List
	var refreshFavoritesList func()
	var refreshAppsList func()
	var refreshServerList func()
	var scheduleTelaWarmup func()
	var maybeStartTelaWork func(bool)
	var startTelaInitialLoad func()
	var resetTelaProgress func()
	var hasTelaCache func() bool
	var telaWarmupScheduled atomic.Bool
	var telaWorkActive atomic.Bool
	var telaLaunchPending atomic.Bool
	var activeRowUpdaters sync.Map   // fyne.CanvasObject -> scid
	var activeRatingFetches sync.Map // scid -> bool
	var telaNetworkPaused atomic.Bool

	frame := &iframe{}
	rectLeft := canvas.NewRectangle(color.Transparent)
	rectLeft.SetMinSize(fyne.NewSize(ui.Width*0.40, scaleSize(35)))

	rectRight := canvas.NewRectangle(color.Transparent)
	rectRight.SetMinSize(fyne.NewSize(ui.Width*0.58, scaleSize(35)))

	rectList := canvas.NewRectangle(color.Transparent)
	rectList.SetMinSize(fyne.NewSize(ui.Width, 100))

	rectWidth := canvas.NewRectangle(color.Transparent)
	rectWidth.SetMinSize(fyne.NewSize(ui.Width, scaleSize(10)))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	isMobileLayout := ui.Width <= 360
	if isMobileLayout {
		rectSpacer.SetMinSize(fyne.NewSize(scaleSize(6), scaleSize(2)))
	}

	heading := canvas.NewText(i18n.T("tela.browser_header"), apptheme.C.Gray)
	heading.TextSize = scaleFont(16)
	heading.Alignment = fyne.TextAlignCenter
	heading.TextStyle = fyne.TextStyle{Bold: true}

	results := canvas.NewText("", apptheme.StatusTextColor())
	results.TextSize = scaleFont(13)

	telaStatus := canvas.NewText("", color.Transparent)
	telaStatus.TextSize = scaleFont(12)

	telaProgress := NewSlimProgressBar()
	telaProgress.Hide()

	labelLastScan := canvas.NewText("", apptheme.C.Green)
	labelLastScan.TextSize = scaleFont(13)

	errorText := canvas.NewText("", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter
	errorText.Hide()

	statusBox := container.NewVBox()
	refreshTelaStatusBox := func() {
		objs := []fyne.CanvasObject{}
		if !errorText.Hidden && strings.TrimSpace(errorText.Text) != "" {
			objs = append(objs, errorText)
		}
		if !results.Hidden && strings.TrimSpace(results.Text) != "" {
			objs = append(objs, results)
		}
		if !telaStatus.Hidden && strings.TrimSpace(telaStatus.Text) != "" {
			objs = append(objs, telaStatus)
		}
		if !telaProgress.Hidden {
			objs = append(objs, telaProgress)
		}
		statusBox.Objects = objs
		statusBox.Refresh()
	}

	var telaSearch []INDEXwithRatings
	var searchMu sync.RWMutex
	var sortBy string
	var sortDescending bool = true
	searchData = binding.BindStringList(&searching)
	var refreshTimer *time.Timer
	var refreshMu sync.Mutex
	refreshSearch := func() {
		refreshMu.Lock()
		defer refreshMu.Unlock()
		if refreshTimer != nil {
			refreshTimer.Stop()
		}
		refreshTimer = time.AfterFunc(1500*time.Millisecond, func() {
			fyne.Do(func() {
				if sortBy != "Ratings" {
					return
				}
				searchMu.Lock()
				newList := telaSearchDisplayAll(telaSearch, sortBy, sortDescending)

				// Compare with current 'searching' list to see if anything actually moved
				if len(newList) == len(searching) {
					changed := false
					for i := range newList {
						if newList[i] != searching[i] {
							changed = true
							break
						}
					}
					if !changed {
						searchMu.Unlock()
						return
					}
				}

				searching = newList
				searchMu.Unlock()
				searchData.Set(searching)
				if searchList != nil {
					searchList.Refresh()
				}
			})
		})
	}

	findTelaSearchEntry := func(scid string) (INDEXwithRatings, bool) {
		searchMu.RLock()
		defer searchMu.RUnlock()
		for _, entry := range telaSearch {
			if entry.SCID == scid {
				return entry, true
			}
		}
		return INDEXwithRatings{}, false
	}

	updateTelaSearchEntry := func(scid string, update func(*INDEXwithRatings)) {
		searchMu.Lock()
		updated := false
		for i := range telaSearch {
			if telaSearch[i].SCID == scid {
				update(&telaSearch[i])
				updated = true
				break
			}
		}
		searchMu.Unlock()
		if updated {
			refreshSearch()
		}
	}

	warmRatings := func() {
		searchMu.RLock()
		if len(telaSearch) == 0 {
			searchMu.RUnlock()
			return
		}
		var missing []string
		for _, entry := range telaSearch {
			if entry.ratings.Average == 0 {
				missing = append(missing, entry.SCID)
			}
		}
		searchMu.RUnlock()

		if len(missing) == 0 {
			return
		}

		go func() {
			// Process in batches of 50 to maximize RPC efficiency
			for i := 0; i < len(missing); i += 50 {
				end := i + 50
				if end > len(missing) {
					end = len(missing)
				}
				batch := missing[i:end]

				// Use background context for async warmup
				_, ratingsMap, _, err := batchFetchINDEXes(context.Background(), batch, 50)
				if err != nil {
					logger.Printf("[TELA] warmRatings batch fetch error: %v\n", err)
					continue
				}

				for scid, ratings := range ratingsMap {
					if ratings.Average > 0 || ratings.Likes > 0 || ratings.Dislikes > 0 {
						updateTelaSearchEntry(scid, func(e *INDEXwithRatings) {
							e.ratings = ratings
						})
					}
				}
			}
		}()
	}

	setTelaStatus := func(text string, clr color.Color) {
		if text == "" || clr == color.Transparent {
			telaStatus.Text = ""
			telaStatus.Color = clr
			fyne.Do(func() {
				telaStatus.Hide()
				telaStatus.Refresh()
				refreshTelaStatusBox()
			})
			return
		}
		if telaStatus.Text == text && telaStatus.Color == clr {
			return
		}
		telaStatus.Text = text
		telaStatus.Color = clr
		fyne.Do(func() {
			telaStatus.Show()
			telaStatus.Refresh()
			refreshTelaStatusBox()
		})
	}

	setResultsText := func(format string, a ...any) {
		s := fmt.Sprintf(format, a...)
		const maxStatusLen = 50
		if len(s) > maxStatusLen {
			s = s[:maxStatusLen-3] + "..."
		}
		fyne.Do(func() {
			results.Text = s
		})
	}

	setTelaProgress := func(value float64) {
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}
		fyne.Do(func() {
			if telaProgress.Hidden {
				telaProgress.Show()
			}
			telaProgress.SetValue(value)
			refreshTelaStatusBox()
		})
	}

	var displayedTelaProgress float64

	showInfiniteTelaProgress := func() {
		fyne.Do(func() {
			if telaProgress.Hidden {
				telaProgress.Show()
			}
			next := displayedTelaProgress + 0.04
			if next < 0.12 {
				next = 0.12
			}
			if next > 0.72 {
				next = 0.72
			}
			displayedTelaProgress = next
			telaProgress.SetValue(next)
			refreshTelaStatusBox()
		})
	}

	updateTelaProgress := func(value float64) {
		if value < displayedTelaProgress {
			value = displayedTelaProgress
		}
		if value > 0.99 {
			value = 0.99
		}
		displayedTelaProgress = value
		setTelaProgress(value)
	}

	showActiveTelaProgress := func(status string, value float64, initial bool) {
		telaViewActive.Store(true)
		if initial {
			results.Hide()
			telaStatus.Text = status
			telaStatus.Color = apptheme.StatusTextColor()
			telaStatus.Refresh()
			if telaProgress.Hidden {
				telaProgress.Show()
			}
			telaProgress.SetValue(value)
			refreshTelaStatusBox()
			return
		}
		fyne.Do(func() {
			results.Hide()
			refreshTelaStatusBox()
		})
		setTelaStatus(status, apptheme.StatusTextColor())
		updateTelaProgress(value)
	}

	resetTelaProgress = func() { displayedTelaProgress = 0 }

	completeTelaScanProgress := func() {
		displayedTelaProgress = 1
		setTelaProgress(1)
	}

	hideTelaProgress := func() {
		fyne.Do(func() {
			telaProgress.Hide()
			refreshTelaStatusBox()
		})
	}

	newTelaListItem := func() fyne.CanvasObject {
		heartBtn := widget.NewButtonWithIcon("", favsOutlineResource(), nil)
		heartBtn.Importance = widget.LowImportance

		activeBg := canvas.NewRectangle(color.Transparent)
		activeBg.SetMinSize(fyne.NewSize(0, scaleSize(39)))

		nameLabel := widget.NewLabel("")
		nameLabel.Alignment = fyne.TextAlignLeading
		nameLabel.Truncation = fyne.TextTruncateEllipsis
		nameLabel.Wrapping = fyne.TextWrapOff

		startCloseBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), nil)
		startCloseBtn.Importance = widget.LowImportance

		launchProgress := NewSlimProgressBar()
		launchProgress.SetBarMinSize(fyne.NewSize(0, scaleSize(10)))
		launchProgress.Hide()

		launchStatus := canvas.NewText("", apptheme.StatusTextColor())
		launchStatus.TextSize = scaleFont(10)
		launchStatus.Alignment = fyne.TextAlignCenter
		launchStatus.Hide()

		ratingLabel := canvas.NewText("0.0", apptheme.C.Account)
		ratingLabel.TextSize = scaleFont(10)
		ratingLabel.TextStyle = fyne.TextStyle{Bold: true}

		ratingHex := canvas.NewImageFromResource(resourceTelaHexagonGray)
		ratingHex.SetMinSize(fyne.NewSize(scaleSize(24), scaleSize(28)))
		ratingHex.FillMode = canvas.ImageFillContain

		ratingContainer := container.NewStack(
			ratingHex,
			container.NewCenter(ratingLabel),
		)

		bottomSpacer := canvas.NewRectangle(color.Transparent)
		bottomSpacer.SetMinSize(fyne.NewSize(0, 1))

		appIcon := canvas.NewImageFromResource(resourceTelaIcon)
		appIcon.SetMinSize(fyne.NewSize(scaleSize(26), scaleSize(26)))
		appIcon.FillMode = canvas.ImageFillContain

		topRow := container.NewBorder(
			nil, bottomSpacer,
			container.NewHBox(appIcon, heartBtn, ratingContainer),
			container.NewCenter(container.NewGridWrap(fyne.NewSize(scaleSize(38), scaleSize(38)), startCloseBtn)),
			container.NewPadded(nameLabel),
		)

		row := container.NewStack(
			activeBg,
			container.NewBorder(
				topRow,
				container.NewVBox(
					launchStatus,
					launchProgress,
				),
				nil, nil, nil,
			),
		)
		return row
	}

	updateTelaFavoriteButton := func(btn *widget.Button, scid string) {
		if engram.Disk == nil {
			btn.SetIcon(favsOutlineMutedResource())
			btn.Disable()
			btn.OnTapped = nil
			return
		}

		walletAddress := engram.Disk.GetAddress().String()
		if IsTELAFavorite(walletAddress, scid) {
			btn.SetIcon(favsResource())
		} else {
			btn.SetIcon(favsOutlineResource())
		}
		btn.Enable()
	}

	toggleTelaFavorite := func(scid string) {
		if engram.Disk == nil {
			errorText.Text = i18n.T("tela.no_wallet")
			errorText.Color = apptheme.C.Gray
			errorText.Refresh()
			return
		}

		walletAddress := engram.Disk.GetAddress().String()
		if IsTELAFavorite(walletAddress, scid) {
			if err := RemoveTELAFavorite(walletAddress, scid); err != nil {
				errorText.Text = i18n.T("tela.error_rm_favorite")
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			errorText.Text = i18n.T("tela.removed_favorites")
			errorText.Color = apptheme.C.Green
		} else {
			entry, ok := findTelaSearchEntry(scid)
			if !ok {
				errorText.Text = i18n.T("tela.error_no_metadata")
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			if err := AddTELAFavorite(walletAddress, scid, entry.NameHdr, entry.DescrHdr, entry.IconHdr, entry.ratings.Average); err != nil {
				errorText.Text = i18n.T("tela.error_add_favorite")
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			errorText.Text = i18n.T("tela.added_favorites")
			errorText.Color = apptheme.C.Green
		}

		errorText.Refresh()
		if refreshFavoritesList != nil {
			refreshFavoritesList()
		}
		searchList.Refresh()
		favoritesList.Refresh()
	}

	configureTelaListRow := func(raw string, co fyne.CanvasObject) {
		row, ok := co.(*fyne.Container)
		if !ok || len(row.Objects) < 2 {
			return
		}

		activeBg, ok := row.Objects[0].(*canvas.Rectangle)
		if !ok {
			return
		}

		var heartBtn *widget.Button
		var nameLabel *widget.Label
		var startCloseBtn *widget.Button
		var launchProgress *slimProgressBar
		var launchStatus *canvas.Text
		var appIcon *canvas.Image
		var ratingLabel *canvas.Text
		var ratingHex *canvas.Image

		var walk func(fyne.CanvasObject)
		walk = func(obj fyne.CanvasObject) {
			switch v := obj.(type) {
			case *widget.Button:
				if heartBtn == nil {
					heartBtn = v
				} else if startCloseBtn == nil {
					startCloseBtn = v
				}
			case *widget.Label:
				if nameLabel == nil {
					nameLabel = v
				}
			case *slimProgressBar:
				launchProgress = v
			case *canvas.Text:
				if v.Color == apptheme.StatusTextColor() {
					launchStatus = v
				} else if v.Color == apptheme.C.Account {
					ratingLabel = v
				}
			case *canvas.Image:
				if appIcon == nil {
					appIcon = v
				} else if ratingHex == nil {
					ratingHex = v
				}
			case *fyne.Container:
				for _, child := range v.Objects {
					walk(child)
				}
			}
		}

		walk(row.Objects[1])
		if heartBtn == nil || nameLabel == nil || startCloseBtn == nil {
			return
		}

		name, scid := parseTelaListEntry(raw)
		nameLabel.SetText(name)

		if appIcon != nil {
			appIcon.Resource = resourceTelaIcon
			if entry, ok := findTelaSearchEntry(scid); ok && entry.IconHdr != "" {
				go func(currentSCID, iconURL, nameHdr string, imgObj *canvas.Image) {
					if img, err := handleImageURL(nameHdr, iconURL, fyne.NewSize(scaleSize(26), scaleSize(26))); err == nil {
						uiDo(func() {
							_, checkSCID := parseTelaListEntry(raw)
							if checkSCID == currentSCID {
								imgObj.Resource = img.Resource
								imgObj.Refresh()
							}
						})
					}
				}(scid, entry.IconHdr, entry.NameHdr, appIcon)
			}
			appIcon.Refresh()
		}

		if ratingLabel != nil && ratingHex != nil {
			ratingLabel.Text = "0.0"
			ratingHex.Resource = resourceTelaHexagonGray

			if entry, ok := findTelaSearchEntry(scid); ok {
				if entry.ratings.Average > 0 {
					ratingLabel.Text = fmt.Sprintf("%.1f", entry.ratings.Average)
					ratingHex.Resource = telaHexagonColor(entry.ratings)
				} else {
					go func(currentSCID string, label *canvas.Text, hex *canvas.Image) {
						if _, loading := activeRatingFetches.LoadOrStore(currentSCID, true); loading {
							return
						}
						defer activeRatingFetches.Delete(currentSCID)

						ratings, err := tela.GetRating(currentSCID, session.Daemon, 0)
						if err == nil && (ratings.Average > 0 || ratings.Dislikes > ratings.Likes) {
							uiDo(func() {
								_, checkSCID := parseTelaListEntry(raw)
								if checkSCID == currentSCID {
									label.Text = fmt.Sprintf("%.1f", ratings.Average)
									hex.Resource = telaHexagonColor(ratings)
									label.Refresh()
									hex.Refresh()
								}
								updateTelaSearchEntry(currentSCID, func(e *INDEXwithRatings) {
									e.ratings = ratings
								})
							})
						}
					}(scid, ratingLabel, ratingHex)
				}
			}
			ratingLabel.Refresh()
			ratingHex.Refresh()
		}

		telaLaunchingSCIDsGlobal.Lock()
		isLaunching := telaLaunchingSCIDsGlobal.m[scid]
		telaLaunchingSCIDsGlobal.Unlock()

		telaStoppingSCIDsGlobal.Lock()
		isStopping := telaStoppingSCIDsGlobal.m[scid]
		telaStoppingSCIDsGlobal.Unlock()

		if isLaunching {
			if launchProgress != nil {
				launchProgress.Show()
			}
			if launchStatus != nil {
				if isStopping {
					launchStatus.Text = i18n.T("tela.status_stopping")
				} else {
					launchStatus.Text = i18n.T("tela.status_starting")
				}
				launchStatus.Show()
			}
			startCloseBtn.SetText("")
			startCloseBtn.SetIcon(theme.CancelIcon())
			startCloseBtn.Enable()

			// Sync UI with existing launch progress
			if _, loaded := activeRowUpdaters.LoadOrStore(co, scid); !loaded {
				go func(targetRow fyne.CanvasObject, rowSCID string) {
					defer activeRowUpdaters.Delete(targetRow)

					telaLaunchStartTimesGlobal.Lock()
					startTime, ok := telaLaunchStartTimesGlobal.m[rowSCID]
					telaLaunchStartTimesGlobal.Unlock()
					if !ok {
						return
					}

					const cap = 0.95
					const tau = 10.0
					for {
						// Check if this row is still assigned to the same SCID
						if current, ok := activeRowUpdaters.Load(targetRow); !ok || current != rowSCID {
							return
						}

						telaLaunchingSCIDsGlobal.Lock()
						stillLaunching := telaLaunchingSCIDsGlobal.m[rowSCID]
						telaLaunchingSCIDsGlobal.Unlock()
						if !stillLaunching {
							return
						}

						elapsed := time.Since(startTime).Seconds()
						val := cap * (1.0 - math.Exp(-elapsed/tau))
						if val > cap {
							val = cap
						}

						uiDo(func() {
							if launchProgress != nil && !launchProgress.Hidden {
								launchProgress.SetValue(val)
							}
							if launchStatus != nil && !launchStatus.Hidden {
								telaStoppingSCIDsGlobal.Lock()
								isStopping := telaStoppingSCIDsGlobal.m[rowSCID]
								telaStoppingSCIDsGlobal.Unlock()
								if isStopping {
									launchStatus.Text = i18n.T("tela.status_stopping")
								} else {
									if val < 0.30 {
										launchStatus.Text = i18n.T("tela.status_connecting_node")
									} else if val < 0.60 {
										launchStatus.Text = i18n.T("tela.status_fetching")
									} else if val < 0.85 {
										launchStatus.Text = i18n.T("tela.status_preparing")
									} else {
										launchStatus.Text = i18n.T("tela.status_almost_ready")
									}
								}
							}
						})
						time.Sleep(200 * time.Millisecond)
					}
				}(co, scid)
			}
		} else if isTelaActive(scid) {
			if launchProgress != nil {
				launchProgress.Hide()
			}
			if launchStatus != nil {
				launchStatus.Hide()
			}
			activeBg.FillColor = color.NRGBA{R: 20, G: 120, B: 70, A: 48}
			startCloseBtn.SetText("")
			startCloseBtn.SetIcon(theme.MediaStopIcon())
			startCloseBtn.Enable()
		} else {
			if launchProgress != nil {
				launchProgress.Hide()
			}
			if launchStatus != nil {
				launchStatus.Hide()
			}
			activeBg.FillColor = color.Transparent
			startCloseBtn.SetText("")
			startCloseBtn.SetIcon(theme.MediaPlayIcon())
			startCloseBtn.Enable()
		}
		activeBg.Refresh()
		updateTelaFavoriteButton(heartBtn, scid)
		heartBtn.OnTapped = func() {
			toggleTelaFavorite(scid)
		}
		startCloseBtn.OnTapped = func() {
			telaLaunchingSCIDsGlobal.Lock()
			isLaunching := telaLaunchingSCIDsGlobal.m[scid]
			telaLaunchingSCIDsGlobal.Unlock()

			if isLaunching {
				telaStoppingSCIDsGlobal.Lock()
				telaStoppingSCIDsGlobal.m[scid] = true
				telaStoppingSCIDsGlobal.Unlock()

				telaLaunchCancelChansGlobal.Lock()
				if cancelChan, ok := telaLaunchCancelChansGlobal.m[scid]; ok {
					close(cancelChan)
					delete(telaLaunchCancelChansGlobal.m, scid)
				}
				telaLaunchCancelChansGlobal.Unlock()
				if launchStatus != nil {
					launchStatus.Text = i18n.T("tela.status_stopping")
					launchStatus.Refresh()
				}
				startCloseBtn.SetIcon(theme.ContentCutIcon())
				startCloseBtn.Refresh()
			} else if isTelaActive(scid) {
				entry, ok := findTelaSearchEntry(scid)
				if ok {
					go func() {
						tela.ShutdownServer(entry.DURL)
						if refreshServerList != nil {
							refreshServerList()
						}
						uiDo(func() {
							searchList.Refresh()
							favoritesList.Refresh()
						})
					}()
				}
			} else {
				if engram.Disk == nil {
					errorText.Text = i18n.T("tela.no_wallet")
					errorText.Color = apptheme.C.Gray
					errorText.Refresh()
					return
				}

				telaLaunchingSCIDsGlobal.Lock()
				if telaLaunchingSCIDsGlobal.m[scid] {
					telaLaunchingSCIDsGlobal.Unlock()
					return
				}
				telaLaunchingSCIDsGlobal.m[scid] = true
				telaLaunchingSCIDsGlobal.Unlock()

				cancelChan := make(chan struct{})
				telaLaunchCancelChansGlobal.Lock()
				telaLaunchCancelChansGlobal.m[scid] = cancelChan
				telaLaunchCancelChansGlobal.Unlock()

				telaLaunchStartTimesGlobal.Lock()
				telaLaunchStartTimesGlobal.m[scid] = time.Now()
				telaLaunchStartTimesGlobal.Unlock()

				if launchStatus != nil {
					launchStatus.Text = i18n.T("tela.status_starting")
					launchStatus.Show()
				}
				activeBg.Refresh()
				startCloseBtn.SetText("")
				startCloseBtn.SetIcon(theme.CancelIcon())
				searchList.Refresh()
				favoritesList.Refresh()

				progressDone := make(chan struct{})
				var cancelled atomic.Bool
				// Progress updates are now handled by configureTelaListRow's sync goroutine
				// which is triggered by the searchesList.Refresh() below.

				cleanupLaunch := func(failed, cancelledLaunch bool) {
					close(progressDone)
					telaLaunchingSCIDsGlobal.Lock()
					delete(telaLaunchingSCIDsGlobal.m, scid)
					telaLaunchingSCIDsGlobal.Unlock()
					telaLaunchCancelChansGlobal.Lock()
					delete(telaLaunchCancelChansGlobal.m, scid)
					telaLaunchCancelChansGlobal.Unlock()
					telaStoppingSCIDsGlobal.Lock()
					delete(telaStoppingSCIDsGlobal.m, scid)
					telaStoppingSCIDsGlobal.Unlock()
					telaLaunchStartTimesGlobal.Lock()
					delete(telaLaunchStartTimesGlobal.m, scid)
					telaLaunchStartTimesGlobal.Unlock()
					uiDo(func() {
						if launchProgress != nil {
							if failed || cancelledLaunch {
								launchProgress.SetValue(launchProgress.value)
								launchProgress.Refresh()
							} else {
								launchProgress.SetValue(1.0)
								launchProgress.Refresh()
							}
						}
						if launchStatus != nil {
							if cancelledLaunch {
								launchStatus.Text = "Cancelled"
								launchStatus.Color = apptheme.C.Gray
							} else if failed {
								launchStatus.Text = "Launch Error"
								launchStatus.Color = apptheme.C.Red
							} else {
								launchStatus.Text = "Done!"
								launchStatus.Color = apptheme.C.Green
							}
							launchStatus.Refresh()
						}
						if launchProgress != nil {
							launchProgress.Hide()
						}
						if launchStatus != nil {
							launchStatus.Hide()
						}
						activeBg.SetMinSize(fyne.NewSize(0, scaleSize(40)))
						activeBg.Refresh()
					})

					if refreshServerList != nil {
						refreshServerList()
					}

					uiDo(func() {
						searchList.Refresh()
						favoritesList.Refresh()
					})
				}

				errorText.Text = ""
				errorText.Refresh()
				go func() {
					select {
					case <-progressDone:
						return
					case <-cancelChan:
						cancelled.Store(true)
						return
					}
				}()

				go func() {
					openURLAfterDelay := func(link string) {
						// Parallelize verify + EnsureXSWD so villager opens near-instantly on remote nodes.
						if verifyAndEnsureTELA(link, scid) {
							if u, err := url.Parse(link); err == nil {
								fyne.CurrentApp().OpenURL(u)
							}
						} else {
							logger.Errorf("[TELA] XSWD or server not ready for %s (scid %s)\n", link, scid)
							fyne.Do(func() {
								errorText.Text = i18n.T("tela.error_cannot_open")
								errorText.Color = apptheme.C.Red
								errorText.Refresh()
							})
						}
					}

					link, err := serveTELAWithStaleRecovery(scid, session.Daemon, &cancelled)
					if cancelled.Load() {
						if err == nil {
							tela.ShutdownServer(scid)
						}
						cleanupLaunch(false, true)
						return
					}

					if err == nil {
						pushTELANavigation(scid)

						// Auto-index any dependent SCIDs (e.g. song registries) AFTER serving,
						// so the TELA app files are cloned and can be scanned for hardcoded SCIDs.
						if gnomon.Index != nil {
							AutoIndexDependentSCIDs(scid)
						}

						go openURLAfterDelay(link)
						go func(s string) {
							if err := StoreEncryptedValue("TELA History", []byte(s), []byte("")); err != nil {
								logger.Errorf("[Engram] Error saving TELA app to history: %s\n", err)
							}
						}(scid)
						cleanupLaunch(false, false)
					} else {
						if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
							telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", scid)}
							linkPermission, permErr := AskPermissionForRequestE(i18n.T("tela.allow_updated_content"), telaLink)
							if permErr != nil {
								logger.Errorf("[Engram] Open TELA link: %s\n", permErr)
								fyne.Do(func() {
									errorText.Text = i18n.T("tela.error_cannot_open")
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							if linkPermission != xswd.Allow {
								cleanupLaunch(true, false)
								return
							}

							link, updateErr := serveTELAUpdates(scid)
							if updateErr != nil {
								logger.Errorf("[Engram] Error serving TELA: %s\n", updateErr)
								fyne.Do(func() {
									errorText.Text = telaErrorToString(updateErr)
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							pushTELANavigation(scid)
							go openURLAfterDelay(link)
							cleanupLaunch(false, false)
						} else {
							logger.Printf("[TELA] ServeTELA failed for SCID %s: %v", scid, err)
							fyne.Do(func() {
								errorText.Text = i18n.T("tela.error_starting")
								errorText.Color = apptheme.C.Red
								errorText.Refresh()
							})
							cleanupLaunch(true, false)
						}
					}
				}()
			}
		}
	}

	historyData = binding.BindStringList(&history)
	historyList = widget.NewListWithData(historyData,
		newTelaListItem,
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			configureTelaListRow(str, co)
		},
	)

	searchList = widget.NewListWithData(searchData,
		newTelaListItem,
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			configureTelaListRow(str, co)
		},
	)

	servingData = binding.BindStringList(&serving)
	servingList = widget.NewListWithData(servingData,
		func() fyne.CanvasObject {
			return container.NewStack(
				container.NewHBox(
					container.NewStack(
						rectLeft,
						widget.NewLabel(""),
					),
					container.NewStack(
						rectRight,
						widget.NewLabel(""),
					),
				),
			)
		},
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			split := strings.Split(str, ";;;")
			if len(split) < 2 {
				return
			}

			fyne.Do(func() {
				co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
				co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			})
		},
	)

	favoritesData = binding.BindStringList(&favorites)
	favoritesList = widget.NewListWithData(favoritesData,
		newTelaListItem,
		func(di binding.DataItem, co fyne.CanvasObject) {
			dat := di.(binding.String)
			str, err := dat.Get()
			if err != nil {
				return
			}

			configureTelaListRow(str, co)
		},
	)

	entryHistory := widget.NewEntry()
	entryHistory.PlaceHolder = "Search History"
	entryHistory.SetIcon(theme.SearchIcon())
	entryHistory.Disable()

	entryServeSCID := widget.NewEntry()
	entryServeSCID.PlaceHolder = "Serve by SCID"

	entryAddSCID := widget.NewEntry()
	entryAddSCID.PlaceHolder = "Add SCID"
	entryAddSCID.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		if len(s) == 64 {
			if gnomon.Index != nil {
				if gnomon.GetAllSCIDVariableDetails(s) != nil {
					errorText.Text = "scid already exists"
					errorText.Color = apptheme.StatusTextColor()
					errorText.Refresh()
					return
				}

				code, err := getContractCode(s)
				if err != nil || code == "" {
					errorText.Text = "could not get scid"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				err = gnomon.AddSCIDToIndex(s)
				if err != nil {
					errorText.Text = "error adding scid"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}

				entryAddSCID.SetText("")
				errorText.Text = "scid added"
				errorText.Color = apptheme.C.Green
				errorText.Refresh()
			}
		}
	}

	entrySearchCompletions := []string{"author:", "durl:", "name:", "my:"}
	entrySearch := x.NewCompletionEntry(entrySearchCompletions)
	entrySearch.PlaceHolder = i18n.T("tela.search_placeholder")
	entrySearch.SetIcon(theme.SearchIcon())
	entrySearch.Disable()

	sep := canvas.NewRectangle(apptheme.C.Gray)
	sep.SetMinSize(fyne.NewSize(ui.Width*0.2, 2))

	line1 := container.NewVBox(
		layout.NewSpacer(),
		sep,
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

	var isSearching bool

	btnBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		session.Domain = "app.wallet" // break any loops now
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutDashboard())
		removeOverlays()
	})

	btnSettingsTela := newSizedIconButton(theme.SettingsIcon(), func() {
		session.Domain = "app.tela.settings" // Mark as coming from TELA
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	})

	linkClearHistory := widget.NewHyperlinkWithStyle(i18n.T("settings.clear_history"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: false})
	linkClearHistory.OnTapped = func() {
		verificationOverlay(
			false,
			i18n.T("tela.browser_header"),
			i18n.T("settings.clear_history_prompt"),
			i18n.T("common.confirm"),
			func(b bool) {
				if b {
					if gnomon.Index == nil || session.Offline {
						return
					}

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

					session.Window.SetContent(layoutTransition())
					session.Window.SetContent(layoutTELA())
				}
			},
		)
	}

	wSelect := widget.NewSelect([]string{"Search", "Favorites", "History"}, nil)
	wSelect.SetSelectedIndex(0)

	btnRescanTela := newSizedIconButton(theme.ViewRefreshIcon(), func() {
		rescanLabel := widget.NewLabel(i18n.T("tela.rescan_prompt"))
		rescanLabel.Wrapping = fyne.TextWrapWord

		dlg := dialog.NewCustomWithoutButtons(i18n.T("tela.browser_header"), rescanLabel, session.Window)

		btnConfirm := widget.NewButtonWithIcon(i18n.T("tela.rescan"), theme.ViewRefreshIcon(), func() {
			dlg.Hide()
			clearAllTELACache()
			forceFreshScan = true
			searchMu.Lock()
			searching = []string{}
			telaSearch = []INDEXwithRatings{}
			searchMu.Unlock()
			searchData.Set(searching)

			generation := currentWalletGeneration()

			results.Text = i18n.T("tela.resetting_gnomon")
			results.Color = apptheme.StatusTextColor()
			uiDo(func() {
				if !isWalletGenerationActive(generation) {
					return
				}
				results.Refresh()
			})

			go func() {
				if err := resetGnomonIndex(); err != nil {
					logger.Errorf("[TELA] Could not reset gnomon index: %s\n", err)
					return
				}

				for i := 0; i < 60; i++ {
					if !isWalletGenerationActive(generation) {
						return
					}
					time.Sleep(time.Second)
					if gnomon.Index != nil {
						break
					}
				}

				uiDo(func() {
					if !isWalletGenerationActive(generation) {
						return
					}
					wSelect.SetSelected("Search")
				})
			}()
		})

		btnCancel := widget.NewButtonWithIcon(i18n.T("common.cancel"), theme.CancelIcon(), func() {
			dlg.Hide()
		})

		dlg.SetButtons([]fyne.CanvasObject{wrapMobileButton(btnConfirm), btnCancel})
		dlg.Show()
	})

	activateTelaSearch := func() {}

	sortByOptions := []string{"Ratings", "A-Z", "Recent"}
	if storedSortBy, found := getTELADual("Sort By"); found {
		if storedSortBy == "Z-A" {
			sortBy = "A-Z"
			sortDescending = true
			setTELADual("Sort By", []byte("A-Z"))
			setTELADual("Sort Order", []byte("Descending"))
		} else {
			sortBy = storedSortBy
		}
	} else {
		sortBy = sortByOptions[0]
	}

	if sortBy == "A-Z" {
		sortDescending = false
	} else {
		sortDescending = true
	}

	if storedSortOrder, found := getTELADual("Sort Order"); found {
		if storedSortOrder == "Ascending" {
			sortDescending = false
		} else if storedSortOrder == "Descending" {
			sortDescending = true
		}
	}

	btnSortOrder := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), nil)
	if !sortDescending {
		btnSortOrder.SetIcon(theme.MenuDropUpIcon())
	}
	btnSortOrder.Importance = widget.LowImportance
	btnSortOrder.OnTapped = func() {
		sortDescending = !sortDescending
		if sortDescending {
			btnSortOrder.SetIcon(theme.MenuDropDownIcon())
			setTELADual("Sort Order", []byte("Descending"))
		} else {
			btnSortOrder.SetIcon(theme.MenuDropUpIcon())
			setTELADual("Sort Order", []byte("Ascending"))
		}

		if wSelect.Selected == "Search" {
			searchMu.RLock()
			searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
			searchMu.RUnlock()
			_ = searchData.Set(searching)
			searchList.Refresh()
		}
	}

	btnTela := widget.NewButtonWithIcon(i18n.T("tela.tab_apps"), globeResource(), func() {
		if wSelect.Selected == "Search" {
			activateTelaSearch()
			return
		}
		wSelect.SetSelected("Search")
	})
	btnTela.Importance = widget.LowImportance

	favoritesLabel := i18n.T("tela.tab_favorites")

	btnFavorites := widget.NewButtonWithIcon(favoritesLabel, favsResource(), func() {
		wSelect.SetSelected("Favorites")
	})
	btnFavorites.Importance = widget.LowImportance

	btnHistory := widget.NewButtonWithIcon(i18n.T("tela.tab_history"), historyResource(), func() {
		wSelect.SetSelected("History")
	})
	btnHistory.Importance = widget.LowImportance

	// Horizontal button row (like dashboard)
	var tabButtons fyne.CanvasObject
	if isMobileDevice() {
		// Use HBox instead of Grid on mobile to allow wide buttons (with text) to fit correctly.
		// We use a narrower size enforcer for the sort button to save space.
		sortSize := canvas.NewRectangle(color.Transparent)
		sortSize.SetMinSize(scalePoint(40, 48))
		btnSortOrderMobile := container.NewStack(sortSize, btnSortOrder)

		tabButtons = container.NewHBox(
			btnSortOrderMobile,
			wrapMobileButton(btnTela),
			wrapMobileButton(btnFavorites),
			wrapMobileButton(btnHistory),
		)
	} else {
		tabButtons = container.NewHBox(
			btnSortOrder,
			btnTela,
			btnFavorites,
			btnHistory,
		)
	}

	btnShutdown := widget.NewButton("Shutdown TELA", nil)

	var restrictiveMode, rescanRecheck bool
	var lastScan, searchExclusions string
	var minLikes float64
	var telaSCIDs []string
	var sAll = map[string]bool{}
	// Initialize TELA settings from storage
	if storedMinLikes, found := getTELADual("Min Likes"); found {
		if f, err := strconv.ParseFloat(storedMinLikes, 64); err == nil {
			minLikes = f
		}
	} else {
		minLikes = 30
	}

	if storedExclusions, found := getTELADual("Exclusions"); found {
		searchExclusions = storedExclusions
	}

	if storedRescanRecheck, found := getTELADual("Rescan Recheck"); found {
		if storedRescanRecheck == "Yes" {
			rescanRecheck = true
		}
	}

	restrictiveMode = false // Default OFF (unrestrictive)
	// First check new "Restrictive Mode" key (set by Settings page)
	if restrictiveModeValue, found := getTELADual("Restrictive Mode"); found {
		if restrictiveModeValue == "true" {
			restrictiveMode = true
		}
	} else {
		// Fallback to legacy "Mode" key for backward compatibility
		if storedTelaMode, found := getTELADual("Mode"); found {
			if storedTelaMode == "Restrictive" {
				restrictiveMode = true
			}
		}
	}

	var getSearchResults func()
	hasTelaCache = func() bool {
		if raw, err := GetEncryptedValue("TELA Search", []byte("DisplayCache")); err == nil && len(raw) > 0 {
			var cachedDisplay telaDisplayCache
			if json.Unmarshal(raw, &cachedDisplay) == nil && len(cachedDisplay) > 0 {
				return true
			}
		}
		if raw, err := GetEncryptedValue("TELA Search", []byte("SCIDs")); err == nil && len(raw) > 0 {
			var cachedSCIDs []string
			if json.Unmarshal(raw, &cachedSCIDs) == nil && len(cachedSCIDs) > 0 {
				return true
			}
		}
		if raw, err := GetEncryptedValue("TELA Search", []byte("IndexCache")); err == nil && len(raw) > 0 {
			var cachedINDEXes indexCache
			if json.Unmarshal(raw, &cachedINDEXes) == nil && len(cachedINDEXes) > 0 {
				return true
			}
		}
		if raw, err := GetEncryptedValue("TELA Search", []byte("CandidateCache")); err == nil && len(raw) > 0 {
			var cachedCandidates telaCandidateCache
			if json.Unmarshal(raw, &cachedCandidates) == nil {
				for _, meta := range cachedCandidates {
					if meta.Result == telaCandidateValidIndex {
						return true
					}
				}
			}
		}
		return false
	}
	getSearchResults = func() {
		if !telaWorkActive.CompareAndSwap(false, true) {
			return
		}
		defer func() {
			telaLaunchPending.Store(false)
			telaWorkActive.Store(false)
			// Clear network-paused flag if the wallet session has changed (closed/switched).
			// The paused-retry goroutine checks the flag itself, but this is a safety net.
			if !isWalletGenerationActive(currentWalletGeneration()) {
				telaNetworkPaused.Store(false)
			}
			if r := recover(); r != nil {
				logger.Errorf("[TELA-SEARCH] getSearchResults PANIC recovered: %v\n", r)
				isSearching = false
				scheduleTelaWarmup()
			}
		}()
		logger.Printf("[TELA-SEARCH] getSearchResults() starting...\n")
		scanStart := time.Now()
		scanCtx, scanCancel := context.WithCancel(context.Background())
		defer scanCancel()
		var syncWaitSeconds int
		var storedSCIDsCount int
		var allCandidates int
		var scannedCandidates int64
		var versionHits int64
		var indexInfoCalls int64
		var retryCount int64
		var filteredNonDisplayable int64
		var filteredByExclusion int64
		var filteredByMinLikes int64
		var preDispatchSkips int64
		var negCacheSkips int64
		var prefilterPassed int64
		var prefilterDropped int64
		var uiRefreshCount int64
		var progressWriteCount int64
		var interruptReason string
		var currentHeight int64
		var phasePrefilterMs int64
		var phaseScanMs int64
		var phaseFinalizeMs int64
		cacheHitMode := "full"
		fullScanReason := "cold_start"
		cacheIntegrity := "ok"
		keepProgressVisible := true
		var heightDelta int64
		var storedIndexedHeight int64

		// Scan-phase progress mapping. The default (prefilter path) maps the
		// scan across 60%→90%. The precomputed-candidates path skips the
		// prefilter entirely, so its scan phase starts lower (45%) and spans
		// wider (45%→90%) to avoid a jarring jump straight to 60%.
		scanProgressBase := 0.60
		scanProgressSpan := 0.30

		var gnomonSyncStartTime time.Time
		var estimatedTelaFallback = 30 * time.Second

		currentDaemonHeight := func() int64 {
			if engram.Disk == nil {
				return 0
			}

			return int64(engram.Disk.Get_Daemon_Height())
		}

		isGnomonCaughtUp := func() bool {
			if gnomon.Index == nil {
				return false
			}

			daemonHeight := currentDaemonHeight()
			if daemonHeight <= 0 {
				return gnomon.Index.LastIndexedHeight > 0
			}

			return gnomon.Index.LastIndexedHeight >= daemonHeight
		}

		allowTelaIndexMutations := isGnomonCaughtUp()

		deviceClass := "desktop"
		workerPoolSize := runtime.NumCPU()
		uiRefreshInterval := 250 * time.Millisecond
		progressCheckpointInterval := 2 * time.Second
		if a.Driver().Device().IsMobile() {
			deviceClass = "mobile"
			workerPoolSize = runtime.NumCPU() / 2
			if workerPoolSize < 6 {
				workerPoolSize = 6
			}
			if workerPoolSize > 12 {
				workerPoolSize = 12
			}
			uiRefreshInterval = 500 * time.Millisecond
			progressCheckpointInterval = 4 * time.Second
		} else {
			workerPoolSize = runtime.NumCPU() * 2
			if workerPoolSize < 16 {
				workerPoolSize = 16
			}
			if workerPoolSize > 64 {
				workerPoolSize = 64
			}
		}

		saveProgress := func(position, total int, scid, state string) {
			saveScanProgress(position, total, scid, state)
			atomic.AddInt64(&progressWriteCount, 1)
		}

		fyne.Do(func() {
			entrySearch.Disable()
			entryAddSCID.Disable()
		})
		if isSearching {
			return
		}

		isSearching = true

		// Handle force fresh scan - clear all caches and proceed
		if forceFreshScan {
			logger.Printf("[TELA] Force fresh scan requested - clearing all caches\n")
			searchMu.Lock()
			telaSearch = []INDEXwithRatings{}
			telaSCIDs = []string{}
			searchMu.Unlock()
			sAll = map[string]bool{}
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))
			forceFreshScan = false
			clearScanProgress()
			fullScanReason = "force_fresh_scan"
		}

		// On re-visit telaSearch is empty because it's a local variable in layoutTELA().
		// Load cached display results so we can show them immediately.
		searchMu.Lock()
		if len(telaSearch) == 0 {
			cachedDisplay := loadTelaDisplayCache()
			if len(cachedDisplay) > 0 {
				for _, entry := range cachedDisplay {
					if !isDisplayableTelaApp(entry.INDEX) {
						continue
					}
					telaSearch = append(telaSearch, entry)
				}
				telaSearch = deduplicateTelaSearch(telaSearch)
				logger.Printf("[TELA] Loaded %d apps from display cache into telaSearch\n", len(telaSearch))
			}
		}
		searchMu.Unlock()
		warmRatings()

		// Check for existing progress and handle resume scenarios
		progress := loadScanProgress()
		resumePosition := 0

		if progress.State == "completed" && !isScanProgressStale(progress, 24) {
			// Use cached results - progress is valid, already scanned
		} else if progress.State == "interrupted" && !isScanProgressStale(progress, 24) {
			// Resume from interrupted scan
			resumePosition = progress.Position
			setResultsText("  Resuming scan from position %d...", resumePosition)
			results.Color = apptheme.StatusTextColor()
			fyne.Do(func() {
				results.Refresh()
			})
		} else if progress.State == "interrupted" && isScanProgressStale(progress, 24) {
			// Clear stale interrupted progress
			clearScanProgress()
		}

		if gnomon.Index != nil {
			if storedHeightRaw, err := GetEncryptedValue("TELA Search", []byte("Last Indexed Height")); err == nil {
				if parsedHeight, parseErr := strconv.ParseInt(string(storedHeightRaw), 10, 64); parseErr == nil {
					storedIndexedHeight = parsedHeight
					heightDelta = gnomon.Index.LastIndexedHeight - storedIndexedHeight
					if heightDelta < 0 {
						heightDelta = 0
					}
				} else {
					cacheIntegrity = "missing_height"
				}
			} else {
				cacheIntegrity = "missing_height"
			}
		}
		if rescanRecheck {
			fullScanReason = "rescan_recheck"
		} else if heightDelta > 0 {
			cacheHitMode = "delta"
			fullScanReason = "height_delta"
		}

		// Already scanned - only skip if no updates are expected
		searchMu.RLock()
		hasCached := len(telaSearch) > 0
		searchMu.RUnlock()
		if hasCached && heightDelta == 0 && !rescanRecheck {
			keepProgressVisible = false
			fyne.Do(func() {
				searchMu.Lock()
				searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
				searchMu.Unlock()
				searchData.Set(searching)
				searchList.Refresh()
				searchMu.RLock()
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(telaSearch))
				searchMu.RUnlock()
				results.Color = apptheme.StatusTextColor()
				entrySearch.Enable()
				entryAddSCID.Enable()
			})

			labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
			labelLastScan.Color = apptheme.C.Green
			isSearching = false

			fyne.Do(func() {
				results.Refresh()
				labelLastScan.Refresh()
				hideTelaProgress()
			})

			return
		}

		if !keepProgressVisible && heightDelta == 0 && !rescanRecheck {
			searchMu.Lock()
			telaSearch = []INDEXwithRatings{}
			searchMu.Unlock()
			searchData.Set(nil)
		}
		labelLastScan.Text = ""

		fyne.Do(func() {
			btnShutdown.Disable()
			labelLastScan.Refresh()
		})

		defer func() {
			isSearching = false
			if !keepProgressVisible {
				setTelaStatus("", color.Transparent)
				hideTelaProgress()
			}
			fyne.Do(func() {
				entrySearch.Enable()
				entryAddSCID.Enable()
			})
			if !session.Offline && gnomon.Index != nil {
				if btnShutdown.Disabled() {
					fyne.Do(func() {
						btnShutdown.Enable()
					})
				}
			}
		}()

		// hasValidTelaJSONCache checks for a recent (< 24h) plain JSON cache.
		// If present, we can skip the Gnomon sync wait entirely — we already
		// know which SCIDs are TELA candidates from a previous prefilter run.
		hasValidTelaJSONCache := func() bool {
			cachePath := filepath.Join(AppPath(), "datashards", "tela_scid_cache.json")
			raw, err := os.ReadFile(cachePath)
			if err != nil || len(raw) == 0 {
				return false
			}
			var cache struct {
				SCIDs     []string `json:"scids"`
				Timestamp int64    `json:"timestamp"`
			}
			if err := json.Unmarshal(raw, &cache); err != nil || len(cache.SCIDs) == 0 {
				return false
			}
			if time.Now().Unix()-cache.Timestamp >= 86400 {
				return false
			}
			return true
		}

		gnomonReadyForTela := func() bool {
			// If we have a recent JSON cache, Gnomon doesn't need to be fully synced.
			// We already know which SCIDs are TELA candidates — skip the sync wait.
			if hasValidTelaJSONCache() {
				return true
			}
			if hasTelaCache() || len(telaSearch) > 0 {
				return true
			}
			if !gnomon.telaBootstrapReady() {
				return false
			}
			if gnomon.Index == nil {
				return false
			}
			if gnomon.Index.LastIndexedHeight <= 0 {
				return false
			}
			return isGnomonCaughtUp()
		}

		gnomonSyncStarted := false
		for !gnomonReadyForTela() {
			if !gnomonSyncStarted {
				gnomonSyncStartTime = time.Now()
				gnomonSyncStarted = true
			}
			syncWaitSeconds++
			// Check if user navigated away
			if !strings.Contains(session.Domain, ".tela") {
				interruptReason = "navigated_away"
				saveProgress(0, 0, "", "interrupted")
				return
			}

			// Check if Gnomon index became nil (stopped unexpectedly)
			if gnomon.Index == nil {
				interruptReason = "gnomon_nil_while_syncing"
				results.Text = "  Gnomon stopped unexpectedly"
				results.Color = apptheme.C.Red
				fyne.Do(func() {
					results.Refresh()
				})
				saveProgress(0, 0, "", "interrupted")
				return
			}

			// Check connection health - wait for reconnect if disconnected
			if !isDaemonConnected() {
				interruptReason = "connection_lost_syncing"
				results.Text = "  Connection lost, waiting for reconnect..."
				results.Color = apptheme.StatusTextColor()
				fyne.Do(func() {
					results.Refresh()
				})

				// Wait for connection to restore (up to 30 seconds)
				reconnectAttempts := 0
				for !isDaemonConnected() && reconnectAttempts < 30 {
					time.Sleep(time.Second)
					reconnectAttempts++

					// Check if user navigated away while waiting
					if !strings.Contains(session.Domain, ".tela") {
						saveProgress(0, 0, "", "interrupted")
						return
					}
				}

				// If still disconnected after 30 seconds, mark as interrupted
				if !isDaemonConnected() {
					interruptReason = "connection_timeout_syncing"
					keepProgressVisible = true
					results.Text = "  Connection timeout"
					results.Color = apptheme.C.Red
					fyne.Do(func() {
						results.Refresh()
					})
					saveProgress(0, 0, "", "interrupted")
					return
				}

				// Connection restored - continue syncing
				results.Text = "  Connection restored, resuming sync..."
				results.Color = apptheme.StatusTextColor()
			}

			// Show time-based progress during sync wait
			if gnomon.Index != nil && engram.Disk != nil {
				daemonHeight := int64(engram.Disk.Get_Daemon_Height())
				indexedHeight := gnomon.Index.LastIndexedHeight
				elapsed := time.Since(gnomonSyncStartTime)
				remainingBlocks := daemonHeight - indexedHeight
				processedBlocks := indexedHeight
				var estimatedGnomon time.Duration
				if processedBlocks > 0 && remainingBlocks > 0 {
					timePerBlock := float64(elapsed) / float64(processedBlocks)
					remainingTime := time.Duration(timePerBlock * float64(remainingBlocks))
					estimatedGnomon = elapsed + remainingTime
				} else if processedBlocks > 0 {
					estimatedGnomon = elapsed
				} else {
					estimatedGnomon = elapsed + estimatedTelaFallback
				}
				provisionalTotal := estimatedGnomon + estimatedTelaFallback
				if provisionalTotal > 0 {
					syncProgress := float64(elapsed) / float64(provisionalTotal)
					if daemonHeight > 0 && indexedHeight > 0 {
						heightProgress := float64(indexedHeight) / float64(daemonHeight)
						// Ensure we start at a low value even if synced, moving towards 100%
						if heightProgress > 0.99 && syncProgress < 0.1 {
							syncProgress = 0.05 + (syncProgress * 0.5) // Start around 5% if Gnomon synced
						} else {
							syncProgress = (syncProgress * 0.4) + (heightProgress * 0.6)
						}
					}
					// Cap sync progress at 15% — real work (prefilter/scan) hasn't started yet
					if syncProgress > 0.15 {
						syncProgress = 0.15
					}
					updateTelaProgress(syncProgress)
				}
				setTelaStatus(fmt.Sprintf("Synching gnomon index... [%d / %d]", indexedHeight, daemonHeight), apptheme.StatusTextColor())
				fyne.Do(func() {
					results.Refresh()
				})
			}

			fyne.Do(func() {
				entrySearch.Disable()
				entryAddSCID.Disable()
			})
			time.Sleep(1 * time.Second)
		}

		// Re-evaluate after the sync wait: the value captured at the start of getSearchResults
		// is stale if we blocked until Gnomon caught up, otherwise "defer cached only" can skip
		// the full owner/SCID scan incorrectly.
		if forceFreshScan {
			logger.Printf("[TELA] Force fresh scan observed after sync wait - clearing state\n")
			searchMu.Lock()
			telaSearch = []INDEXwithRatings{}
			telaSCIDs = []string{}
			sAll = map[string]bool{}
			searchMu.Unlock()
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))
			forceFreshScan = false
			clearScanProgress()
			fullScanReason = "force_fresh_scan"
		}
		allowTelaIndexMutations = isGnomonCaughtUp()

		// Gnomon sync complete - record duration and initialize TELA timing
		indexCacheStore := loadTelaIndexCache()
		ratingsCache := make(map[string]tela.Rating_Result)
		candidateCache := loadTelaCandidateCache()
		currentScanHeight := storedIndexedHeight
		var candidateCacheMu sync.RWMutex
		var indexMu sync.Mutex
		var scidsToIndex []string
		if gnomon.Index != nil {
			currentScanHeight = gnomon.Index.LastIndexedHeight
		}
		if !restrictiveMode {
			// Merge negative SCIDs from both in-memory candidate cache and persistent storage
			// for maximum cache hit rate across sessions.
			sAll = candidateCache.negativeSet()
			persistedNegatives := loadStringSetFromEncryptedStorage("TELA Search", "NegativeCache")
			for scid := range persistedNegatives {
				sAll[scid] = true
			}
			if len(sAll) > 0 {
				logger.Printf("[TELA] Loaded %d negative SCIDs from cache (%d from storage)\n", len(sAll), len(persistedNegatives))
			}
		}

		setCandidateCache := func(scid, result string) {
			candidateCacheMu.Lock()
			candidateCache.set(scid, result, currentScanHeight)
			candidateCacheMu.Unlock()
		}
		isNegativeSCID := func(scid string) bool {
			candidateCacheMu.RLock()
			defer candidateCacheMu.RUnlock()
			return sAll[scid]
		}
		setNegativeSCID := func(scid string, negative bool) {
			candidateCacheMu.Lock()
			if negative {
				sAll[scid] = true
			} else {
				delete(sAll, scid)
			}
			candidateCacheMu.Unlock()
		}

		cachedDisplay := loadTelaDisplayCache()
		if len(cachedDisplay) > 0 {
			for _, entry := range cachedDisplay {
				if !isDisplayableTelaApp(entry.INDEX) {
					continue
				}
				searchMu.Lock()
				telaSearch = append(telaSearch, entry)
				telaSCIDs = append(telaSCIDs, entry.SCID)
				searchMu.Unlock()
				indexCacheStore[entry.SCID] = entry.INDEX
			}
		}
		searchMu.Lock()
		telaSearch = deduplicateTelaSearch(telaSearch)
		searchMu.Unlock()

		storedSCIDs, err := GetEncryptedValue("TELA Search", []byte("SCIDs"))
		if err != nil {
			// Nothing stored, scan for SCIDs
			if len(telaSCIDs) == 0 {
				telaSCIDs = candidateCache.validSCIDs()
			}
			cacheIntegrity = "missing_scids"
			fullScanReason = "no_scid_cache"
			logger.Debugf("[Engram] Could not get stored TELA SCIDs: %s\n", err)
		} else {
			// Have stored SCIDs
			var unmarshaledSCIDs []string
			if err := json.Unmarshal(storedSCIDs, &unmarshaledSCIDs); err == nil {
				scidMap := make(map[string]bool)
				for _, sc := range telaSCIDs {
					scidMap[sc] = true
				}
				for _, sc := range unmarshaledSCIDs {
					if !scidMap[sc] {
						telaSCIDs = append(telaSCIDs, sc)
					}
				}
			}

			fyne.Do(func() {
				results.Refresh()
			})

			// Batch-fetch INDEX data for cached SCIDs missing from indexCacheStore
			// This replaces per-SCID tela.GetINDEXInfo() calls that each open a new WebSocket
			searchMu.RLock()
			var cacheMissed []string
			for _, sc := range telaSCIDs {
				if _, ok := indexCacheStore[sc]; !ok {
					cacheMissed = append(cacheMissed, sc)
				}
			}
			searchMu.RUnlock()
			if len(cacheMissed) > 0 {
				setResultsText("  Fetching INDEX data... (%d SCIDs)", len(cacheMissed))
				results.Color = apptheme.StatusTextColor()
				fyne.Do(func() {
					results.Refresh()
				})

				fetched, ratingsFetched, invalid, fetchErr := batchFetchINDEXes(scanCtx, cacheMissed, 50)
				if fetchErr != nil {
					logger.Printf("[TELA] Batch INDEX fetch for cached SCIDs: %v\n", fetchErr)
				}
				for scid, index := range fetched {
					indexCacheStore[scid] = index
					setCandidateCache(scid, telaCandidateValidIndex)
					setNegativeSCID(scid, false)
					if r, ok := ratingsFetched[scid]; ok {
						ratingsCache[scid] = r
					}
				}
				for scid := range invalid {
					setCandidateCache(scid, telaCandidateInvalidIndex)
					setNegativeSCID(scid, true)
				}
				atomic.AddInt64(&indexInfoCalls, int64(len(cacheMissed)))
			}

			searchMu.RLock()
			var searchMap = make(map[string]bool)
			for _, entry := range telaSearch {
				searchMap[entry.SCID] = true
			}
			searchMu.RUnlock()
			var missingSCIDs []string
			for _, sc := range telaSCIDs {
				if !searchMap[sc] {
					missingSCIDs = append(missingSCIDs, sc)
				}
			}

			if len(missingSCIDs) > 0 {
				cachedAdded := int64(0)
				cachedWorkers := workerPoolSize / 2
				if cachedWorkers < 8 {
					cachedWorkers = 8
				}
				if cachedWorkers > 24 {
					cachedWorkers = 24
				}
				cachedSlots := make(chan struct{}, cachedWorkers)
				var cachedWg sync.WaitGroup

				for i, sc := range missingSCIDs {
					if !isDaemonConnected() {
						break
					}

					cachedSlots <- struct{}{}
					cachedWg.Add(1)
					go func(idx int, scid string) {
						defer func() {
							<-cachedSlots
							cachedWg.Done()
						}()

						var index tela.INDEX
						if cached, ok := indexCacheStore[scid]; ok {
							index = cached
						} else {
							return
						}

						if !isDisplayableTelaApp(index) {
							setCandidateCache(scid, telaCandidateNoDocs)
							setNegativeSCID(scid, true)
							return
						}

						if allowTelaIndexMutations && gnomon.GetAllSCIDVariableDetails(scid) == nil {
							if atomic.AddInt64(&cachedAdded, 1)%8 == 0 {
								setResultsText("  Adding... (%d / %d)", idx+1, len(missingSCIDs))
								fyne.Do(func() {
									results.Color = apptheme.StatusTextColor()
									results.Refresh()
								})
							}
							gnomon.AddSCIDToIndex(scid)
						}

						if restrictiveMode {
							setCandidateCache(scid, telaCandidateValidIndex)
							return
						}

						_, ratings, err := getLikesRatioCached(scid, index.DURL, searchExclusions, minLikes, ratingsCache)
						if err != nil {
							if strings.Contains(err.Error(), "found search exclusion") {
								atomic.AddInt64(&filteredByExclusion, 1)
							} else if strings.Contains(err.Error(), "below min rating setting") {
								atomic.AddInt64(&filteredByMinLikes, 1)
							}
							setCandidateCache(scid, telaCandidateExcludedByURL)
							return
						}

						setCandidateCache(scid, telaCandidateValidIndex)
						setNegativeSCID(scid, false)

						searchMu.Lock()
						telaSearch = append(telaSearch, INDEXwithRatings{ratings: ratings, INDEX: index})
						searchMu.Unlock()
					}(i, sc)
				}

				cachedWg.Wait()
			}
			storedSCIDsCount = len(telaSCIDs)

			// Only defer the full scan when we have cached rows to show; otherwise continue
			// into GetAllOwnersAndSCIDs so an initial or empty-cache run still enumerates.
			searchMu.RLock()
			hasSearch := len(telaSearch) > 0
			hasSCIDs := len(telaSCIDs) > 0
			searchMu.RUnlock()
			if !allowTelaIndexMutations && (hasSearch || hasSCIDs) {
				cacheHitMode = "cached_syncing"
				fullScanReason = ""
				if !hasSearch && hasSCIDs {
					searchMu.RLock()
					localSCIDs := make([]string, len(telaSCIDs))
					copy(localSCIDs, telaSCIDs)
					searchMu.RUnlock()
					for _, scid := range localSCIDs {
						if index, ok := indexCacheStore[scid]; ok {
							if !isDisplayableTelaApp(index) {
								continue
							}
							searchMu.Lock()
							telaSearch = append(telaSearch, INDEXwithRatings{INDEX: index})
							searchMu.Unlock()
						}
					}
					warmRatings()
				}
				fyne.Do(func() {
					searchMu.Lock()
					searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
					searchMu.Unlock()
					searchData.Set(searching)
					searchList.Refresh()
					searchMu.RLock()
					if len(telaSearch) > 0 {
						results.Text = fmt.Sprintf("  TELA cache loaded while Gnomon syncs: %d", len(telaSearch))
					} else {
						results.Text = "  Loading cached TELA data..."
					}
					searchMu.RUnlock()
					results.Color = apptheme.StatusTextColor()
					entrySearch.Enable()
					entryAddSCID.Enable()
				})

				if last, err := GetEncryptedValue("TELA Search", []byte("Last Scan")); err == nil {
					lastScan = string(last)
					labelLastScan.Text = fmt.Sprintf("  %s (syncing)", lastScan)
					labelLastScan.Color = apptheme.StatusTextColor()
				}

				fyne.Do(func() {
					results.Refresh()
					labelLastScan.Refresh()
				})

				keepProgressVisible = false
				completeTelaScanProgress()
				logger.Printf("[TELA] Deferring full scan until Gnomon catches up; showing cached results only\n")
				return
			}

			if !rescanRecheck && (len(telaSearch) > 0 || len(telaSCIDs) > 0) && heightDelta == 0 {
				cacheHitMode = "cached_only"
				fullScanReason = ""
				keepProgressVisible = false
				completeTelaScanProgress()
				fyne.Do(func() {
					searchMu.Lock()
					searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
					searchMu.Unlock()
					searchData.Set(searching)
					searchList.Refresh()
					searchMu.RLock()
					results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(telaSearch))
					searchMu.RUnlock()
					results.Color = apptheme.StatusTextColor()
					entrySearch.Enable()
					entryAddSCID.Enable()
				})

				if last, err := GetEncryptedValue("TELA Search", []byte("Last Scan")); err == nil {
					lastScan = string(last)
					labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
					labelLastScan.Color = apptheme.C.Green
				}

				if restrictiveMode && len(telaSearch) < 1 {
					errorText.Text = "TELA is in restrictive mode"
					errorText.Color = apptheme.StatusTextColor()
				}

				fyne.Do(func() {
					results.Refresh()
					labelLastScan.Refresh()
					errorText.Refresh()
				})

				logger.Printf("[TELA] Search metrics: outcome=completed elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), versionHits, indexInfoCalls, retryCount, len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)

				return
			}
			if heightDelta > 0 {
				fullScanReason = "height_delta"
			}
		}

		var wg sync.WaitGroup

		hasCachedTelaData := hasTelaCache()
		var all = map[string]string{}
		usedPrecomputedCandidates := false
		if restrictiveMode {
			for _, sc := range telaSCIDs {
				all[sc] = ""
			}
		} else {
			if gnomon.Index == nil ||
				(gnomon.Index.DBType == "gravdb" && gnomon.Index.GravDBBackend == nil) ||
				(gnomon.Index.DBType == "boltdb" && gnomon.Index.BBSBackend == nil) {
				keepProgressVisible = true
				setTelaStatus("Waiting for Gnomon backend...", apptheme.StatusTextColor())
				showInfiniteTelaProgress()
				scheduleTelaWarmup()
				return
			}
			// Fast path: use pre-computed TELA candidates if available
			candidates := gnomon.GetTelaCandidates()
			if len(candidates) > 0 {
				usedPrecomputedCandidates = true
				logger.Printf("[TELA-SEARCH] Using %d pre-computed TELA candidates from Gnomon\n", len(candidates))
				for _, scid := range candidates {
					all[scid] = ""
				}
				logger.Printf("[TELA-SEARCH] Candidate pool ready: %d total (backfillActive=%v backfillFailed=%v lastHeight=%d gnomonHeight=%d)\n",
					len(all), telaBackfillActive.Load(), telaBackfillFailed.Load(), lastBackfillHeight, currentHeight)
			} else {
				// Fallback 1: plain JSON file cache (no encryption, no Graviton, survives abrupt kills)
				cachePath := filepath.Join(AppPath(), "datashards", "tela_scid_cache.json")
				if raw, err := os.ReadFile(cachePath); err == nil && len(raw) > 0 {
					var cache struct {
						SCIDs     []string `json:"scids"`
						Timestamp int64    `json:"timestamp"`
						Daemon    string   `json:"daemon"`
					}
					if err := json.Unmarshal(raw, &cache); err == nil && len(cache.SCIDs) > 0 {
						if time.Now().Unix()-cache.Timestamp < 86400 {
							usedPrecomputedCandidates = true
							logger.Printf("[TELA-SEARCH] Using %d validated TELA SCIDs from JSON cache (age=%dh, cached_daemon=%s, current_daemon=%s)\n", len(cache.SCIDs), (time.Now().Unix()-cache.Timestamp)/3600, cache.Daemon, session.Daemon)
							for _, scid := range cache.SCIDs {
								all[scid] = ""
							}
						} else {
							logger.Printf("[TELA-SEARCH] JSON cache stale (age=%dh, max=24h)\n", (time.Now().Unix()-cache.Timestamp)/3600)
						}
					} else {
						logger.Printf("[TELA-SEARCH] JSON cache unmarshal failed or empty: %v\n", err)
					}
				} else {
					logger.Printf("[TELA-SEARCH] JSON cache not found or unreadable: %v\n", err)
				}

				// Fallback 2: encrypted Graviton cache (legacy, often fails silently)
				if !usedPrecomputedCandidates {
					if raw, err := GetEncryptedValue("TELA Search", []byte("ValidatedSCIDs")); err == nil && len(raw) > 0 {
						var validated []string
						if err := json.Unmarshal(raw, &validated); err == nil && len(validated) > 0 {
							if tsRaw, err := GetEncryptedValue("TELA Search", []byte("ValidatedSCIDsTimestamp")); err == nil {
								var ts int64
								if err := json.Unmarshal(tsRaw, &ts); err == nil {
									if time.Now().Unix()-ts < 86400 {
										usedPrecomputedCandidates = true
										logger.Printf("[TELA-SEARCH] Using %d validated TELA SCIDs from encrypted cache (age=%dh)\n", len(validated), (time.Now().Unix()-ts)/3600)
										for _, scid := range validated {
											all[scid] = ""
										}
									}
								}
							}
						}
					}
				}

				if !usedPrecomputedCandidates {
					logger.Printf("[TELA-SEARCH] Fetching all indexed owners and SCIDs...\n")
					all = gnomon.GetAllOwnersAndSCIDs()
					logger.Printf("[TELA-SEARCH] Found %d total indexed SCIDs\n", len(all))
				}
			}
		}

		if !restrictiveMode && !hasCachedTelaData && len(all) <= 1 {
			keepProgressVisible = true
			setTelaStatus("Gnomon indexing in progress...", apptheme.StatusTextColor())
			showInfiniteTelaProgress()
			scheduleTelaWarmup()
			return
		}

		allSCIDs := make([]string, 0, len(all))
		for sc := range all {
			allSCIDs = append(allSCIDs, sc)
		}
		sort.Strings(allSCIDs)

		// Delta scan block removed as it drops valid SCIDs with no interaction heights.

		// Create set of known TELA SCIDs for O(1) lookup
		knownTelaSCIDs := make(map[string]bool, len(telaSCIDs))
		for _, sc := range telaSCIDs {
			knownTelaSCIDs[sc] = true
		}

		prefilterAllowed := map[string]bool{}
		if !restrictiveMode {
			if usedPrecomputedCandidates {
				// Candidates from GetTelaCandidates() are already verified to have telaVersion.
				// Skip the expensive RPC prefilter entirely.
				logger.Printf("[TELA-SEARCH] Skipping prefilter for %d pre-computed candidates\n", len(allSCIDs))
				for _, sc := range allSCIDs {
					if !rescanRecheck && isNegativeSCID(sc) {
						prefilterAllowed[sc] = false
					} else {
						prefilterAllowed[sc] = true
					}
				}
				// Candidates are already verified; the remaining work is the
				// INDEX batch fetch + scan. Start the scan phase lower than the
				// prefilter path (45% instead of 60%) so the bar tracks the
				// fetch step instead of jumping straight to 60%.
				scanProgressBase = 0.45
				scanProgressSpan = 0.45
				updateTelaProgress(0.45)
			} else {
				candidates := make([]string, 0, len(allSCIDs))
				for _, sc := range allSCIDs {
					if !rescanRecheck && isNegativeSCID(sc) {
						prefilterAllowed[sc] = false
						continue
					}
					// Skip prefilter for SCIDs with cached INDEX data
					if _, hasIndexData := indexCacheStore[sc]; hasIndexData {
						prefilterAllowed[sc] = true
						continue
					}
					// Skip prefilter for known TELA SCIDs from storage
					if knownTelaSCIDs[sc] {
						prefilterAllowed[sc] = true
						continue
					}
					candidates = append(candidates, sc)
				}

				setTelaStatus(fmt.Sprintf("Checking TELA candidates... (%d total)", len(candidates)), apptheme.StatusTextColor())
				displayedTelaProgress = 0.10
				setTelaProgress(0.10)
				uiDo(func() {
					results.Refresh()
				})

				prefilterStart := time.Now()

				// Create a dedicated RPC pool for prefilter.
				poolSize := 6
				if !a.Driver().Device().IsMobile() {
					poolSize = 8
				}
				batchSize := 200
				if !a.Driver().Device().IsMobile() {
					batchSize = 500
				}
				// Reduce concurrency for remote daemons to avoid connection overwhelm.
				daemonLower := strings.ToLower(session.Daemon)
				if !strings.Contains(daemonLower, "127.0.0.1") && !strings.Contains(daemonLower, "localhost") && !strings.HasPrefix(daemonLower, ":") {
					if poolSize > 4 {
						poolSize = 4
					}
					if batchSize > 200 {
						batchSize = 200
					}
				}
				pool, poolCleanup, poolErr := dialRPCPool(session.Daemon, poolSize)
				if poolErr != nil {
					logger.Printf("[TELA] Failed to create RPC pool (%d connections): %v\n", poolSize, poolErr)
					if gnomon.Index != nil && gnomon.Index.RPC != nil && gnomon.Index.RPC.RPC != nil {
						pool = []*jrpc2.Client{gnomon.Index.RPC.RPC}
						poolCleanup = func() {}
					} else {
						pool = nil
						poolCleanup = func() {}
					}
				}

				var passed map[string]bool
				var batchStats batchPrefilterStats
				var batchErr error
				if len(pool) > 0 {
					passed, batchStats, batchErr = batchPrefilterTelaVersions(scanCtx, candidates, batchSize, 3, pool, func(completed, total int) {
						results.Color = apptheme.StatusTextColor()
						var progress float64
						if total > 0 {
							progress = 0.15 + 0.45*float64(completed)/float64(total)
						}
						updateTelaProgress(progress)
						setTelaStatus(fmt.Sprintf("Checking TELA candidates... (%d / %d)", completed, total), apptheme.StatusTextColor())
						uiDo(func() {
							results.Refresh()
						})
					})
					logger.Printf("[TELA] Prefilter returned, cleaning up %d pool connections...\n", len(pool))
					poolCleanup()
					logger.Printf("[TELA] Pool cleanup done\n")
				} else {
					batchErr = fmt.Errorf("no RPC connections available")
				}
				phasePrefilterMs = time.Since(prefilterStart).Milliseconds()
				logger.Printf("[TELA] Prefilter phase took %dms, passed=%d err=%v\n", phasePrefilterMs, len(passed), batchErr)
				if batchErr != nil {
					logger.Printf("[TELA] Batch prefilter error: %v\n", batchErr)
					if len(telaSearch) > 0 {
						keepProgressVisible = false
						logger.Printf("[TELA] Prefilter failed but %d cached results available, showing them\n", len(telaSearch))
						fyne.Do(func() {
							searchMu.Lock()
							searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
							searchMu.Unlock()
							searchData.Set(searching)
							searchList.Refresh()
							searchMu.RLock()
							results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d (prefilter error)", len(telaSearch))
							searchMu.RUnlock()
							results.Color = apptheme.StatusTextColor()
							results.Refresh()
						})
						completeTelaScanProgress()
						return
					}
					keepProgressVisible = true
					setTelaStatus("Network error during prefilter, retrying...", apptheme.StatusTextColor())
					showInfiniteTelaProgress()
					scheduleTelaWarmup()
					return
				}

				for sc := range passed {
					prefilterAllowed[sc] = true
					atomic.AddInt64(&prefilterPassed, 1)
				}
				atomic.AddInt64(&prefilterDropped, int64(len(candidates)-len(passed)))
				atomic.AddInt64(&versionHits, batchStats.VersionHits)
				logger.Printf("[TELA] Prefilter: passed=%d dropped=%d version_hits=%d\n", len(passed), len(candidates)-len(passed), batchStats.VersionHits)

				// Persist validated TELA SCIDs to plain JSON cache for fast-path fallback on next startup.
				if len(passed) > 0 {
					validatedSCIDs := make([]string, 0, len(passed))
					for scid := range passed {
						validatedSCIDs = append(validatedSCIDs, scid)
					}
					cache := struct {
						SCIDs     []string `json:"scids"`
						Timestamp int64    `json:"timestamp"`
						Daemon    string   `json:"daemon"`
					}{
						SCIDs:     validatedSCIDs,
						Timestamp: time.Now().Unix(),
						Daemon:    session.Daemon,
					}
					if raw, err := json.MarshalIndent(cache, "", "  "); err == nil {
						cachePath := filepath.Join(AppPath(), "datashards", "tela_scid_cache.json")
						if writeErr := os.WriteFile(cachePath, raw, 0600); writeErr != nil {
							logger.Printf("[TELA] Failed to write JSON cache: %v\n", writeErr)
						} else {
							logger.Printf("[TELA] Persisted %d validated SCIDs to JSON cache\n", len(validatedSCIDs))
						}
					}

					// Also try legacy encrypted cache (may help same-session reuse)
					if raw, err := json.Marshal(validatedSCIDs); err == nil {
						if encErr := StoreEncryptedValue("TELA Search", []byte("ValidatedSCIDs"), raw); encErr != nil {
							logger.Printf("[TELA] Failed to write encrypted SCID cache: %v\n", encErr)
						}
						if tsRaw, err := json.Marshal(time.Now().Unix()); err == nil {
							if encErr := StoreEncryptedValue("TELA Search", []byte("ValidatedSCIDsTimestamp"), tsRaw); encErr != nil {
								logger.Printf("[TELA] Failed to write encrypted timestamp cache: %v\n", encErr)
							}
						}
					}
				}
			}

			// Always start a background backfill to discover NEW TELA apps published
			// since the embedded list was compiled. This runs regardless of whether
			// we used embedded SCIDs or ran the prefilter — new apps need discovery.
			// Also re-trigger when Gnomon has indexed new blocks since the last backfill.
			currentHeight = 0
			if gnomon.Index != nil {
				currentHeight = gnomon.Index.LastIndexedHeight
			}
			heightGrew := currentHeight > lastBackfillHeight && lastBackfillHeight > 0
			if !restrictiveMode && gnomon.Index != nil && !telaBackfillActive.Load() && (!telaBackfillFailed.Load() || heightGrew) {
				if heightGrew {
					logger.Printf("[TELA] Gnomon height grew %d -> %d; resetting backfill failure state\n", lastBackfillHeight, currentHeight)
					telaBackfillFailed.Store(false)
				}
				workers := 8
				if a.Driver().Device().IsMobile() {
					workers = 4
				}
				telaBackfillActive.Store(true)
				go func() {
					defer telaBackfillActive.Store(false)
					defer func() {
						if r := recover(); r != nil {
							logger.Printf("[TELA] Backfill panic: %v\n", r)
							telaBackfillFailed.Store(true)
						}
					}()
					err := gnomon.Index.BackfillTelaCandidates(workers)
					if err != nil {
						logger.Printf("[TELA] Backfill failed: %v\n", err)
						telaBackfillFailed.Store(true)
					} else {
						lastBackfillHeight = currentHeight
						logger.Printf("[TELA] Backfill completed at height %d\n", currentHeight)
					}
				}()
			}
		}

		// Batch-fetch INDEX data for prefilter-passed SCIDs not yet in indexCacheStore.
		// This replaces per-SCID tela.GetINDEXInfo() calls that each open a new WebSocket.
		indexFetchFailed := make(map[string]bool) // Track SCIDs whose INDEX fetch failed due to network errors
		networkErrorDuringFetch := false          // Track if there was a network error during batch fetch
		indexFetchRecoverableFailure := false
		if !restrictiveMode {
			var indexNeeded []string
			for scid, allowed := range prefilterAllowed {
				if allowed {
					if _, ok := indexCacheStore[scid]; !ok {
						indexNeeded = append(indexNeeded, scid)
					}
				}
			}
			if len(indexNeeded) > 0 {
				logger.Printf("[TELA] Batch INDEX fetch starting for %d SCIDs...\n", len(indexNeeded))
				setResultsText("  Fetching INDEX data... (%d SCIDs)", len(indexNeeded))
				results.Color = apptheme.StatusTextColor()
				uiDo(func() {
					results.Refresh()
				})

				fetched, ratingsFetched, invalid, fetchErr := batchFetchINDEXes(scanCtx, indexNeeded, 50)
				logger.Printf("[TELA] Batch INDEX fetch done: fetched=%d err=%v\n", len(fetched), fetchErr)
				if fetchErr != nil {
					logger.Printf("[TELA] Batch INDEX fetch for scan: %v\n", fetchErr)
					networkErrorDuringFetch = true
					if len(indexNeeded) > 0 && len(fetched) == 0 {
						indexFetchRecoverableFailure = true
					}
					// Mark SCIDs as failed due to network error - these should NOT be marked as negative
					// They will be retried on the next scan
					for _, scid := range indexNeeded {
						if _, ok := fetched[scid]; !ok {
							indexFetchFailed[scid] = true
						}
					}
				}
				for scid, index := range fetched {
					indexCacheStore[scid] = index
					setCandidateCache(scid, telaCandidateValidIndex)
					setNegativeSCID(scid, false)
					if r, ok := ratingsFetched[scid]; ok {
						ratingsCache[scid] = r
					}
				}
				for scid := range invalid {
					setCandidateCache(scid, telaCandidateInvalidIndex)
					setNegativeSCID(scid, true)
				}
				atomic.AddInt64(&indexInfoCalls, int64(len(indexNeeded)))
			}
		}

		if indexFetchRecoverableFailure && len(telaSearch) == 0 {
			keepProgressVisible = true
			interruptReason = "index_fetch_retrying"
			results.Hide()
			setTelaStatus("Retrying TELA fetch...", apptheme.StatusTextColor())
			showInfiniteTelaProgress()
			phaseFinalizeMs = 0
			logger.Printf("[TELA] Search metrics: outcome=interrupted reason=index_fetch_retrying elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
			retryGeneration := currentWalletGeneration()
			go func() {
				time.Sleep(3 * time.Second)
				if !strings.Contains(session.Domain, ".tela") || !isWalletGenerationActive(retryGeneration) || globals.Exit_In_Progress {
					return
				}
				maybeStartTelaWork(true)
			}()
			return
		}

		allLen := len(allSCIDs)
		allCandidates = allLen
		resumeTarget := resumePosition
		scanned := int64(resumePosition) // Progress counter, starts from resume position
		scannedCandidates = scanned
		workers := make(chan struct{}, workerPoolSize)
		interrupted := false
		var scanMu sync.Mutex
		lastUIRefresh := time.Now().Add(-uiRefreshInterval)
		lastProgressSave := time.Now()
		seenSCIDs := make(map[string]bool, len(telaSCIDs))
		for _, scid := range telaSCIDs {
			seenSCIDs[scid] = true
		}

		scanPhaseStart := time.Now()

		for i := resumeTarget; i < allLen; i++ {
			sc := allSCIDs[i]

			// Check for interrupted conditions
			if gnomon.Index == nil || !strings.Contains(session.Domain, ".tela") {
				if gnomon.Index == nil {
					interruptReason = "gnomon_nil_during_scan"
				} else {
					interruptReason = "navigated_away"
				}
				interrupted = true
				break
			}

			// Check connection during scan
			if !isDaemonConnected() {
				interruptReason = "connection_lost_during_scan"
				results.Text = "  Connection lost during scan"
				results.Color = apptheme.C.Red
				uiDo(func() {
					results.Refresh()
				})
				interrupted = true
				break
			}

			scanMu.Lock()
			alreadySeen := seenSCIDs[sc]
			scanMu.Unlock()
			if !restrictiveMode && !rescanRecheck && isNegativeSCID(sc) {
				atomic.AddInt64(&negCacheSkips, 1)
			}
			if !restrictiveMode && !rescanRecheck && (isNegativeSCID(sc) || alreadySeen || !prefilterAllowed[sc]) {
				atomic.AddInt64(&preDispatchSkips, 1)
				scanned = atomic.AddInt64(&scannedCandidates, 1)
				continue
			}

			scanned = atomic.AddInt64(&scannedCandidates, 1)
			now := time.Now()
			if now.Sub(lastUIRefresh) >= uiRefreshInterval || scanned >= int64(allLen) {
				lastUIRefresh = now
				setResultsText("  Scanning... (%d / %d)", scanned, allLen)
				results.Color = apptheme.StatusTextColor()
				// Phase-based progress: scan spans scanProgressBase -> 90%
				if allLen > 0 {
					updateTelaProgress(scanProgressBase + scanProgressSpan*float64(scanned)/float64(allLen))
				}
				uiDo(func() {
					results.Refresh()
				})
				atomic.AddInt64(&uiRefreshCount, 1)
			}

			if now.Sub(lastProgressSave) >= progressCheckpointInterval {
				saveProgress(int(scanned), allLen, sc, "scanning")
				lastProgressSave = now

				scanMu.Lock()
				scidsSnapshot := make([]string, len(telaSCIDs))
				copy(scidsSnapshot, telaSCIDs)
				scanMu.Unlock()

				if storeSCIDs, err := json.Marshal(scidsSnapshot); err == nil {
					if err := StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs); err != nil {
						logger.Printf("[TELA] Failed storing checkpoint SCIDs: %v\n", err)
					} else {
						logger.Printf("[TELA] Checkpoint saved %d SCIDs\n", len(scidsSnapshot))
					}
				}

				if err := saveTelaIndexCache(indexCacheStore); err != nil {
					logger.Printf("[TELA] Failed storing checkpoint INDEX cache: %v\n", err)
				}
			}

			workers <- struct{}{}
			wg.Add(1)
			go func(scid string) {
				defer func() {
					<-workers
					wg.Done()
				}()

				// Check if Gnomon was stopped
				if gnomon.Index == nil {
					return
				}

				if restrictiveMode || prefilterAllowed[scid] {
					// Skip SCIDs whose INDEX fetch failed due to network errors - don't mark as negative
					// They will be retried on the next scan
					if indexFetchFailed[scid] {
						return
					}

					var index tela.INDEX
					if cached, ok := indexCacheStore[scid]; ok {
						index = cached
					} else {
						// INDEX not in cache - this SCID passed prefilter but wasn't fetched
						// This could happen if batch fetch was skipped or failed silently
						// Don't mark as negative, just skip for now
						return
					}

					if isDisplayableTelaApp(index) {

						if allowTelaIndexMutations && gnomon.GetAllSCIDVariableDetails(scid) == nil {
							indexMu.Lock()
							scidsToIndex = append(scidsToIndex, scid)
							indexMu.Unlock()
						}

						// In restrictive mode, the list is initialzed from telaSCIDs
						scanMu.Lock()
						if !restrictiveMode {
							if !seenSCIDs[scid] {
								seenSCIDs[scid] = true
								telaSCIDs = append(telaSCIDs, scid)
							}
						}
						scanMu.Unlock()

						_, ratings, err := getLikesRatioCached(scid, index.DURL, searchExclusions, minLikes, ratingsCache)
						if err != nil {
							if strings.Contains(err.Error(), "found search exclusion") {
								atomic.AddInt64(&filteredByExclusion, 1)
							} else if strings.Contains(err.Error(), "below min rating setting") {
								atomic.AddInt64(&filteredByMinLikes, 1)
							}
							setCandidateCache(scid, telaCandidateExcludedByURL)
							return
						}

						setCandidateCache(scid, telaCandidateValidIndex)
						setNegativeSCID(scid, false)

						searchMu.Lock()
						telaSearch = append(telaSearch, INDEXwithRatings{ratings: ratings, INDEX: index})
						searchMu.Unlock()
					} else {
						atomic.AddInt64(&filteredNonDisplayable, 1)
						setCandidateCache(scid, telaCandidateNoDocs)
						setNegativeSCID(scid, true)
					}
				}
			}(sc)
		}

		if !strings.Contains(session.Domain, ".tela") {
			interrupted = true
		}

		wg.Wait()
		phaseScanMs = time.Since(scanPhaseStart).Milliseconds()

		if len(scidsToIndex) > 0 && gnomon.Index != nil {
			batch := make(map[string]*structures.FastSyncImport, len(scidsToIndex))
			for _, scid := range scidsToIndex {
				batch[scid] = &structures.FastSyncImport{}
			}
			if err := gnomon.Index.AddSCIDToIndex(batch, false, true); err != nil {
				logger.Printf("[TELA] Batch index error: %v\n", err)
			} else {
				logger.Printf("[TELA] Batch indexed %d SCIDs\n", len(scidsToIndex))
			}
		}

		if interrupted {
			scanMu.Lock()
			scidsSnapshot := make([]string, len(telaSCIDs))
			copy(scidsSnapshot, telaSCIDs)
			scanMu.Unlock()

			if storeSCIDs, err := json.Marshal(scidsSnapshot); err == nil {
				if err := StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs); err != nil {
					logger.Printf("[TELA] Failed storing interrupted SCIDs: %v\n", err)
				} else {
					logger.Printf("[TELA] Saved %d SCIDs before interruption\n", len(scidsSnapshot))
				}
			}

			if err := saveTelaIndexCache(indexCacheStore); err != nil {
				logger.Printf("[TELA] Failed storing interrupted INDEX cache: %v\n", err)
			}

			candidateCacheMu.RLock()
			candidateCacheSnapshot := make(telaCandidateCache, len(candidateCache))
			for scid, meta := range candidateCache {
				candidateCacheSnapshot[scid] = meta
			}
			candidateCacheMu.RUnlock()
			if err := saveTelaCandidateCache(candidateCacheSnapshot); err != nil {
				logger.Printf("[TELA] Failed storing interrupted candidate cache: %v\n", err)
			}

			if !restrictiveMode {
				if err := saveStringSetToEncryptedStorage("TELA Search", "NegativeCache", sAll); err != nil {
					logger.Printf("[TELA] Failed storing interrupted negative cache: %v\n", err)
				}
			}

			saveProgress(int(atomic.LoadInt64(&scannedCandidates)), allLen, "", "interrupted")
			results.Text = "  Scan interrupted"
			results.Color = apptheme.StatusTextColor()
			fyne.Do(func() {
				results.Refresh()
				entrySearch.Enable()
				entryAddSCID.Enable()
			})
			logger.Printf("[TELA] Search metrics: outcome=interrupted reason=%s elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", interruptReason, time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
			return
		}

		finalizeStart := time.Now()

		fyne.Do(func() {
			searchMu.Lock()
			searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
			searchMu.Unlock()
			searchData.Set(searching)
			searchList.Refresh()
			results.Show()
			if networkErrorDuringFetch {
				searchMu.RLock()
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d (some apps may be missing - network error during fetch)", len(telaSearch))
				searchMu.RUnlock()
				results.Color = apptheme.StatusTextColor()
			} else {
				searchMu.RLock()
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(telaSearch))
				searchMu.RUnlock()
				results.Color = apptheme.StatusTextColor()
			}
			results.Refresh()
		})
		warmRatings()

		timeNow := time.Now().Format(time.RFC822)
		StoreEncryptedValue("TELA Search", []byte("Last Scan"), []byte(timeNow))
		if gnomon.Index != nil {
			if err := StoreEncryptedValue("TELA Search", []byte("Last Indexed Height"), []byte(strconv.FormatInt(gnomon.Index.LastIndexedHeight, 10))); err != nil {
				cacheIntegrity = "write_failed"
				logger.Printf("[TELA] Failed storing Last Indexed Height: %v\n", err)
			}
		}
		if allLen > 0 && atomic.LoadInt64(&scannedCandidates) >= int64(allLen) {
			saveProgress(allLen, allLen, "", "completed")
			completeTelaScanProgress()
		} else {
			saveProgress(int(atomic.LoadInt64(&scannedCandidates)), allLen, "", "interrupted")
			logger.Printf("[Gnomon] Scan ended before completion: %d/%d\n", atomic.LoadInt64(&scannedCandidates), allLen)
		}

		if storeSCIDs, err := json.Marshal(telaSCIDs); err == nil {
			if err := StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs); err != nil {
				cacheIntegrity = "write_failed"
				logger.Printf("[TELA] Failed storing SCIDs cache: count=%d bytes=%d err=%v\n", len(telaSCIDs), len(storeSCIDs), err)
			}
		} else {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed marshaling SCIDs cache: count=%d err=%v\n", len(telaSCIDs), err)
		}

		if !restrictiveMode {
			if err := saveStringSetToEncryptedStorage("TELA Search", "NegativeCache", sAll); err != nil {
				cacheIntegrity = "write_failed"
				logger.Printf("[TELA] Failed storing negative cache: entries=%d err=%v\n", len(sAll), err)
			}
		}

		if err := saveTelaIndexCache(indexCacheStore); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing INDEX cache: entries=%d err=%v\n", len(indexCacheStore), err)
		}

		searchMu.Lock()
		telaSearch = deduplicateTelaSearch(telaSearch)
		if err := saveTelaDisplayCache(telaDisplayCache(telaSearch)); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing display cache: entries=%d err=%v\n", len(telaSearch), err)
		}
		searchMu.Unlock()

		candidateCacheMu.RLock()
		candidateCacheSnapshot := make(telaCandidateCache, len(candidateCache))
		for scid, meta := range candidateCache {
			candidateCacheSnapshot[scid] = meta
		}
		candidateCacheMu.RUnlock()
		if err := saveTelaCandidateCache(candidateCacheSnapshot); err != nil {
			cacheIntegrity = "write_failed"
			logger.Printf("[TELA] Failed storing candidate cache: entries=%d err=%v\n", len(candidateCacheSnapshot), err)
		}

		if restrictiveMode && len(searching) < 1 {
			errorText.Text = "TELA is in restrictive mode"
			errorText.Color = apptheme.StatusTextColor()
			errorText.Refresh()
		}

		lastScan = timeNow
		labelLastScan.Text = fmt.Sprintf("  %s", lastScan)
		labelLastScan.Color = apptheme.C.Green

		fyne.Do(func() {
			labelLastScan.Refresh()
			entrySearch.Enable()
			entryAddSCID.Enable()
		})
		phaseFinalizeMs = time.Since(finalizeStart).Milliseconds()

		searchMu.RLock()
		displayedSCIDs := make([]string, 0, len(telaSearch))
		seenDisplayed := make(map[string]struct{}, len(telaSearch))
		for _, entry := range telaSearch {
			if entry.SCID == "" {
				continue
			}
			if _, exists := seenDisplayed[entry.SCID]; exists {
				continue
			}
			seenDisplayed[entry.SCID] = struct{}{}
			displayedSCIDs = append(displayedSCIDs, entry.SCID)
		}
		searchMu.RUnlock()
		if !restrictiveMode && len(displayedSCIDs) > 0 {
			telaSCIDs = displayedSCIDs
		}

		logger.Printf("[TELA] Search metrics: outcome=completed elapsed_ms=%d sync_wait_s=%d stored_scids=%d candidates=%d scanned=%d version_hits=%d index_calls=%d retries=%d results=%d filtered_non_displayable=%d filtered_exclusions=%d filtered_min_likes=%d device_class=%s worker_pool=%d ui_refreshes=%d progress_writes=%d pre_dispatch_skips=%d neg_cache_skips=%d prefilter_passed=%d prefilter_dropped=%d cache_hit_mode=%s height_delta=%d full_scan_reason=%s cache_integrity=%s phase_prefilter_ms=%d phase_scan_ms=%d phase_finalize_ms=%d\n", time.Since(scanStart).Milliseconds(), syncWaitSeconds, storedSCIDsCount, allCandidates, atomic.LoadInt64(&scannedCandidates), atomic.LoadInt64(&versionHits), atomic.LoadInt64(&indexInfoCalls), atomic.LoadInt64(&retryCount), len(telaSearch), atomic.LoadInt64(&filteredNonDisplayable), atomic.LoadInt64(&filteredByExclusion), atomic.LoadInt64(&filteredByMinLikes), deviceClass, workerPoolSize, atomic.LoadInt64(&uiRefreshCount), atomic.LoadInt64(&progressWriteCount), atomic.LoadInt64(&preDispatchSkips), atomic.LoadInt64(&negCacheSkips), atomic.LoadInt64(&prefilterPassed), atomic.LoadInt64(&prefilterDropped), cacheHitMode, heightDelta, fullScanReason, cacheIntegrity, phasePrefilterMs, phaseScanMs, phaseFinalizeMs)
		logger.Printf("[TELA] Discovery state: backfillActive=%v backfillFailed=%v lastBackfillHeight=%d gnomonHeight=%d displayed=%d\n",
			telaBackfillActive.Load(), telaBackfillFailed.Load(), lastBackfillHeight, currentHeight, len(displayedSCIDs))
		keepProgressVisible = false

		// Start background pre-warm for TELA apps on mobile to reduce launch latency.
		// Pre-warm up to 3 most recently used apps from history.
		if a.Driver().Device().IsMobile() && len(telaSearch) > 0 {
			go func() {
				time.Sleep(3 * time.Second) // Wait for UI to settle
				if !strings.Contains(session.Domain, ".tela") || globals.Exit_In_Progress {
					return
				}

				// Get recently used SCIDs from history
				var recentSCIDs []string
				historyRaw, err := GetEncryptedValue("TELA History", []byte("RecentSCIDs"))
				if err == nil && len(historyRaw) > 0 {
					json.Unmarshal(historyRaw, &recentSCIDs)
				}

				// Pre-warm up to 3 apps that are in our search results
				preWarmCount := 0
				for _, scid := range recentSCIDs {
					if preWarmCount >= 3 {
						break
					}
					// Check if this SCID is in our current results
					found := false
					for _, entry := range telaSearch {
						if entry.SCID == scid {
							found = true
							break
						}
					}
					if !found {
						continue
					}

					// Check if already served
					alreadyServed := false
					for _, s := range getTelaActiveServers() {
						if s.SCID == scid {
							alreadyServed = true
							break
						}
					}
					if alreadyServed {
						continue
					}

					logger.Printf("[TELA-PREWARM] Pre-warming SCID %s (%d/3)\n", scid, preWarmCount+1)
					if _, err := serveTELACollisionRecovery(scid, session.Daemon); err != nil {
						logger.Printf("[TELA-PREWARM] Failed to pre-warm %s: %v\n", scid, err)
					} else {
						preWarmCount++
					}
				}
				if preWarmCount > 0 {
					logger.Printf("[TELA-PREWARM] Pre-warmed %d apps\n", preWarmCount)
				}
			}()
		}
	}

	scheduleTelaWarmup = func() {
		if !telaWarmupScheduled.CompareAndSwap(false, true) {
			return
		}
		generation := currentWalletGeneration()
		go func() {
			defer telaWarmupScheduled.Store(false)
			time.Sleep(2 * time.Second)
			if globals.Exit_In_Progress || !isWalletGenerationActive(generation) {
				return
			}
			if !strings.Contains(session.Domain, ".tela") {
				return
			}
			if telaNetworkPaused.Load() || telaWorkActive.Load() || telaLaunchPending.Load() {
				return
			}
			maybeStartTelaWork(true)
		}()
	}

	maybeStartTelaWork = func(force bool) {
		if !strings.Contains(session.Domain, ".tela") || globals.Exit_In_Progress {
			return
		}
		// Do not launch while waiting for network to restore.
		if telaNetworkPaused.Load() {
			return
		}
		if telaWorkActive.Load() {
			return
		}
		if telaLaunchPending.Load() {
			return
		}
		if !telaLaunchPending.CompareAndSwap(false, true) {
			return
		}
		go getSearchResults()
	}

	startTelaInitialLoad = func() {
		if len(searching) > 0 || len(telaSearch) > 0 {
			return
		}
		resetTelaProgress()
		showActiveTelaProgress(i18n.T("tela.status_connecting_gnomon"), 0.02, true)
		if refreshAppsList != nil {
			refreshAppsList()
		}
		if gnomon.Index == nil {
			if engram.Disk != nil {
				generation := currentWalletGeneration()
				enableGnomon, _ := getGnomon()
				if enableGnomon == "1" && isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
					go startGnomon()
				}
			}
			maybeStartTelaWork(true)
			return
		}
		maybeStartTelaWork(true)
	}

	searchDebouncer := NewDebouncer(300 * time.Millisecond)

	entrySearch.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		normalizedInput := normalizeTelaSearch(s)

		if s == "" {
			if wSelect.Selected == "Favorites" {
				refreshFavoritesList()
				favoritesList.Refresh()
				if engram.Disk == nil {
					results.Text = "  No wallet connected."
					results.Color = apptheme.C.Gray
				} else if len(favorites) == 0 {
					results.Text = "  No favorites yet."
					results.Color = apptheme.C.Gray
				} else {
					results.Text = fmt.Sprintf("  Favorites:  %d", len(favorites))
					results.Color = apptheme.C.Green
				}
				results.Refresh()
			} else {
				if len(telaSearch) > 0 {
					searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
					_ = searchData.Set(searching)
					if refreshAppsList != nil {
						refreshAppsList()
					}
				} else {
					results.Text = "  No TELA apps loaded."
					results.Color = apptheme.C.Gray
					results.Refresh()
				}
			}
			if !a.Driver().Device().IsMobile() {
				entrySearch.HideCompletion()
			}

			return
		}

		if !a.Driver().Device().IsMobile() {
			if len(s) < 3 {
				entrySearch.SetOptions(append([]string{s}, entrySearchCompletions...))
				entrySearch.ShowCompletion()
			} else {
				entrySearch.HideCompletion()
			}
		}

		if wSelect.Selected == "Favorites" {
			var queryResult []string
			for _, data := range favorites {
				for _, split := range strings.Split(data, ";;;") {
					if strings.Contains(normalizeTelaSearch(split), normalizedInput) {
						queryResult = append(queryResult, data)
						break
					}
				}
			}

			sort.Strings(queryResult)
			favoritesData.Set(queryResult)
			favoritesList.Refresh()
			results.Text = fmt.Sprintf("  Favorites:  %d", len(queryResult))
			results.Color = apptheme.C.Green
			results.Refresh()
			entrySearch.Enable()

			return
		}

		searchDebouncer.Debounce(func() {
			var queryResult []INDEXwithRatings
			query := strings.Split(s, ":")
			if len(query) < 2 {
				if len(s) == 64 {
					for _, ind := range telaSearch {
						if ind.SCID != s {
							continue
						}
						_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
						if err != nil {
							continue
						}
						queryResult = append(queryResult, ind)
						break
					}
				} else {
					for _, ind := range telaSearch {
						data := []string{ind.NameHdr, ind.DescrHdr, ind.DURL, ind.SCID}
						matched := false
						for _, split := range data {
							if strings.Contains(normalizeTelaSearch(split), normalizedInput) {
								matched = true
								break
							}
						}
						if !matched {
							continue
						}
						_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
						if err != nil {
							continue
						}
						queryResult = append(queryResult, ind)
					}
				}

				resultDisplay := telaSearchDisplayAll(queryResult, sortBy, sortDescending)
				fyne.Do(func() {
					searching = resultDisplay
					searchData.Set(searching)
					searchList.Refresh()
					results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(queryResult))
					results.Color = apptheme.StatusTextColor()
					results.Refresh()
					entrySearch.Enable()
				})
				return
			}

			switch normalizeTelaSearch(query[0]) {
			case "name":
				searchMu.RLock()
				snapshot := make([]INDEXwithRatings, len(telaSearch))
				copy(snapshot, telaSearch)
				searchMu.RUnlock()
				for _, ind := range snapshot {
					if !strings.Contains(normalizeTelaSearch(ind.NameHdr), normalizeTelaSearch(query[1])) {
						continue
					}
					_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
					if err != nil {
						continue
					}
					queryResult = append(queryResult, ind)
				}
			case "durl":
				searchMu.RLock()
				snapshot := make([]INDEXwithRatings, len(telaSearch))
				copy(snapshot, telaSearch)
				searchMu.RUnlock()
				for _, ind := range snapshot {
					if !strings.Contains(normalizeTelaSearch(ind.DURL), normalizeTelaSearch(query[1])) {
						continue
					}
					_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
					if err != nil {
						continue
					}
					queryResult = append(queryResult, ind)
				}
			case "my":
				if engram.Disk != nil {
					walletAddr := engram.Disk.GetAddress().String()
					for _, ind := range telaSearch {
						if ind.Author == walletAddr {
							queryResult = append(queryResult, ind)
						}
					}
				}
			case "author":
				if len(query[1]) != 66 {
					return
				}

				_, err := globals.ParseValidateAddress(query[1])
				if err != nil {
					return
				}

				for _, ind := range telaSearch {
					if ind.Author != query[1] {
						continue
					}
					_, _, err := getLikesRatio(ind.SCID, ind.DURL, searchExclusions, minLikes)
					if err != nil {
						continue
					}
					queryResult = append(queryResult, ind)
				}
			default:
				fyne.Do(func() {
					errorText.Text = "unknown search prefix"
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
				})
				return
			}

			resultDisplay := telaSearchDisplayAll(queryResult, sortBy, sortDescending)
			fyne.Do(func() {
				searching = resultDisplay
				searchData.Set(searching)
				searchList.Refresh()
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(queryResult))
				results.Color = apptheme.StatusTextColor()
				results.Refresh()
				entrySearch.Enable()
			})
		})
	}

	entryAddSCID.OnChanged = func(s string) {
		if len(s) == 64 {
			defer entryAddSCID.SetText("")
			bootstrapIndex, err := tela.GetINDEXInfo(s, session.Daemon)
			if err != nil {
				logger.Errorf("[GetINDEXInfo] Bootstrap: %s\n", err)
				errorText.Text = "could not get bootstrap SCID"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			if !strings.HasSuffix(bootstrapIndex.DURL, tela.TAG_BOOTSTRAP) {
				logger.Errorf("[Engram] SCID %s is not a TELA bootstrap INDEX\n", s)
				errorText.Text = "invalid bootstrap SCID"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			storeSCIDs, err := json.Marshal(bootstrapIndex.DOCs)
			if err != nil {
				logger.Errorf("[Engram] Could not marshal bootstrap: %s\n", err)
				errorText.Text = "error initializing bootstrap"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}

			err = StoreEncryptedValue("TELA Search", []byte("SCIDs"), storeSCIDs)
			if err != nil {
				logger.Errorf("[Engram] Could store bootstrap: %s\n", err)
				errorText.Text = "error storing bootstrap"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
			_ = DeleteKey("TELA Search", []byte("NegativeCache"))
			_ = DeleteKey("TELA Search", []byte("IndexCache"))
			_ = DeleteKey("TELA Search", []byte("CandidateCache"))
			_ = DeleteKey("TELA Search", []byte("DisplayCache"))

			telaSCIDs = bootstrapIndex.DOCs
			errorText.Text = "bootstrap initialized"
			errorText.Color = apptheme.C.Green
			errorText.Refresh()

			maybeStartTelaWork(true)
		}
	}

	// Refresh the active server list
	refreshServerList = func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[Engram] refreshServerList panic recovered: %v\n", r)
			}
		}()
		time.Sleep(time.Second * 2)
		var serversRunning []string
		for _, serv := range getTelaActiveServers() {
			serversRunning = append(serversRunning, serv.Name+";;;"+serv.Address+";;;;;;"+serv.SCID)
		}

		sort.Strings(serversRunning)
		fyne.Do(func() {
			servingData.Set(serversRunning)
			servingList.Refresh()
			if refreshAppsList != nil {
				refreshAppsList()
			}
			if !isSearching && wSelect.Selected == "Search" && len(serversRunning) > 0 {
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(searching))
				results.Color = apptheme.StatusTextColor()
				results.Refresh()
			}
		})
	}

	refreshFavoritesList = func() {
		if engram.Disk != nil {
			walletAddress := engram.Disk.GetAddress().String()
			favs, err := GetTELAFavorites(walletAddress)
			if err != nil || len(favs) == 0 {
				favorites = []string{}
				fyne.Do(func() {
					favoritesData.Set(favorites)
				})
			} else {
				favorites = []string{}
				for scid, favData := range favs {
					favorites = append(favorites, favData.Name+";;;"+scid)
				}
				sort.Strings(favorites)
				fyne.Do(func() {
					favoritesData.Set(favorites)
				})
			}
		}
	}

	refreshAppsList = func() {
		searchMu.RLock()
		if len(telaSearch) == 0 {
			searchMu.RUnlock()
			return
		}

		updated := telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
		searchMu.RUnlock()
		fyne.Do(func() {
			searching = updated
			searchData.Set(searching)
			searchList.Refresh()
			if !isSearching && wSelect.Selected == "Search" {
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(searching))
				results.Color = apptheme.StatusTextColor()
				results.Refresh()
			}
		})
	}

	refreshTELA := func() {
		go refreshServerList()
		refreshFavoritesList()
		refreshAppsList()
	}

	btnShutdown.OnTapped = func() {
		switch btnShutdown.Text {
		case "Rescan Blockchain":
			verificationOverlay(
				false,
				i18n.T("tela.browser_header"),
				"Rescan blockchain? This will clear cached results and rescan all TELA apps.",
				"Confirm",
				func(b bool) {
					if b {
						if isSearching {
							return
						}

						clearAllTELACache()
						telaSearch = []INDEXwithRatings{}
						telaSCIDs = []string{}
						sAll = map[string]bool{}
						forceFreshScan = true
						// Reset backfill failure so a rescan can trigger fresh discovery
						telaBackfillFailed.Store(false)
						lastBackfillHeight = 0
						logger.Printf("[TELA] Rescan triggered: reset backfill state\n")
						errorText.Text = ""
						errorText.Refresh()
						go getSearchResults()
					}
				},
			)
		default:
			verificationOverlay(
				false,
				i18n.T("tela.browser_header"),
				"Shutdown all active TELA servers?",
				"Confirm",
				func(b bool) {
					if b {
						tela.ShutdownTELA()
						servingData.Set(nil)
						errorText.Text = ""
						errorText.Refresh()
					}
				},
			)
		}

		go refreshServerList()
	}

	historyBox := container.NewStack(
		rectList,
		historyList,
	)

	searchBox := container.NewStack(
		rectList,
		searchList,
	)

	servingBox := container.NewStack(
		rectList,
		servingList,
	)

	favoritesBox := container.NewStack(
		rectList,
		favoritesList,
	)

	layoutBrowser := container.NewBorder(
		container.NewVBox(
			entryHistory,
			entrySearch,
			entryServeSCID,
			tabButtons,
			statusBox,
		),
		nil,
		nil,
		nil,
		container.NewStack(
			favoritesBox,
			historyBox,
			searchBox,
			servingBox,
		),
	)

	// Hide all alternative views initially
	entrySearch.Hide()
	entryServeSCID.Hide()
	favoritesBox.Hide()
	historyBox.Hide()
	searchBox.Hide()
	servingBox.Hide()

	results.Show()
	results.Refresh()

	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		if favs, _ := GetTELAFavorites(walletAddress); favs != nil && len(favs) > 0 {
			go preIndexFavorites(favs)
		}
	}

	go func() {
		fyne.Do(func() {
			wSelect.SetSelected("Search")
			startTelaInitialLoad()
		})
	}()

	var historyResults []string
	var historyMu sync.Mutex
	var historyLoading bool

	getHistoryResults := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[Engram] getHistoryResults panic recovered: %v\n", r)
			}
		}()
		historyMu.Lock()
		if historyLoading {
			historyMu.Unlock()
			return
		}
		historyLoading = true
		historyMu.Unlock()

		historyResults = nil
		historyData.Set(nil)
		defer func() {
			historyMu.Lock()
			historyLoading = false
			historyMu.Unlock()
		}()

		disk := engram.Disk
		idx := gnomon.Index
		if disk != nil && idx != nil {
			for {
				if !strings.Contains(session.Domain, ".tela") {
					return
				}

				disk = engram.Disk
				idx = gnomon.Index
				if disk == nil || idx == nil {
					return
				}

				if idx.LastIndexedHeight >= int64(disk.Get_Daemon_Height()) {
					break
				}

				fyne.Do(func() {
					entryHistory.Disable()
					results.Refresh()
				})

				time.Sleep(time.Second)
			}

			results.Text = "  Loading launched apps..."
			results.Color = apptheme.StatusTextColor()

			fyne.Do(func() {
				entryHistory.Enable()
				results.Refresh()
			})

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
				scid := crypto.HashHexToHash(string(k))

				title, desc, _, _, _ := getContractHeader(scid)

				if title == "" {
					title = scid.String()
				}

				if desc == "" {
					desc = "N/A"
				}

				historyResults = append(historyResults, title+";;;"+scid.String()+";;;"+desc)
			}

			sort.Strings(historyResults)
			history = historyResults
			historyData.Set(history)

			results.Text = fmt.Sprintf("  Launched Apps:  %d", len(history))
			results.Color = apptheme.C.Green

			fyne.Do(func() {
				historyList.Refresh()
				results.Refresh()
				btnShutdown.Enable()
			})
		}
	}

	entryHistory.OnChanged = func(s string) {
		if s == "" {
			go getHistoryResults()
			return
		}

		normalizedInput := normalizeTelaSearch(s)

		var queryResult []string
		for _, data := range history {
			for _, split := range strings.Split(data, ";;;") {
				if strings.Contains(normalizeTelaSearch(split), normalizedInput) {
					queryResult = append(queryResult, data)
					break
				}
			}
		}

		sort.Strings(queryResult)
		history = queryResult
		historyData.Set(history)

		results.Text = fmt.Sprintf("  %s  %d", i18n.T("files.search_history"), len(queryResult))
		results.Color = apptheme.C.Green
		entryHistory.Enable()

		fyne.Do(func() {
			historyList.Refresh()
			results.Refresh()
		})
	}

	activateTelaSearch = func() {
		errorText.Text = ""
		errorText.Refresh()

		entryHistory.Hide()
		entrySearch.Show()
		entryServeSCID.Hide()
		favoritesBox.Hide()
		historyBox.Hide()
		searchBox.Show()
		servingBox.Hide()
		results.Show()

		telaViewActive.Store(false)
		setTelaStatus("", color.Transparent)
		hideTelaProgress()

		if len(searching) > 0 || len(telaSearch) > 0 {
			if len(searching) == 0 && len(telaSearch) > 0 {
				searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
				_ = searchData.Set(searching)
			}
			if refreshAppsList != nil {
				refreshAppsList()
			} else {
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(searching))
				results.Color = apptheme.StatusTextColor()
				results.Refresh()
			}
			maybeStartTelaWork(true)
			return
		}

		// On re-visit telaSearch is empty because it's a local variable.
		// Load cached display results immediately so apps don't disappear.
		cachedDisplay := loadTelaDisplayCache()
		if len(cachedDisplay) > 0 {
			for _, entry := range cachedDisplay {
				if !isDisplayableTelaApp(entry.INDEX) {
					continue
				}
				telaSearch = append(telaSearch, entry)
			}
			telaSearch = deduplicateTelaSearch(telaSearch)
			if len(telaSearch) > 0 {
				searching = telaSearchDisplayAll(telaSearch, sortBy, sortDescending)
				_ = searchData.Set(searching)
				searchList.Refresh()
				results.Text = fmt.Sprintf(fmt.Sprintf("  %s", i18n.T("tela.app_count"))+"  %d", len(telaSearch))
				results.Color = apptheme.StatusTextColor()
				results.Refresh()
				maybeStartTelaWork(true)
				return
			}
		}

		entrySearch.SetPlaceHolder("Search Apps")
		results.Text = "  No scanned TELA apps yet."
		if forceFreshScan {
			results.Text = "  Resetting TELA results..."
		}
		results.Color = apptheme.C.Gray
		results.Refresh()

		searchList.Refresh()
		maybeStartTelaWork(true)
	}

	wSelect.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()

		// Hide all first
		entryHistory.Hide()
		entrySearch.Hide()
		entryServeSCID.Hide()
		favoritesBox.Hide()
		historyBox.Hide()
		searchBox.Hide()
		servingBox.Hide()

		switch s {
		case "Favorites":
			results.Show()
			telaStatus.Hide()
			refreshTelaStatusBox()
			entrySearch.Show()
			entrySearch.SetPlaceHolder("Search favorites")
			refreshFavoritesList()
			if engram.Disk == nil {
				results.Text = "  No wallet connected."
				results.Color = apptheme.C.Gray
			} else if len(favorites) == 0 {
				results.Text = "  No favorites yet."
				results.Color = apptheme.C.Gray
			} else {
				results.Text = fmt.Sprintf("  Favorites:  %d", len(favorites))
				results.Color = apptheme.C.Green
			}
			results.Refresh()
			favoritesBox.Show()
			favoritesList.Refresh()
		case "History":
			results.Show()
			telaStatus.Hide()
			refreshTelaStatusBox()
			if gnomon.Index == nil {
				results.Text = "  Index is unavailable."
				results.Color = apptheme.C.Gray
				results.Show()
				results.Refresh()
				refreshTelaStatusBox()
			}

			generation := currentWalletGeneration()
			if isWalletGenerationActive(generation) && !globals.Exit_In_Progress {
				go getHistoryResults()
			}

			entryHistory.Show()
			historyBox.Show()
			historyList.Refresh()
			servingList.UnselectAll()
		case "Search":
			results.Show()
			telaStatus.Hide()
			refreshTelaStatusBox()
			activateTelaSearch()
		}
	}

	if session.Offline {
		results.Text = "  Disabled in offline mode."
		results.Color = apptheme.C.Gray
		results.Refresh()
		entryServeSCID.Disable()
		entryAddSCID.Disable()
		btnShutdown.Disable()
	} else if gnomon.Index == nil {
		results.Text = "  Index is unavailable."
		results.Color = apptheme.C.Gray
		results.Refresh()
		entryAddSCID.Disable()
	}

	// Note: activateTelaSearch() is called via wSelect.SetSelected("Search") above
	// We don't call it again here to avoid double execution which causes race conditions on Android

	entryServeSCID.OnChanged = func(s string) {
		errorText.Text = ""
		errorText.Refresh()
		if len(s) == 64 {
			go func() {
				// Create a TELALink to parse and get its ratings for user to verifiy before serving the content
				telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", s)}
				linkPermission, err := AskPermissionForRequestE("Open TELA Link", telaLink)
				if err != nil {
					logger.Errorf("[Engram] Open TELA link: %s\n", err)
					fyne.Do(func() {
						errorText.Text = i18n.T("tela.error_cannot_open")
						errorText.Color = apptheme.C.Red
						errorText.Refresh()
					})

					return
				}

				if linkPermission != xswd.Allow {
					fyne.Do(func() {
						entryServeSCID.SetText("")
					})
					return
				}

				showLoadingOverlay()
				defer func() {
					go refreshServerList()
				}()

				var index tela.INDEX

				// If serving without Gnomon, scid will not end up in history
				if gnomon.Index != nil {
					result := gnomon.GetAllSCIDVariableDetails(s)
					if len(result) == 0 {
						_, err := getTxData(s)
						if err != nil {
							return
						}
					}

					index.NameHdr, index.DescrHdr, _, _, _ = getContractHeader(crypto.HashHexToHash(s))

					if index.NameHdr == "" {
						index.NameHdr = s
					}

					if len(index.NameHdr) > 36 {
						index.NameHdr = index.NameHdr[0:36] + "..."
					}

					if index.DescrHdr == "" {
						index.DescrHdr = "N/A"
					}

					if len(index.DescrHdr) > 40 {
						index.DescrHdr = index.DescrHdr[0:40] + "..."
					}
				}

				entryServeSCID.SetText("")

				if link, err := serveTELAWithStaleRecovery(s, session.Daemon); err == nil {
					if verifyTELAServerIsUp(link) {
						url, err := url.Parse(link)
						if err != nil {
							logger.Errorf("[Engram] TELA URL parse: %s\n", err)
							errorText.Text = i18n.T("tela.error_parse_url")
							errorText.Color = apptheme.C.Red

							fyne.Do(func() {
								errorText.Refresh()
							})

							return // If url is not valid, scid won't be saved in history
						} else {
							pushTELANavigation(s)
							// Manually entered apps get the same dependent-SCID
							// auto-indexing as search-result launches.
							if gnomon.Index != nil {
								AutoIndexDependentSCIDs(s)
							}
							// Guarantee XSWD is running on the correct dual-stack port before opening browser.
							// Skip opening a tab that can never connect when it isn't ready.
							if !EnsureXSWD() {
								logger.Errorf("[TELA] XSWD not ready, cannot open %s\n", link)
								errorText.Text = i18n.T("tela.error_cannot_open")
								errorText.Color = apptheme.C.Red
								errorText.Refresh()
								return
							}
							fyne.CurrentApp().OpenURL(url)
						}
					} else {
						logger.Errorf("[TELA] Server did not come up in time for %s\n", link)
					}

					if gnomon.Index != nil {
						historyResults = append(historyResults, index.NameHdr+";;;"+index.DescrHdr+";;;;;;"+s)
						sort.Strings(historyResults)
						history = historyResults
						historyData.Set(history)

						results.Text = ""

						err = StoreEncryptedValue("TELA History", []byte(s), []byte(""))
						if err != nil {
							logger.Errorf("[Engram] Error saving TELA search result: %s\n", err)
						}
					}
				} else {
					if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
						removeOverlays()

						// Create a TELALink to parse and get its ratings for user to verifiy before serving updated content
						telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", s)}
						linkPermission, err := AskPermissionForRequestE(i18n.T("tela.allow_updated_content"), telaLink)
						if err != nil {
							logger.Errorf("[Engram] Open TELA link: %s\n", err)
							errorText.Text = i18n.T("tela.error_cannot_open")
							errorText.Color = apptheme.C.Red

							fyne.Do(func() {
								errorText.Refresh()
							})

							return
						}

						if linkPermission != xswd.Allow {
							entryServeSCID.SetText("")
							return
						}

						if link, err := serveTELAUpdates(s); err == nil {
							if verifyTELAServerIsUp(link) {
								url, err := url.Parse(link)
								if err != nil {
									logger.Errorf("[Engram] TELA URL parse: %s\n", err)
									errorText.Text = i18n.T("tela.error_parse_url")
									errorText.Color = apptheme.C.Red

									fyne.Do(func() {
										errorText.Refresh()
									})

									return
								} else {
									pushTELANavigation(s)
									// Guarantee XSWD is running on the correct dual-stack port before opening browser.
									// Skip opening a tab that can never connect when it isn't ready.
									if !EnsureXSWD() {
										logger.Errorf("[TELA] XSWD not ready, cannot open %s\n", link)
										errorText.Text = i18n.T("tela.error_cannot_open")
										errorText.Color = apptheme.C.Red
										errorText.Refresh()
										return
									}
									fyne.CurrentApp().OpenURL(url)
								}
							} else {
								logger.Errorf("[TELA] Server did not come up in time for %s\n", link)
							}
						}

						if gnomon.Index != nil {
							historyResults = append(historyResults, index.NameHdr+";;;"+index.DescrHdr+";;;;;;"+s)
							sort.Strings(historyResults)
							history = historyResults
							historyData.Set(history)
							fyne.Do(func() {
								historyList.Refresh()
							})

							results.Text = ""

							err = StoreEncryptedValue("TELA History", []byte(s), []byte(""))
							if err != nil {
								logger.Errorf("[Engram] Error saving TELA search result: %s\n", err)
							}
						}

						return
					}

					logger.Errorf("[Engram] Error serving TELA: %s\n", err)
					errorText.Text = telaErrorToString(err)
					errorText.Color = apptheme.C.Red
				}

				fyne.Do(func() {
					historyList.Refresh()
					errorText.Refresh()
					results.Refresh()
				})

				removeOverlays()
			}()
		}
	}

	historyList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(history[id], ";;;")
		if len(split) < 4 || len(split[3]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		}

		scid := split[3]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
		}

		historyList.UnselectAll()
		historyList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshTELA))
	}

	searchList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(searching[id], ";;;")
		if len(split) < 2 || len(split[1]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		}

		scid := split[1]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
		}

		searchList.UnselectAll()
		searchList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshTELA))
	}

	servingList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()
		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(serving[id], ";;;")
		if len(split) < 4 || len(split[3]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		}

		scid := split[3]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
		}

		servingList.UnselectAll()
		servingList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshTELA))
	}

	favoritesList.OnSelected = func(id widget.ListItemID) {
		errorText.Text = ""
		errorText.Refresh()

		if id < 0 || id >= len(favorites) {
			return
		}

		showLoadingOverlay()
		defer removeOverlays()

		split := strings.Split(favorites[id], ";;;")
		if len(split) < 2 || len(split[1]) != 64 {
			logger.Errorf("[Engram] TELA Invalid SCID from favorites\n")
			errorText.Text = "invalid TELA scid"
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		}

		scid := split[1]
		var index tela.INDEX
		var err error

		cache := loadTelaIndexCache()
		if cached, ok := cache[scid]; ok && len(cached.DOCs) > 0 {
			index = cached
		} else {
			index, err = tela.GetINDEXInfo(scid, session.Daemon)
			if err != nil {
				logger.Errorf("[Engram] GetINDEXInfo from favorites: %s\n", err)
				errorText.Text = "invalid INDEX scid"
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
		}

		favoritesList.UnselectAll()
		favoritesList.FocusLost()
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTELAManager(index, refreshServerList))
	}

	top := container.NewVBox(
		rectSpacer,
		rectSpacer,
		container.NewCenter(
			heading,
		),
		rectSpacer,
		rectSpacer,
	)

	bottom := container.NewStack(
		container.NewVBox(
			rectSpacer,
			container.NewCenter(
				container.New(layout.NewGridLayoutWithColumns(3),
					btnRescanTela,
					btnBack,
					btnSettingsTela,
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
			layoutBrowser,
		),
	)

	// Create TELA background using theme's DarkMatter
	bgOverlay := canvas.NewRectangle(apptheme.C.DarkMatter)
	bgOverlay.SetMinSize(fyne.NewSize(ui.Width, ui.Height))

	layoutWithBg := container.NewStack(
		// res.telaBg, // Background image - temporarily disabled for color testing
		bgOverlay, // Background color only
		layout,
	)

	return NewVScroll(layoutWithBg)
}

// Layout details of a TELA INDEX
func layoutTELAManager(index tela.INDEX, callback func(), autoLaunch ...bool) fyne.CanvasObject {
	shouldLaunch := false
	if len(autoLaunch) > 0 {
		shouldLaunch = autoLaunch[0]
	}

	session.Domain = "app.tela.manager"
	originalCallerDomain := session.LastDomain // Safely capture the original TELA browser content

	var cachedData *TELAFavoriteData
	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		cachedData, _ = GetTELAFavoriteData(walletAddress, index.SCID)
	}

	frame := &iframe{}

	rectBox := canvas.NewRectangle(color.Transparent)
	rectBox.SetMinSize(fyne.NewSize(ui.MaxWidth*0.99, ui.MaxHeight*0.58))

	rectWidth90 := canvas.NewRectangle(color.Transparent)
	rectWidth90.SetMinSize(scalePoint(320, 1))

	rectSpacer := canvas.NewRectangle(color.Transparent)
	rectSpacer.SetMinSize(compactSpacerSize())

	labelName := widget.NewRichText(&widget.TextSegment{
		Text: index.NameHdr,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
		}})
	labelName.Wrapping = fyne.TextWrapWord

	labelDesc := widget.NewRichText(&widget.TextSegment{
		Text: index.DescrHdr,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignCenter,
			ColorName: theme.ColorNameForeground,
			TextStyle: fyne.TextStyle{Bold: false},
		}})
	labelDesc.Wrapping = fyne.TextWrapWord

	labelDURL := canvas.NewText(i18n.T("tela.durl"), apptheme.C.Gray)
	labelDURL.TextSize = scaleFont(14)
	labelDURL.Alignment = fyne.TextAlignCenter
	labelDURL.TextStyle = fyne.TextStyle{Bold: true}

	textDURL := widget.NewRichTextFromMarkdown(index.DURL)
	textDURL.Wrapping = fyne.TextWrapWord

	labelSCID := canvas.NewText(i18n.T("assets.scid"), apptheme.C.Gray)
	labelSCID.TextSize = scaleFont(14)
	labelSCID.Alignment = fyne.TextAlignCenter
	labelSCID.TextStyle = fyne.TextStyle{Bold: true}

	textSCID := widget.NewRichTextFromMarkdown(index.SCID)
	textSCID.Wrapping = fyne.TextWrapWord

	btnViewExplorer := widget.NewButtonWithIcon("", explorerGlobeResource(), func() {
		if engram.Disk.GetNetwork() {
			link, _ := url.Parse("https://explorer.derofoundation.org/tx/" + index.SCID)
			_ = fyne.CurrentApp().OpenURL(link)
		} else {
			link, _ := url.Parse("https://testnetexplorer.derofoundation.org/tx/" + index.SCID)
			_ = fyne.CurrentApp().OpenURL(link)
		}
	})

	btnCopySCID := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(index.SCID)
	})

	labelAuthor := canvas.NewText(i18n.T("assets.author"), apptheme.C.Gray)
	labelAuthor.TextSize = scaleFont(14)
	labelAuthor.Alignment = fyne.TextAlignCenter
	labelAuthor.TextStyle = fyne.TextStyle{Bold: true}

	author := index.Author
	if author == "anon" {
		author = "--"
	}
	textAuthor := widget.NewRichTextFromMarkdown(author)
	textAuthor.Wrapping = fyne.TextWrapWord

	btnMessageAuthor := widget.NewButtonWithIcon("", theme.MailComposeIcon(), func() {
		if index.Author != "" {
			messages.Contact = index.Author
			session.PreviousDomain = session.Domain
			session.LastDomain = session.Window.Content()
			session.Window.Canvas().SetContent(layoutTransition())
			removeOverlays()
			session.Window.Canvas().SetContent(layoutPM())
		}
	})

	btnCopyAuthor := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(index.Author)
	})

	labelStatus := canvas.NewText(i18n.T("tela.app_status"), apptheme.C.Gray)
	labelStatus.TextSize = scaleFont(14)
	labelStatus.Alignment = fyne.TextAlignCenter
	labelStatus.TextStyle = fyne.TextStyle{Bold: true}

	textStatus := canvas.NewText(i18n.T("tela.status_offline"), apptheme.C.Gray)
	textStatus.TextSize = scaleFont(22)
	textStatus.Alignment = fyne.TextAlignCenter
	textStatus.TextStyle = fyne.TextStyle{Bold: true}

	sepWidth := ui.Width * 0.9

	labelSeparator := canvas.NewRectangle(apptheme.C.Gray)
	labelSeparator.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator2 := canvas.NewRectangle(apptheme.C.Gray)
	labelSeparator2.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator3 := canvas.NewRectangle(apptheme.C.Gray)
	labelSeparator3.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator4 := canvas.NewRectangle(apptheme.C.Gray)
	labelSeparator4.SetMinSize(fyne.NewSize(sepWidth, 1))

	labelSeparator5 := canvas.NewRectangle(apptheme.C.Gray)
	labelSeparator5.SetMinSize(fyne.NewSize(sepWidth, 1))

	linkBack := newSizedIconButton(theme.NavigateBackIcon(), func() {
		removeOverlays()
		capture := session.Window.Content()
		session.Window.SetContent(layoutTransition())
		if originalCallerDomain != nil {
			session.Window.SetContent(originalCallerDomain)
		} else {
			session.Window.SetContent(session.LastDomain)
		}
		session.Domain = "app.tela"
		session.LastDomain = capture
		// The callback (e.g. re-opening the dashboard or refreshing the list)
		// touches UI, so it must run on the main goroutine — a bare go func
		// would make layoutTELAManager's callers (villager edit, TELA refresh)
		// rebuild UI off-thread and trip Fyne's thread checks.
		if callback != nil {
			go func() {
				fyne.Do(callback)
			}()
		}
	})

	btnFilesContracts := newSizedIconButton(theme.FolderIcon(), func() {
		session.Domain = "app.tela.manager.files" // Mark as coming from TELA Manager
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutFilesAndContracts())
		removeOverlays()
	})

	btnSettingsTela := newSizedIconButton(theme.SettingsIcon(), func() {
		session.Domain = "app.tela.manager.settings" // Mark as coming from TELA Manager
		session.LastDomain = session.Window.Content()
		session.Window.SetContent(layoutTransition())
		session.Window.SetContent(layoutAppSettings())
		removeOverlays()
	})

	image := canvas.NewImageFromResource(resourceTelaIcon)
	image.SetMinSize(fyne.NewSize(ui.Width*0.3, ui.Width*0.3))
	image.FillMode = canvas.ImageFillContain

	go func() {
		var iconURL string
		if cachedData != nil && cachedData.IconURL != "" && time.Now().Unix()-cachedData.LastUpdated < 3600 {
			iconURL = cachedData.IconURL
		} else {
			_, _, iconURLHdr, _, _ := getContractHeader(crypto.HashHexToHash(index.SCID))
			if iconURLHdr == "" && index.IconHdr != "" {
				iconURLHdr = index.IconHdr
			}
			iconURL = iconURLHdr
		}

		if iconURL != "" {
			if img, err := handleImageURL(index.NameHdr, iconURL, fyne.NewSize(ui.Width*0.3, ui.Width*0.3)); err == nil {
				fyne.Do(func() {
					image.Resource = img.Resource
					image.Refresh()
				})
			} else {
				logger.Errorf("[Engram] Could not validate icon image: %s\n", err)
			}
		}
	}()

	errorText := canvas.NewText(" ", apptheme.C.Green)
	errorText.TextSize = scaleFont(12)
	errorText.Alignment = fyne.TextAlignCenter

	launchProgress := NewSlimProgressBar()
	launchProgress.Hide()

	launchStatus := canvas.NewText("", apptheme.StatusTextColor())
	launchStatus.TextSize = scaleFont(12)
	launchStatus.Alignment = fyne.TextAlignCenter
	launchStatus.Hide()

	spacerStatus := canvas.NewRectangle(color.Transparent)
	spacerStatus.SetMinSize(fyne.NewSize(0, 34))

	linkOpenInBrowser := widget.NewHyperlinkWithStyle(i18n.T("tela.open_browser"), nil, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	linkOpenInBrowser.Hide()
	linkOpenInBrowser.OnTapped = func() {
		params := fmt.Sprintf("tela://open/%s", index.SCID)
		var toggledUpdates bool
		if !tela.UpdatesAllowed() {
			// user has accepted updated content when serving, call AllowUpdates because OpenTELALink returns error on any updated content
			tela.AllowUpdates(true)
			toggledUpdates = true
		}

		link, err := tela.OpenTELALink(params, session.Daemon)
		if toggledUpdates {
			tela.AllowUpdates(false)
		}

		link = cleanTELALink(link)

		if err != nil {
			logger.Errorf("[Engram] handling TELA link: %s\n", err)
			errorText.Text = i18n.T("tela.error_tela_link")
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		}

		if verifyTELAServerIsUp(link) {
			url, err := url.Parse(link)
			if err != nil {
				logger.Errorf("[Engram] TELA URL parse: %s\n", err)
				errorText.Text = i18n.T("tela.error_parse_url")
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
			} else {
				pushTELANavigation(index.SCID)
				// Server may predate an auto-index pass (or a fresh fastsync
				// install): make sure dependent SCIDs are indexed before the
				// app queries Gnomon. Async and idempotent.
				if gnomon.Index != nil {
					AutoIndexDependentSCIDs(index.SCID)
				}
				CleanStaleXSWDConnections()
				// Guarantee XSWD is running on the correct dual-stack port before opening browser.
				// Skip opening a tab that can never connect when it isn't ready.
				if !EnsureXSWD() {
					logger.Errorf("[TELA] XSWD not ready, cannot open %s\n", link)
					errorText.Text = i18n.T("tela.error_cannot_open")
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
					return
				}
				q := url.Query()
				q.Set("_t", fmt.Sprintf("%d", time.Now().UnixMilli()))
				q.Set("_v", fmt.Sprintf("%d", time.Now().UnixNano()))
				url.RawQuery = q.Encode()

				// On mobile, add a small buffer to prevent chrome-error://chromewebdata/
				// This gives the browser time to fully register the server is ready
				if fyne.CurrentApp().Driver().Device().IsMobile() {
					time.Sleep(500 * time.Millisecond)
				}

				fyne.CurrentApp().OpenURL(url)
			}
		} else {
			logger.Errorf("[TELA] Server did not come up in time for %s\n", link)
		}
	}

	btnServer := widget.NewButtonWithIcon(i18n.T("tela.start_app"), theme.MediaPlayIcon(), nil)

	// Check if app is actually running via both SCID and DURL
	appActuallyRunning := false
	for _, s := range getTelaActiveServers() {
		if s.SCID == index.SCID || s.Name == index.DURL {
			appActuallyRunning = true
			break
		}
	}

	// Clean up stale launch state if app is actually running
	// This handles the case where user launches from TELA browser page and then
	// inspects the app while it's loading - we need to show current state, not stale launch state
	if appActuallyRunning {
		telaLaunchingSCIDsGlobal.Lock()
		delete(telaLaunchingSCIDsGlobal.m, index.SCID)
		telaLaunchingSCIDsGlobal.Unlock()

		telaStoppingSCIDsGlobal.Lock()
		delete(telaStoppingSCIDsGlobal.m, index.SCID)
		telaStoppingSCIDsGlobal.Unlock()

		telaLaunchCancelChansGlobal.Lock()
		delete(telaLaunchCancelChansGlobal.m, index.SCID)
		telaLaunchCancelChansGlobal.Unlock()

		telaLaunchStartTimesGlobal.Lock()
		delete(telaLaunchStartTimesGlobal.m, index.SCID)
		telaLaunchStartTimesGlobal.Unlock()
	}

	telaLaunchingSCIDsGlobal.Lock()
	isLaunchingGlobal := telaLaunchingSCIDsGlobal.m[index.SCID]
	telaLaunchingSCIDsGlobal.Unlock()

	telaStoppingSCIDsGlobal.Lock()
	isStoppingGlobal := telaStoppingSCIDsGlobal.m[index.SCID]
	telaStoppingSCIDsGlobal.Unlock()

	if isLaunchingGlobal {
		launchProgress.Show()
		if isStoppingGlobal {
			launchStatus.Text = i18n.T("tela.status_stopping")
		} else {
			launchStatus.Text = i18n.T("tela.status_starting_app")
		}
		launchStatus.Show()
		btnServer.Text = i18n.T("common.cancel")
		btnServer.SetIcon(theme.CancelIcon())

		// Sync UI with existing launch progress
		go func() {
			telaLaunchStartTimesGlobal.Lock()
			startTime, ok := telaLaunchStartTimesGlobal.m[index.SCID]
			telaLaunchStartTimesGlobal.Unlock()
			if !ok {
				return
			}

			const cap = 0.95
			const tau = 10.0
			for {
				telaLaunchingSCIDsGlobal.Lock()
				stillLaunching := telaLaunchingSCIDsGlobal.m[index.SCID]
				telaLaunchingSCIDsGlobal.Unlock()
				if !stillLaunching {
					break
				}

				elapsed := time.Since(startTime).Seconds()
				val := cap * (1.0 - math.Exp(-elapsed/tau))
				if val > cap {
					val = cap
				}

				uiDo(func() {
					if launchProgress != nil && !launchProgress.Hidden {
						launchProgress.SetValue(val)
					}
					if launchStatus != nil && !launchStatus.Hidden {
						telaStoppingSCIDsGlobal.Lock()
						isStopping := telaStoppingSCIDsGlobal.m[index.SCID]
						telaStoppingSCIDsGlobal.Unlock()
						if isStopping {
							launchStatus.Text = i18n.T("tela.status_stopping")
						} else {
							if val < 0.30 {
								launchStatus.Text = i18n.T("tela.status_connecting_node")
							} else if val < 0.60 {
								launchStatus.Text = i18n.T("tela.status_fetching")
							} else if val < 0.85 {
								launchStatus.Text = i18n.T("tela.status_preparing")
							} else {
								launchStatus.Text = i18n.T("tela.status_almost_ready")
							}
						}
					}
				})
				time.Sleep(200 * time.Millisecond)
			}

			uiDo(func() {
				if tela.HasServer(index.DURL) {
					textStatus.Text = i18n.T("tela.status_running")
					textStatus.Color = apptheme.C.Green
					textStatus.Refresh()
					btnServer.Text = i18n.T("tela.shutdown_app")
					btnServer.SetIcon(theme.MediaStopIcon())
					btnServer.Refresh()
					launchProgress.Hide()
					launchStatus.Hide()
					linkOpenInBrowser.Show()
				} else {
					if launchProgress != nil {
						launchProgress.SetValue(launchProgress.value)
						launchProgress.Refresh()
					}
					if launchStatus != nil {
						launchStatus.Text = "Launch Error"
						launchStatus.Color = apptheme.C.Red
						launchStatus.Refresh()
					}
					if btnServer != nil {
						btnServer.Text = i18n.T("tela.start_app")
						btnServer.SetIcon(theme.MediaPlayIcon())
						btnServer.Refresh()
					}
				}
			})
		}()
	} else if tela.HasServer(index.DURL) {
		textStatus.Text = i18n.T("tela.status_running")
		textStatus.Color = apptheme.C.Green
		textStatus.Refresh()
		btnServer.Text = i18n.T("tela.shutdown_app")
		btnServer.Refresh()
		linkOpenInBrowser.Show()
	}

	btnServer.OnTapped = func() {
		telaLaunchingSCIDsGlobal.Lock()
		isLaunching := telaLaunchingSCIDsGlobal.m[index.SCID]
		telaLaunchingSCIDsGlobal.Unlock()

		if isLaunching {
			telaStoppingSCIDsGlobal.Lock()
			telaStoppingSCIDsGlobal.m[index.SCID] = true
			telaStoppingSCIDsGlobal.Unlock()

			telaLaunchCancelChansGlobal.Lock()
			if cancelChan, ok := telaLaunchCancelChansGlobal.m[index.SCID]; ok {
				close(cancelChan)
				delete(telaLaunchCancelChansGlobal.m, index.SCID)
			}
			telaLaunchCancelChansGlobal.Unlock()
			if launchStatus != nil {
				launchStatus.Text = i18n.T("tela.status_stopping")
				launchStatus.Refresh()
			}
			btnServer.SetIcon(theme.ContentCutIcon())
			btnServer.Refresh()
		} else if btnServer.Text == i18n.T("tela.shutdown_app") {
			tela.ShutdownServer(index.DURL)
			errorText.Text = ""
			errorText.Refresh()
			textStatus.Text = i18n.T("tela.status_offline")
			textStatus.Color = apptheme.C.Gray
			textStatus.Refresh()
			btnServer.Text = i18n.T("tela.start_app")
			btnServer.Refresh()
			linkOpenInBrowser.Hide()
			if callback != nil {
				callback()
			}
		} else {
			telaLaunchingSCIDsGlobal.Lock()
			if telaLaunchingSCIDsGlobal.m[index.SCID] {
				telaLaunchingSCIDsGlobal.Unlock()
				return
			}
			telaLaunchingSCIDsGlobal.m[index.SCID] = true
			telaLaunchingSCIDsGlobal.Unlock()

			cancelChan := make(chan struct{})
			telaLaunchCancelChansGlobal.Lock()
			telaLaunchCancelChansGlobal.m[index.SCID] = cancelChan
			telaLaunchCancelChansGlobal.Unlock()

			telaLaunchStartTimesGlobal.Lock()
			telaLaunchStartTimesGlobal.m[index.SCID] = time.Now()
			telaLaunchStartTimesGlobal.Unlock()

			launchProgress.Show()
			launchProgress.SetValue(0)
			launchStatus.Text = i18n.T("tela.status_starting_app")
			launchStatus.Show()
			btnServer.SetText(i18n.T("common.cancel"))
			btnServer.SetIcon(theme.CancelIcon())
			btnServer.Refresh()
			launchProgress.Refresh()
			launchStatus.Refresh()

			progressDone := make(chan struct{})
			progressStart := time.Now()
			var cancelled atomic.Bool
			go func() {
				const cap = 0.95
				const tau = 10.0
				for {
					select {
					case <-progressDone:
						return
					case <-cancelChan:
						cancelled.Store(true)
						return
					case <-time.After(200 * time.Millisecond):
						elapsed := time.Since(progressStart).Seconds()
						val := cap * (1.0 - math.Exp(-elapsed/tau))
						if val > cap {
							val = cap
						}
						uiDo(func() {
							if launchProgress != nil && !launchProgress.Hidden {
								launchProgress.SetValue(val)
							}
							if launchStatus != nil && !launchStatus.Hidden {
								telaStoppingSCIDsGlobal.Lock()
								isStopping := telaStoppingSCIDsGlobal.m[index.SCID]
								telaStoppingSCIDsGlobal.Unlock()
								if isStopping {
									launchStatus.Text = i18n.T("tela.status_stopping")
								} else {
									if val < 0.30 {
										launchStatus.Text = i18n.T("tela.status_connecting_node")
									} else if val < 0.60 {
										launchStatus.Text = i18n.T("tela.status_fetching")
									} else if val < 0.85 {
										launchStatus.Text = i18n.T("tela.status_preparing")
									} else {
										launchStatus.Text = i18n.T("tela.status_almost_ready")
									}
								}
							}
						})
					}
				}
			}()

			cleanupLaunch := func(failed, cancelledLaunch bool) {
				close(progressDone)
				telaLaunchingSCIDsGlobal.Lock()
				delete(telaLaunchingSCIDsGlobal.m, index.SCID)
				telaLaunchingSCIDsGlobal.Unlock()
				telaLaunchCancelChansGlobal.Lock()
				delete(telaLaunchCancelChansGlobal.m, index.SCID)
				telaLaunchCancelChansGlobal.Unlock()
				telaStoppingSCIDsGlobal.Lock()
				delete(telaStoppingSCIDsGlobal.m, index.SCID)
				telaStoppingSCIDsGlobal.Unlock()
				telaLaunchStartTimesGlobal.Lock()
				delete(telaLaunchStartTimesGlobal.m, index.SCID)
				telaLaunchStartTimesGlobal.Unlock()
				uiDo(func() {
					if launchProgress != nil {
						if failed || cancelledLaunch {
							launchProgress.SetValue(launchProgress.value)
							launchProgress.Refresh()
						} else {
							launchProgress.SetValue(1.0)
							launchProgress.Refresh()
						}
					}
					if launchStatus != nil {
						if cancelledLaunch {
							launchStatus.Text = "Cancelled"
							launchStatus.Color = apptheme.C.Gray
						} else if failed {
							launchStatus.Text = "Failed"
							launchStatus.Color = apptheme.C.Red
						} else {
							launchStatus.Text = "Done!"
							launchStatus.Color = apptheme.C.Green
						}
						launchStatus.Refresh()
					}
					time.AfterFunc(400*time.Millisecond, func() {
						uiDo(func() {
							if launchProgress != nil {
								launchProgress.Hide()
							}
							if launchStatus != nil {
								launchStatus.Hide()
							}

							if failed || cancelledLaunch {
								btnServer.Text = i18n.T("tela.start_app")
								btnServer.SetIcon(theme.MediaPlayIcon())
								btnServer.Refresh()
							}
						})
					})
				})
			}

			go func() {
				openURLAfterDelay := func(link string) {
					if verifyTELAServerIsUp(link) {
						CleanStaleXSWDConnections()
						// Guarantee XSWD is running on the correct dual-stack port before opening browser.
						// Skip opening a tab that can never connect when it isn't ready.
						if !EnsureXSWD() {
							logger.Errorf("[TELA] XSWD not ready, cannot open %s\n", link)
							fyne.Do(func() {
								errorText.Text = i18n.T("tela.error_cannot_open")
								errorText.Color = apptheme.C.Red
								errorText.Refresh()
							})
							return
						}

						if u, perr := url.Parse(link); perr == nil {
							q := u.Query()
							q.Set("_t", fmt.Sprintf("%d", time.Now().UnixMilli()))
							q.Set("_v", fmt.Sprintf("%d", time.Now().UnixNano()))
							u.RawQuery = q.Encode()

							// On mobile, add a small buffer to prevent chrome-error://chromewebdata/
							// This gives the browser time to fully register the server is ready
							if fyne.CurrentApp().Driver().Device().IsMobile() {
								time.Sleep(500 * time.Millisecond)
							}

							fyne.CurrentApp().OpenURL(u)
						}
					} else {
						logger.Errorf("[TELA] Server did not come up in time for %s\n", link)
					}
				}

				link, err := serveTELAWithStaleRecovery(index.SCID, session.Daemon, &cancelled)
				if cancelled.Load() {
					if err == nil {
						tela.ShutdownServer(index.SCID)
					}
					cleanupLaunch(false, true)
					return
				}

				if err == nil {
					pushTELANavigation(index.SCID)

					// Auto-index dependent SCIDs (e.g. song registries hardcoded in
					// app JS or stored as contract variables) so apps like DeroBeats
					// can query them through Gnomon. Async and idempotent: repeat
					// launches skip already-indexed dependencies.
					if gnomon.Index != nil {
						AutoIndexDependentSCIDs(index.SCID)
					}

					if err := StoreEncryptedValue("TELA History", []byte(index.SCID), []byte("")); err != nil {
						logger.Errorf("[Engram] Error saving TELA app to history: %s\n", err)
					}

					go openURLAfterDelay(link)

					uiDo(func() {
						textStatus.Text = "   " + i18n.T("tela.status_running")
						textStatus.Color = apptheme.C.Green
						textStatus.Refresh()
						btnServer.Text = i18n.T("tela.shutdown_app")
						btnServer.SetIcon(theme.MediaStopIcon())
						btnServer.Refresh()
						linkOpenInBrowser.Show()
					})

					telaActiveServersGlobal.Lock()
					telaActiveServersGlobal.active[index.SCID] = true
					telaActiveServersGlobal.Unlock()

					cleanupLaunch(false, false)
				} else {
					if strings.Contains(err.Error(), "user defined no updates and content has been updated to") {
						generation := currentWalletGeneration()
						go func() {
							if !isWalletGenerationActive(generation) {
								cleanupLaunch(true, false)
								return
							}

							telaLink := TELALink_Params{TelaLink: fmt.Sprintf("tela://open/%s", index.SCID)}
							linkPermission, permErr := AskPermissionForRequestE(i18n.T("tela.allow_updated_content"), telaLink)
							if permErr != nil {
								logger.Errorf("[Engram] Open TELA link: %s\n", permErr)
								uiDo(func() {
									errorText.Text = i18n.T("tela.error_cannot_open")
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							if linkPermission != xswd.Allow {
								cleanupLaunch(true, false)
								return
							}

							servedLink, serveErr := serveTELAUpdates(index.SCID)
							if serveErr != nil {
								logger.Errorf("[Engram] Error serving TELA: %s\n", serveErr)
								uiDo(func() {
									errorText.Text = telaErrorToString(serveErr)
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								})
								cleanupLaunch(true, false)
								return
							}

							if verifyTELAServerIsUp(servedLink) {
								parsedURL, parseErr := url.Parse(servedLink)
								if parseErr != nil {
									logger.Errorf("[Engram] TELA URL parse: %s\n", parseErr)
									errorText.Text = i18n.T("tela.error_parse_url")
									errorText.Color = apptheme.C.Red
									errorText.Refresh()
								} else {
									pushTELANavigation(index.SCID)
									// Updated content was just re-cloned: rescan for
									// dependent SCIDs (see Start App branch above).
									if gnomon.Index != nil {
										AutoIndexDependentSCIDs(index.SCID)
									}
									CleanStaleXSWDConnections()
									// Guarantee XSWD is running on the correct dual-stack port before opening browser.
									// Skip opening a tab that can never connect when it isn't ready.
									if !EnsureXSWD() {
										logger.Errorf("[TELA] XSWD not ready, cannot open %s\n", servedLink)
										errorText.Text = i18n.T("tela.error_cannot_open")
										errorText.Color = apptheme.C.Red
										errorText.Refresh()
										return
									}
									q := parsedURL.Query()
									q.Set("_t", fmt.Sprintf("%d", time.Now().UnixMilli()))
									q.Set("_v", fmt.Sprintf("%d", time.Now().UnixNano()))
									parsedURL.RawQuery = q.Encode()

									// On mobile, add a small buffer to prevent chrome-error://chromewebdata/
									// This gives the browser time to fully register the server is ready
									if fyne.CurrentApp().Driver().Device().IsMobile() {
										time.Sleep(500 * time.Millisecond)
									}

									fyne.CurrentApp().OpenURL(parsedURL)
								}
							} else {
								logger.Errorf("[TELA] Server did not come up in time for %s\n", servedLink)
							}

							uiDo(func() {
								textStatus.Text = "   " + i18n.T("tela.status_running")
								textStatus.Color = apptheme.C.Green
								textStatus.Refresh()
								btnServer.Text = i18n.T("tela.shutdown_app")
								btnServer.SetIcon(theme.MediaStopIcon())
								btnServer.Refresh()
								linkOpenInBrowser.Show()
							})

							telaActiveServersGlobal.Lock()
							telaActiveServersGlobal.active[index.SCID] = true
							telaActiveServersGlobal.Unlock()

							if saveErr := StoreEncryptedValue("TELA History", []byte(index.SCID), []byte("")); saveErr != nil {
								logger.Errorf("[Engram] Error saving TELA search result: %s\n", saveErr)
							}

							cleanupLaunch(false, false)
						}()
					} else {
						fyne.Do(func() {
							logger.Errorf("[Engram] Error serving TELA: %s\n", err)
							errorText.Text = telaErrorToString(err)
							errorText.Color = apptheme.C.Red
							errorText.Refresh()
						})
						cleanupLaunch(true, false)
					}
				}
			}()
		}
	}

	var ratings tela.Rating_Result
	if cachedData != nil && cachedData.Rating > 0 {
		ratings.Average = cachedData.Rating
	}

	labelRatingAverage := canvas.NewText(fmt.Sprintf("%.1f", ratings.Average), apptheme.C.Account)
	labelRatingAverage.TextSize = scaleFont(24)
	labelRatingAverage.Alignment = fyne.TextAlignCenter
	labelRatingAverage.TextStyle = fyne.TextStyle{Bold: true}

	hexagonImg := canvas.NewImageFromResource(telaHexagonColor(ratings))
	hexagonImg.SetMinSize(fyne.NewSize(80, 86))

	go func() {
		freshRatings, err := tela.GetRating(index.SCID, session.Daemon, 0)
		if err != nil {
			logger.Errorf("[Engram] GetRating: %s\n", err)
			return
		}

		fyne.Do(func() {
			ratings = freshRatings
			labelRatingAverage.Text = fmt.Sprintf("%.1f", ratings.Average)
			labelRatingAverage.Refresh()
			hexagonImg.Resource = telaHexagonColor(ratings)
			hexagonImg.Refresh()
		})

		if engram.Disk != nil && cachedData != nil {
			walletAddr := engram.Disk.GetAddress().String()
			AddTELAFavorite(walletAddr, index.SCID, cachedData.Name, cachedData.Description, cachedData.IconURL, freshRatings.Average)
		}
	}()

	linkTelaRatings := widget.NewHyperlinkWithStyle(i18n.T("tela.view_ratings"), nil, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	linkTelaRatings.OnTapped = func() {
		showLoadingOverlay()
		go func() {
			err := viewTELARatingsOverlay(index.NameHdr, index.SCID)
			if err != nil {
				removeOverlays()
				uiDo(func() {
					errorText.Text = err.Error()
					errorText.Color = apptheme.C.Red
					errorText.Refresh()
				})
			}
		}()
	}

	var favContainer *fyne.Container
	var favCenter *fyne.Container
	var btnFavorite *widget.Button

	btnFavoriteIcon := favsOutlineResource()
	if engram.Disk != nil {
		walletAddress := engram.Disk.GetAddress().String()
		if IsTELAFavorite(walletAddress, index.SCID) {
			btnFavoriteIcon = favsResource()
		}
	}

	btnFavorite = widget.NewButtonWithIcon("", btnFavoriteIcon, func() {
		if engram.Disk == nil {
			errorText.Text = i18n.T("tela.no_wallet")
			errorText.Color = apptheme.C.Red
			errorText.Refresh()
			return
		}

		walletAddress := engram.Disk.GetAddress().String()

		if IsTELAFavorite(walletAddress, index.SCID) {
			err := RemoveTELAFavorite(walletAddress, index.SCID)
			if err != nil {
				errorText.Text = i18n.T("tela.error_rm_favorite")
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
			btnFavorite.SetIcon(favsOutlineResource())
			errorText.Text = i18n.T("tela.removed_favorites")
			errorText.Color = apptheme.C.Green
		} else {
			err := AddTELAFavorite(walletAddress, index.SCID, index.NameHdr, index.DescrHdr, index.IconHdr, ratings.Average)
			if err != nil {
				errorText.Text = i18n.T("tela.error_add_favorite")
				errorText.Color = apptheme.C.Red
				errorText.Refresh()
				return
			}
			btnFavorite.SetIcon(favsResource())
			errorText.Text = i18n.T("tela.added_favorites")
			errorText.Color = apptheme.C.Green
		}
		errorText.Refresh()

		if favContainer != nil {
			favContainer.Refresh()
		}
		if favCenter != nil {
			favCenter.Refresh()
		}

		if callback != nil {
			callback()
		}
	})

	favContainer = container.NewHBox(
		btnFavorite,
	)

	favCenter = container.NewCenter(
		favContainer,
	)

	center := container.NewStack(
		rectBox,
		container.NewVScroll(
			container.NewStack(
				rectWidth90,
				container.NewHBox(
					layout.NewSpacer(),
					container.NewVBox(
						favCenter,
						rectSpacer,
						container.NewCenter(
							image,
						),
						rectSpacer,
						labelName,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							container.NewStack(
								rectWidth90,
								labelDesc,
							),
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator,
						rectSpacer,
						rectSpacer,
						labelStatus,
						rectSpacer,
						container.NewCenter(
							container.NewStack(
								rectWidth90,
								wrapMobileButton(btnServer),
							),
						),
						rectSpacer,
						launchStatus,
						launchProgress,
						rectSpacer,
						textStatus,
						rectSpacer,
						linkOpenInBrowser,
						rectSpacer,
						errorText,
						rectSpacer,
						labelSeparator2,
						rectSpacer,
						rectSpacer,
						container.NewStack(
							container.NewHBox(
								layout.NewSpacer(),
								container.NewStack(
									hexagonImg,
									container.NewCenter(
										labelRatingAverage,
									),
								),
								layout.NewSpacer(),
							),
						),
						rectSpacer,
						rectSpacer,
						rectSpacer,
						rectSpacer,
						container.NewCenter(
							container.NewStack(
								rectWidth90,
								wrapMobileButton(widget.NewButton(i18n.T("tela.rate"), func() {
									rateTELAOverlay(index.NameHdr, index.SCID)
								})),
							),
						),
						rectSpacer,
						rectSpacer,
						container.NewHBox(
							layout.NewSpacer(),
							linkTelaRatings,
							layout.NewSpacer(),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator3,
						rectSpacer,
						rectSpacer,
						labelDURL,
						textDURL,
						rectSpacer,
						rectSpacer,
						labelSeparator4,
						rectSpacer,
						rectSpacer,
						labelAuthor,
						textAuthor,
						container.NewCenter(
							container.NewHBox(
								btnCopyAuthor,
								btnMessageAuthor,
							),
						),
						rectSpacer,
						rectSpacer,
						labelSeparator5,
						rectSpacer,
						rectSpacer,
						labelSCID,
						textSCID,
						container.NewCenter(
							container.NewHBox(
								btnViewExplorer,
								btnCopySCID,
							),
						),
						rectSpacer,
						rectSpacer,
						container.NewStack(
							rectWidth90,
						),
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
				container.New(layout.NewGridLayoutWithColumns(3),
					btnFilesContracts,
					linkBack,
					btnSettingsTela,
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

	go func() {
		time.Sleep(500 * time.Millisecond)

		servers := getTelaActiveServers()

		tempMap := make(map[string]bool)
		for _, s := range servers {
			tempMap[s.SCID] = true
			tempMap[s.Name] = true
		}

		appRunningNow := tempMap[index.SCID] || tempMap[index.DURL]

		currentButtonText := btnServer.Text

		if appRunningNow && currentButtonText != i18n.T("tela.shutdown_app") {
			uiDo(func() {
				launchProgress.Hide()
				launchStatus.Hide()
				textStatus.Text = i18n.T("tela.status_running")
				textStatus.Color = apptheme.C.Green
				textStatus.Refresh()
				btnServer.Text = i18n.T("tela.shutdown_app")
				btnServer.SetIcon(theme.MediaStopIcon())
				btnServer.Refresh()
				linkOpenInBrowser.Show()
			})
		} else if !appRunningNow && currentButtonText == i18n.T("tela.shutdown_app") {
			uiDo(func() {
				textStatus.Text = i18n.T("tela.status_offline")
				textStatus.Color = apptheme.C.Gray
				textStatus.Refresh()
				btnServer.Text = i18n.T("tela.start_app")
				btnServer.SetIcon(theme.MediaPlayIcon())
				btnServer.Refresh()
				linkOpenInBrowser.Hide()
			})
		}
	}()

	vScroll := NewVScroll(layout)
	cachedTelaManagerContent = vScroll

	if shouldLaunch && btnServer.Text == i18n.T("tela.start_app") {
		go func() {
			time.Sleep(100 * time.Millisecond)
			fyne.Do(btnServer.OnTapped)
		}()
	}

	return vScroll
}
