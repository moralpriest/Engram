package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
)

// Tier is the response class when a dangerous operation is detected.
// All current operations are TierHardBlock (HardBlock). SC-interaction
// patterns (deploy-via-Transfer, scinvoke-with-deposit) intentionally flow
// through the standard xswd permission dialog and are NOT gated here, so
// legitimate DEX swaps, SC deposits, and SC reads are not blocked.
type Tier int

const (
	TierHardBlock Tier = iota
)

// DangerousOp is a single registry row.
type DangerousOp struct {
	PatternID string
	Methods   []string
	Reason    string
	Tier      Tier
	Inspect   func(raw json.RawMessage) string
}

func formatDeroAtoms(amount uint64) string {
	s := globals.FormatMoney(amount)
	if strings.HasSuffix(s, " DERO") {
		s = strings.TrimSuffix(s, " DERO")
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		s = "0"
	}
	return s
}

func inspectPureBurn(raw json.RawMessage) string {
	var p rpc.Transfer_Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	hasBurn := false
	var burnAmt uint64
	for _, tr := range p.Transfers {
		if tr.Burn > 0 && tr.Amount == 0 {
			hasBurn = true
			burnAmt = tr.Burn
			break
		}
	}
	if !hasBurn {
		return ""
	}
	if p.SC_Code != "" || p.SC_ID != "" || len(p.SC_RPC) > 0 {
		return ""
	}
	return fmt.Sprintf("Burn = %s DERO", formatDeroAtoms(burnAmt))
}

func inspectSeedExport(raw json.RawMessage) string {
	var p rpc.Query_Key_Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	if !strings.Contains(strings.ToLower(p.Key_type), "mnemonic") {
		return ""
	}
	return "key_type = " + p.Key_type
}

var dangerousOps = []DangerousOp{
	{
		PatternID: "burn_transfer_pure",
		Methods:   []string{"Transfer", "transfer", "transfer_split"},
		Reason:    "Token burn via transfer",
		Tier:      TierHardBlock,
		Inspect:   inspectPureBurn,
	},
	{
		PatternID: "seed_export",
		Methods:   []string{"QueryKey", "query_key"},
		Reason:    "Seed phrase export",
		Tier:      TierHardBlock,
		Inspect:   inspectSeedExport,
	},
}

// InspectDangerous returns the first matching DangerousOp or nil.
func InspectDangerous(method string, raw json.RawMessage) (*DangerousOp, string, bool) {
	if len(raw) == 0 {
		return nil, "", false
	}
	for i := range dangerousOps {
		op := &dangerousOps[i]
		matched := false
		for _, m := range op.Methods {
			if m == method {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if detail := op.Inspect(raw); detail != "" {
			return op, detail, true
		}
	}
	return nil, "", false
}

var (
	xswdBlockedCountsMu sync.Mutex
	xswdBlockedCounts   = map[string]int{}
)

func incrementBlockedCount(appId string) int {
	xswdBlockedCountsMu.Lock()
	defer xswdBlockedCountsMu.Unlock()
	xswdBlockedCounts[appId]++
	return xswdBlockedCounts[appId]
}

func resetBlockedCountsForWallet() {
	xswdBlockedCountsMu.Lock()
	defer xswdBlockedCountsMu.Unlock()
	xswdBlockedCounts = make(map[string]int)
}

func tierLabel(t Tier) string {
	if t == TierHardBlock {
		return "HardBlock"
	}
	return ""
}
