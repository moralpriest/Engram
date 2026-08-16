package main

// Live testnet integration test for the 28-bit registration PoW change.
//
// It exercises the exact flow a user hits when creating a wallet on testnet:
// a fresh wallet is created via walletapi.Create_Encrypted_Wallet_Random (the
// same call the GUI makes), the registerAccount mining loop runs against the
// 28-bit registrationPoWSolved target (faithful copy of the worker goroutine
// in functions.go), and the winner is submitted through
// engram.Disk.SendTransaction to a real testnet daemon.
//
// Requirements:
//   - A testnet daemon reachable at ENGRAM_REG_NODE (default 127.0.0.1:40402).
//     The HF4 build enforces 28 bits from genesis on testnet, so this proves
//     the client's registrations clear the post-fork consensus rule. On a
//     pre-HF4 testnet the 28-bit winner trivially satisfies the 24-bit rule.
//   - ~5-20 minutes of wall time (2^28 expected attempts).
//
// Gated behind ENGRAM_REG_TESTNET=1 so normal `go test ./...` stays fast and
// offline-safe. Run with:
//
//	ENGRAM_REG_TESTNET=1 go test -count=1 -run TestRegistrationTestnetLive -v -timeout 2400s .
//
// Set ENGRAM_REG_BENCH_SECS=N to only mine for N seconds and report the
// hashrate (no daemon or submission needed). The winner tx is saved to
// ENGRAM_REG_OUT (default /tmp/dero-reg-test/winner.hex) when found.

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deroproject/derohe/cryptography/bn256"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/transaction"
	"github.com/deroproject/derohe/walletapi"
)

func TestRegistrationTestnetLive(t *testing.T) {
	if os.Getenv("ENGRAM_REG_TESTNET") != "1" {
		t.Skip("set ENGRAM_REG_TESTNET=1 to run the live testnet registration test")
	}

	benchSecs := 0
	if v := os.Getenv("ENGRAM_REG_BENCH_SECS"); v != "" {
		fmt.Sscanf(v, "%d", &benchSecs)
	}

	endpoint := os.Getenv("ENGRAM_REG_NODE")
	if endpoint == "" {
		endpoint = "127.0.0.1:40402"
	}
	winnerOut := os.Getenv("ENGRAM_REG_OUT")
	if winnerOut == "" {
		winnerOut = "/tmp/dero-reg-test/winner.hex"
	}

	// Mirror main.go's globals setup so walletapi talks to the right network.
	if globals.Arguments == nil {
		globals.Arguments = make(map[string]interface{})
	}
	globals.Arguments["--testnet"] = true
	globals.Arguments["--simulator"] = false
	globals.Arguments["--daemon-address"] = endpoint
	globals.Arguments["--offline"] = false
	globals.InitNetwork() // Config.Name -> "testnet" so the daemon check passes

	if benchSecs == 0 {
		if err := walletapi.Connect(endpoint); err != nil {
			t.Fatalf("walletapi.Connect(%s): %v", endpoint, err)
		}
		if !walletapi.IsDaemonOnline() {
			t.Fatalf("walletapi is not online after Connect(%s)", endpoint)
		}
		t.Logf("connected to testnet daemon %s", endpoint)
	}

	// Fresh wallet, same creation call the GUI uses on a new-account click.
	wd, err := walletapi.Create_Encrypted_Wallet_Random(filepath.Join(t.TempDir(), "reg-testnet.db"), "testpass")
	if err != nil {
		t.Fatalf("Create_Encrypted_Wallet_Random: %v", err)
	}
	engram.Disk = wd
	defer func() { engram.Disk = nil }()
	t.Logf("wallet address: %s", wd.GetAddress().String())

	winner, attempts, elapsed, err := mineRegistration(t, benchSecs)
	if err != nil {
		t.Fatalf("mining failed: %v", err)
	}

	hashRate := float64(0)
	if elapsed.Seconds() > 0 {
		hashRate = float64(attempts) / elapsed.Seconds()
	}
	t.Logf("mining done: %d attempts in %s (~%.0f h/s)", attempts, elapsed, hashRate)

	if benchSecs > 0 || winner == nil {
		if benchSecs > 0 {
			t.Logf("bench mode: not submitting (set ENGRAM_REG_BENCH_SECS=0 to run the full test)")
		}
		return
	}

	hash := winner.GetHash()
	t.Logf("winner %x (zeros=%d)", hash, leadingZeroBits(hash))

	// Persist the winner so a timed-out run can resubmit without re-mining.
	if err := os.MkdirAll(filepath.Dir(winnerOut), 0o755); err != nil {
		t.Logf("warning: could not create %s: %v", filepath.Dir(winnerOut), err)
	} else if err := os.WriteFile(winnerOut, []byte(hex.EncodeToString(winner.Serialize())), 0o644); err != nil {
		t.Logf("warning: could not write %s: %v", winnerOut, err)
	} else {
		t.Logf("saved winner tx hex to %s", winnerOut)
	}

	// Submit through the exact path the GUI uses after mining.
	t.Logf("submitting registration tx %x via engram.Disk.SendTransaction ...", hash)
	if err := engram.Disk.SendTransaction(winner); err != nil {
		t.Fatalf("SendTransaction REJECTED: %v", err)
	}
	t.Logf("SUBMITTED registration tx %x (accepted by daemon)", hash)
}

// mineRegistration replicates the registerAccount worker goroutine from
// functions.go: build the skeleton once, then advance the nonce by one per
// attempt (tmppoint += G) and check the 28-bit registrationPoWSolved target.
// benchSecs > 0 stops all workers after that many seconds and reports.
func mineRegistration(t *testing.T, benchSecs int) (*transaction.Transaction, int64, time.Duration, error) {
	start := time.Now()
	workers := runtime.GOMAXPROCS(0)
	if workers > 1 {
		workers-- // leave a core for the UI thread, same as registerAccount
	}
	if workers < 1 {
		workers = 1
	}
	t.Logf("starting %d mining workers (target %d leading zero bits)", workers, registrationPoWLeadingZeroBits)

	first := engram.Disk.GetRegistrationTX()
	pub := engram.Disk.Get_Keys().Public.G1().String()
	secret := engram.Disk.Get_Keys().Secret.BigInt()

	var activeWorkers int32 = int32(workers)

	successful := make(chan *transaction.Transaction, 1)
	allWorkersDone := make(chan struct{})
	cancel := make(chan struct{})
	var attempts int64
	var wg sync.WaitGroup

	// Progress reporter.
	stopReport := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopReport:
				return
			case <-time.After(15 * time.Second):
				a := atomic.LoadInt64(&attempts)
				t.Logf("  ... %d attempts, ~%.0f h/s total", a, float64(a)/time.Since(start).Seconds())
			}
		}
	}()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				// When the last worker returns (bench deadline or winner found
				// elsewhere), unblock the waiter below.
				if atomic.AddInt32(&activeWorkers, -1) == 0 {
					close(allWorkersDone)
				}
			}()

			var tmppoint bn256.G1
			tmpsecret := crypto.RandomScalar()
			tmppoint.ScalarMult(crypto.G, tmpsecret)

			for {
				select {
				case <-cancel:
					return
				default:
				}
				if benchSecs > 0 && time.Since(start) > time.Duration(benchSecs)*time.Second {
					return
				}

				ltx := *first
				c := crypto.ReducedHash([]byte(fmt.Sprintf("%s%s", pub, tmppoint.String())))
				s := new(big.Int).Mul(c, secret)
				s.Mod(s, bn256.Order)
				s.Add(s, tmpsecret)
				s.Mod(s, bn256.Order)

				crypto.FillBytes(c, ltx.C[:])
				crypto.FillBytes(s, ltx.S[:])

				hash := ltx.GetHash()
				atomic.AddInt64(&attempts, 1)

				if registrationPoWSolved(hash) {
					candidate := ltx
					if candidate.IsRegistrationValid() {
						select {
						case successful <- &candidate:
						default:
						}
					}
					return
				}
				tmpsecret.Add(tmpsecret, big.NewInt(1))
				tmppoint.Add(&tmppoint, crypto.G)
			}
		}()
	}

	// Full run: allow up to 30 min for the 2^28 search. Bench mode: a little
	// more than benchSecs so workers stop via their own deadline first.
	timeout := 30 * time.Minute
	if benchSecs > 0 {
		timeout = time.Duration(benchSecs+60) * time.Second
	}
	var winner *transaction.Transaction
	select {
	case winner = <-successful:
	case <-allWorkersDone:
		// All workers exited without finding a winner. In bench mode this is
		// the expected outcome (they hit the bench deadline); in a full run it
		// means every worker returned without a winner, which should not happen.
		close(stopReport)
		if benchSecs > 0 {
			return nil, atomic.LoadInt64(&attempts), time.Since(start), nil
		}
		return nil, 0, 0, fmt.Errorf("all workers exited without a winner")
	case <-time.After(timeout):
		close(cancel)
		wg.Wait()
		close(stopReport)
		return nil, 0, 0, fmt.Errorf("timed out waiting for a registration winner")
	}
	close(cancel)
	wg.Wait()
	close(stopReport)
	return winner, atomic.LoadInt64(&attempts), time.Since(start), nil
}

func leadingZeroBits(h crypto.Hash) int {
	n := 0
	for _, b := range h {
		for i := 7; i >= 0; i-- {
			if b&(1<<uint(i)) == 0 {
				n++
			} else {
				return n
			}
		}
	}
	return n
}
