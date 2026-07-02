package main

import (
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
)

// Fixed instant so the "2006-01-02" stamp is deterministic.
var testTime = time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

const testStamp = "2024-01-02"

func TestHistoryRowNormal(t *testing.T) {
	cases := []struct {
		name   string
		e      rpc.Entry
		want   string
		wantOK bool
	}{
		{
			name:   "received",
			e:      rpc.Entry{Coinbase: false, Incoming: true, Amount: 150000, Height: 12345, TXID: "tx_recv", Time: testTime},
			want:   "Received;;;" + globals.FormatMoney(150000) + ";;;12345;;;" + testStamp + ";;;tx_recv",
			wantOK: true,
		},
		{
			name:   "sent_is_parenthesized",
			e:      rpc.Entry{Coinbase: false, Incoming: false, Amount: 250000, Height: 7, TXID: "tx_sent", Time: testTime},
			want:   "Sent;;;(" + globals.FormatMoney(250000) + ");;;7;;;" + testStamp + ";;;tx_sent",
			wantOK: true,
		},
		{
			name:   "coinbase_skipped",
			e:      rpc.Entry{Coinbase: true, Incoming: true, Amount: 1, Height: 1, TXID: "cb", Time: testTime},
			want:   "",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := historyRowNormal(tc.e)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("historyRowNormal() = %q,%v; want %q,%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestHistoryRowCoinbase(t *testing.T) {
	cb := rpc.Entry{Coinbase: true, Amount: 600000, Height: 99, TXID: "cbtx", Time: testTime}
	got, ok := historyRowCoinbase(cb)
	want := "Network;;;" + globals.FormatMoney(600000) + ";;;99;;;" + testStamp + ";;;cbtx"
	if !ok || got != want {
		t.Fatalf("historyRowCoinbase() = %q,%v; want %q,true", got, ok, want)
	}
	if _, ok := historyRowCoinbase(rpc.Entry{Coinbase: false, Time: testTime}); ok {
		t.Fatalf("non-coinbase entry should be skipped")
	}
}

// msgEntry builds a message entry with Payload_RPC populated directly (decoded),
// which is what historyRowMessage reads. Empty Payload, so it is robust to the
// optimize-loop later moving the ProcessPayload decode into historyRowMessage
// (the len(Payload)>0 guard will skip it and use this preset value).
func msgEntry(incoming bool, comment, replyback, txid string) rpc.Entry {
	args := rpc.Arguments{
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: comment},
	}
	if replyback != "" {
		args = append(args, rpc.Argument{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: replyback})
	}
	return rpc.Entry{Incoming: incoming, TXID: txid, Time: testTime, Payload_RPC: args}
}

func TestHistoryRowMessage(t *testing.T) {
	// Received, short comment + short replyback (no truncation).
	got, ok := historyRowMessage(msgEntry(true, "hi", "deroAddr", "m1"))
	want := "Received;;;deroAddr;;;hi;;;" + testStamp + ";;;m1;;;deroAddr"
	if !ok || got != want {
		t.Fatalf("message(received) = %q,%v; want %q,true", got, ok, want)
	}

	// Sent ("Sent    " with 4 trailing spaces); long comment + long replyback
	// both truncate username/comment to first 10 chars + "..", but contact (field 6)
	// keeps the full address.
	long := "0123456789ABCDEF"
	trunc := long[0:10] + ".."
	got2, ok2 := historyRowMessage(msgEntry(false, long, long, "m2"))
	want2 := "Sent    ;;;" + trunc + ";;;" + trunc + ";;;" + testStamp + ";;;m2;;;" + long
	if !ok2 || got2 != want2 {
		t.Fatalf("message(sent,long) = %q,%v; want %q,true", got2, ok2, want2)
	}

	// No comment -> skipped.
	if _, ok := historyRowMessage(rpc.Entry{Time: testTime}); ok {
		t.Fatalf("message without comment should be skipped")
	}
}

// TestHistoryRowMessageDecodesPayload exercises the production path: entries from
// Get_Payments_DestinationPort arrive with a raw CBOR Payload and nil Payload_RPC,
// and historyRowMessage must decode it in-place (the optimize-loop hoist). The
// other message tests preset Payload_RPC with empty Payload, so they never hit
// this branch.
func TestHistoryRowMessageDecodesPayload(t *testing.T) {
	payload, err := rpc.Arguments{
		{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(1337)},
		{Name: rpc.RPC_SOURCE_PORT, DataType: rpc.DataUint64, Value: uint64(0)},
		{Name: rpc.RPC_NEEDS_REPLYBACK_ADDRESS, DataType: rpc.DataString, Value: "deroSender"},
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: "gm"},
	}.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	e := rpc.Entry{Incoming: true, TXID: "mp", Time: testTime, Payload: payload}
	got, ok := historyRowMessage(e)
	want := "Received;;;deroSender;;;gm;;;" + testStamp + ";;;mp;;;deroSender"
	if !ok || got != want {
		t.Fatalf("historyRowMessage(raw payload) = %q,%v; want %q,true", got, ok, want)
	}
}

func TestStreamHistoryRows(t *testing.T) {
	// Chain order (oldest first). historyRowNormal drops the coinbase.
	entries := []rpc.Entry{
		{Coinbase: false, Incoming: true, Amount: 100000, Height: 1, TXID: "a", Time: testTime},
		{Coinbase: true, Incoming: true, Amount: 200000, Height: 2, TXID: "cb", Time: testTime},
		{Coinbase: false, Incoming: false, Amount: 300000, Height: 3, TXID: "b", Time: testTime},
	}

	base := streamHistoryRows(entries, historyRowNormal, 0, nil)
	if len(base) != 2 {
		t.Fatalf("want 2 rows (coinbase filtered), got %d: %v", len(base), base)
	}
	// Newest first: height 3 then height 1.
	if !strings.Contains(base[0], ";;;3;;;") || !strings.Contains(base[1], ";;;1;;;") {
		t.Fatalf("expected newest-first (height 3 then 1), got %v", base)
	}

	// The final slice must be invariant to batch size — this is what licenses
	// the optimize-loop changing the reveal cadence.
	for _, batch := range []int{1, 2, 50, 1000} {
		out := streamHistoryRows(entries, historyRowNormal, batch, func([]string) {})
		if len(out) != len(base) {
			t.Fatalf("batch %d changed length: %d vs %d", batch, len(out), len(base))
		}
		for i := range out {
			if out[i] != base[i] {
				t.Fatalf("batch %d changed row %d: %q vs %q", batch, i, out[i], base[i])
			}
		}
	}
}

func TestHistoryListSmoke(t *testing.T) {
	test.NewTempApp(t)

	var data []string
	listBox, listData := newHistoryList(&data)

	rows := []string{
		"Received;;;1.00000;;;5;;;2024-01-02;;;x",
		"Sent;;;(2.00000);;;6;;;2024-01-02;;;y",
	}
	if err := listData.Set(rows); err != nil {
		t.Fatalf("listData.Set: %v", err)
	}
	if listBox.Length() != len(rows) {
		t.Fatalf("listBox.Length() = %d; want %d", listBox.Length(), len(rows))
	}
}

// makeEntries builds n synthetic transfers, most carrying a real CBOR payload so
// that ProcessPayload does production-representative decode work (otherwise the
// benchmark would be blind to the dominant build-path cost).
func makeEntries(n int) []rpc.Entry {
	payload, err := rpc.Arguments{
		{Name: rpc.RPC_DESTINATION_PORT, DataType: rpc.DataUint64, Value: uint64(0)},
		{Name: rpc.RPC_SOURCE_PORT, DataType: rpc.DataUint64, Value: uint64(0)},
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: "payment for services rendered"},
	}.MarshalBinary()
	if err != nil {
		panic(err)
	}
	out := make([]rpc.Entry, n)
	for i := 0; i < n; i++ {
		out[i] = rpc.Entry{
			Height:   uint64(i + 1),
			Incoming: i%2 == 0,
			Amount:   uint64((i + 1) * 137),
			TXID:     "tx" + strconv.Itoa(i),
			Time:     testTime,
			Payload:  payload,
		}
	}
	return out
}

func BenchmarkStreamHistoryRows(b *testing.B) {
	entries := makeEntries(20000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = streamHistoryRows(entries, historyRowNormal, 50, func([]string) {})
	}
}

func BenchmarkHistoryListPopulate(b *testing.B) {
	test.NewApp()
	rows := streamHistoryRows(makeEntries(20000), historyRowNormal, 0, nil)
	var data []string
	_, listData := newHistoryList(&data)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = listData.Set(rows)
	}
}

// TestFastFormatMoneyMatchesGlobals is the correctness gate for the fastFormatMoney
// optimization: it must produce byte-identical output to globals.FormatMoney for
// every uint64. globals.FormatMoney remains the ground truth; this proves the fast
// integer path (and the big.Float fallback above fastMoneyMax) never diverges.
func TestFastFormatMoneyMatchesGlobals(t *testing.T) {
	check := func(a uint64) {
		t.Helper()
		if got, want := fastFormatMoney(a), globals.FormatMoney(a); got != want {
			t.Fatalf("fastFormatMoney(%d) = %q; globals.FormatMoney = %q", a, got, want)
		}
	}

	// Exhaustive low range: every fractional pattern, carry, and leading-zero case.
	for a := uint64(0); a <= 1_000_000; a++ {
		check(a)
	}

	// Many magnitudes of the whole part with edge fractions, up to past total supply
	// and to the fast-path boundary.
	fracs := []uint64{0, 1, 9, 10, 99, 100, 9999, 10000, 50000, 99998, 99999}
	for whole := uint64(1); whole <= 2_000_000_000_000; whole = whole*7 + 3 {
		for _, f := range fracs {
			check(whole*100000 + f)
		}
	}

	// The fast/fallback boundary and the big.Float-only region above it.
	for _, base := range []uint64{
		fastMoneyMax - 8, fastMoneyMax, fastMoneyMax + 8,
		9_000_000_000_000_000_000, 9_223_372_036_854_775_807,
		18_000_000_000_000_000_000, ^uint64(0),
	} {
		for d := uint64(0); d < 9 && base+d >= base; d++ {
			check(base + d)
		}
	}

	// Deterministic random across the full uint64 range.
	r := rand.New(rand.NewSource(20260630))
	for i := 0; i < 1_000_000; i++ {
		check(r.Uint64())
	}
}

// TestAppendDateMatchesFormat is the correctness gate for appendDate: it must
// match t.Format("2006-01-02") byte-for-byte for any date (the fast 4-digit-year
// path and the AppendFormat fallback for edge years alike).
func TestAppendDateMatchesFormat(t *testing.T) {
	check := func(tm time.Time) {
		t.Helper()
		if got, want := string(appendDate(nil, tm)), tm.Format("2006-01-02"); got != want {
			t.Fatalf("appendDate(%v) = %q; Format = %q", tm, got, want)
		}
	}

	// Structured sweep incl. leap years, month/day boundaries, and edge year ranges
	// that exercise the AppendFormat fallback (<1000, >9999).
	for _, y := range []int{1, 99, 999, 1000, 1969, 1970, 2000, 2024, 2025, 9999, 10000, 12024} {
		for mo := 1; mo <= 12; mo++ {
			for _, d := range []int{1, 9, 10, 28, 29, 30, 31} {
				check(time.Date(y, time.Month(mo), d, 13, 14, 15, 0, time.UTC))
			}
		}
	}

	// Deterministic random instants across ~300 years.
	r := rand.New(rand.NewSource(42))
	base := time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < 200_000; i++ {
		check(time.Unix(base+r.Int63n(300*365*24*3600), 0).UTC())
	}
}

// TestHistoryCache pins the cache contract layoutHistory relies on: per-view
// storage, address isolation (one wallet can never paint another's rows),
// whole-wallet eviction on address switch, empty history as a valid hit, and
// clear() dropping everything on wallet close.
func TestHistoryCache(t *testing.T) {
	var c historyCache

	if _, ok := c.get("walletA", "Normal"); ok {
		t.Fatal("empty cache must miss")
	}

	rowsA := []string{"r1", "r2"}
	c.put("walletA", "Normal", rowsA)
	if got, ok := c.get("walletA", "Normal"); !ok || !slices.Equal(got, rowsA) {
		t.Fatalf("get after put = %v,%v; want %v,true", got, ok, rowsA)
	}

	if _, ok := c.get("walletA", "Coinbase"); ok {
		t.Fatal("views must be cached independently")
	}
	c.put("walletA", "Coinbase", []string{"c1"})
	if _, ok := c.get("walletA", "Normal"); !ok {
		t.Fatal("putting a sibling view must not evict")
	}

	if got, ok := c.get("walletB", "Normal"); ok || got != nil {
		t.Fatal("another wallet's address must miss")
	}

	c.put("walletB", "Normal", []string{"x"})
	if _, ok := c.get("walletA", "Normal"); ok {
		t.Fatal("address switch must evict the previous wallet")
	}
	if _, ok := c.get("walletA", "Coinbase"); ok {
		t.Fatal("address switch must evict ALL of the previous wallet's views")
	}

	c.put("walletB", "Messages", nil)
	if got, ok := c.get("walletB", "Messages"); !ok || len(got) != 0 {
		t.Fatal("empty rows must cache as a valid hit")
	}

	c.put("", "Normal", []string{"x"})
	if _, ok := c.get("", "Normal"); ok {
		t.Fatal("empty address must never store or hit")
	}

	c.clear()
	if _, ok := c.get("walletB", "Normal"); ok {
		t.Fatal("clear must evict everything")
	}
}

// TestHistoryCacheConcurrent exercises put/get/clear from many goroutines; it
// exists to fail under `go test -race` if the internal locking ever regresses.
func TestHistoryCacheConcurrent(t *testing.T) {
	var c historyCache
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			addr := "wallet" + strconv.Itoa(n%2)
			for j := 0; j < 500; j++ {
				c.put(addr, "Normal", []string{"r"})
				c.get(addr, "Normal")
				if j%100 == 0 {
					c.clear()
				}
			}
		}(i)
	}
	wg.Wait()
}
