// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Use of this source code in any form is governed by RESEARCH license.
// license that can be found in the LICENSE file.

package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/civilware/tela"
)

var telaNavigationStack struct {
	sync.Mutex
	history []string
}

var telaLaunchingSCIDsGlobal struct {
	sync.Mutex
	m map[string]bool
}

var telaLaunchCancelChansGlobal struct {
	sync.Mutex
	m map[string]chan struct{}
}

var telaActiveServersGlobal struct {
	sync.RWMutex
	info   []tela.ServerInfo
	active map[string]bool
}

var telaStoppingSCIDsGlobal struct {
	sync.Mutex
	m map[string]bool
}

var telaLaunchStartTimesGlobal struct {
	sync.Mutex
	m map[string]time.Time
}

var telaBackfillActive atomic.Bool
var telaBackfillFailed atomic.Bool
var lastBackfillHeight int64

var introShownThisSession bool

var marqueeMu sync.Mutex

func init() {
	telaLaunchingSCIDsGlobal.m = make(map[string]bool)
	telaLaunchCancelChansGlobal.m = make(map[string]chan struct{})
	telaActiveServersGlobal.active = make(map[string]bool)
	telaStoppingSCIDsGlobal.m = make(map[string]bool)
	telaLaunchStartTimesGlobal.m = make(map[string]time.Time)

	go func() {
		for {
			if appExitFlag.Load() {
				return
			}

			servers := tela.GetServerInfo()

			activeMap := make(map[string]bool)
			for _, s := range servers {
				activeMap[s.SCID] = true
			}

			telaActiveServersGlobal.Lock()
			telaActiveServersGlobal.info = servers
			telaActiveServersGlobal.active = activeMap
			telaActiveServersGlobal.Unlock()

			time.Sleep(2 * time.Second)
		}
	}()
}

func isMobileDevice() bool {
	return isMobile()
}

func getTelaActiveServers() []tela.ServerInfo {
	telaActiveServersGlobal.RLock()
	defer telaActiveServersGlobal.RUnlock()

	if telaActiveServersGlobal.info == nil {
		return []tela.ServerInfo{}
	}

	res := make([]tela.ServerInfo, len(telaActiveServersGlobal.info))
	copy(res, telaActiveServersGlobal.info)
	return res
}

func isTelaActive(scid string) bool {
	telaActiveServersGlobal.RLock()
	defer telaActiveServersGlobal.RUnlock()
	return telaActiveServersGlobal.active[scid]
}

func getTelaPath() string {
	return tela.GetPath()
}

func areTelaUpdatesAllowed() bool {
	return tela.UpdatesAllowed()
}

func pushTELANavigation(scid string) {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	telaNavigationStack.history = append(telaNavigationStack.history, scid)
}

func popTELANavigation() string {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	if len(telaNavigationStack.history) == 0 {
		return ""
	}
	index := len(telaNavigationStack.history) - 1
	scid := telaNavigationStack.history[index]
	telaNavigationStack.history = telaNavigationStack.history[:index]
	return scid
}

func hasTELANavigationHistory() bool {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	return len(telaNavigationStack.history) > 0
}

func clearTELANavigationHistory() {
	telaNavigationStack.Lock()
	defer telaNavigationStack.Unlock()
	telaNavigationStack.history = nil
}
