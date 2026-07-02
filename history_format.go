package main

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"

	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
)

// History rows are encoded as ";;;"-delimited strings consumed by the list's
// UpdateItem callback (see newHistoryList). The functions below are pure
// (entry in, row out) so they can be unit-tested and benchmarked without a
// wallet, a daemon, or a running Fyne app. They are extracted verbatim from the
// three inline closures that used to live in layoutHistory().

// appendDate appends t as YYYY-MM-DD, byte-identical to
// t.AppendFormat(dst, "2006-01-02"). For the normal 4-digit-year range it uses
// direct digit math; outside it (year <1000 or >9999) it defers to AppendFormat,
// so the output is identical for any date. Gated by TestAppendDateMatchesFormat.
func appendDate(dst []byte, t time.Time) []byte {
	y, m, d := t.Date()
	if y < 1000 || y > 9999 {
		return t.AppendFormat(dst, "2006-01-02")
	}
	return append(dst,
		byte('0'+y/1000%10), byte('0'+y/100%10), byte('0'+y/10%10), byte('0'+y%10),
		'-', byte('0'+int(m)/10), byte('0'+int(m)%10),
		'-', byte('0'+d/10), byte('0'+d%10),
	)
}

// historyStamp formats t as YYYY-MM-DD, byte-identical to t.Format("2006-01-02").
func historyStamp(t time.Time) string {
	var b [10]byte
	return string(appendDate(b[:0], t))
}

// fastMoneyMax bounds the integer fast path of fastFormatMoney. Below it, exact
// integer arithmetic reproduces globals.FormatMoney byte-for-byte: FormatMoney's
// big.Float rounding error stays far under half a unit in the 5th decimal until
// ~9.2e18, so 1e17 leaves a huge margin. Above it we defer to FormatMoney, so the
// result is identical for every uint64. DERO's whole supply is ~2.1e12 atomic
// units, so the fast path covers all real amounts with orders of magnitude spare.
const fastMoneyMax = 100_000_000_000_000_000 // 1e17 atomic units

// appendMoney appends globals.FormatMoney(amount) to dst without the per-call
// big.Float allocation, deferring to FormatMoney for extreme values (fastMoneyMax).
func appendMoney(dst []byte, amount uint64) []byte {
	if amount > fastMoneyMax {
		return append(dst, globals.FormatMoney(amount)...)
	}
	whole := amount / 100000
	frac := amount % 100000
	dst = strconv.AppendUint(dst, whole, 10)
	return append(dst,
		'.',
		byte('0'+frac/10000%10),
		byte('0'+frac/1000%10),
		byte('0'+frac/100%10),
		byte('0'+frac/10%10),
		byte('0'+frac%10),
	)
}

// fastFormatMoney is an allocation-light equivalent of globals.FormatMoney(amount)
// (atomic units -> "<whole>.<5 decimals>"). Equivalence to FormatMoney across the
// full uint64 range is enforced by the gate test TestFastFormatMoneyMatchesGlobals.
func fastFormatMoney(amount uint64) string {
	var b [24]byte
	return string(appendMoney(b[:0], amount))
}

// historyRowNormal formats a non-coinbase transfer. Returns (row, true) to show
// it, ("", false) to skip. It reads none of the fields ProcessPayload populates.
func historyRowNormal(e rpc.Entry) (string, bool) {
	if e.Coinbase {
		return "", false
	}
	// Build the whole ";;;"-delimited row in one stack buffer: a single allocation
	// for the returned string instead of separate amount/height/stamp strings plus
	// a final concat. Output is byte-identical (pinned by TestHistoryRowNormal).
	var sb [128]byte
	buf := sb[:0]
	if e.Incoming {
		buf = append(buf, "Received;;;"...)
		buf = appendMoney(buf, e.Amount)
	} else {
		buf = append(buf, "Sent;;;("...)
		buf = appendMoney(buf, e.Amount)
		buf = append(buf, ')')
	}
	buf = append(buf, ";;;"...)
	buf = strconv.AppendUint(buf, e.Height, 10)
	buf = append(buf, ";;;"...)
	buf = appendDate(buf, e.Time)
	buf = append(buf, ";;;"...)
	buf = append(buf, e.TXID...)
	return string(buf), true
}

// historyRowCoinbase formats a coinbase (network reward) transfer.
func historyRowCoinbase(e rpc.Entry) (string, bool) {
	if !e.Coinbase {
		return "", false
	}
	stamp := historyStamp(e.Time)
	height := strconv.FormatUint(e.Height, 10)
	amount := fastFormatMoney(e.Amount)
	return "Network" + ";;;" + amount + ";;;" + height + ";;;" + stamp + ";;;" + e.TXID, true
}

// clipHistoryField shortens a display field to 10 chars + ".." when it overflows,
// leaving shorter values untouched. Shared by the message row's username and comment.
func clipHistoryField(s string) string {
	if len(s) > 10 {
		return s[0:10] + ".."
	}
	return s
}

// historyRowMessage formats an encrypted-message transfer (dst port 1337). It is
// the only view that reads Payload_RPC, so it is the only one that needs the
// decoded payload. The build loop currently decodes every entry up front
// (streamHistoryRows), so this function assumes Payload_RPC is already populated.
func historyRowMessage(e rpc.Entry) (string, bool) {
	// Only the message view reads the decoded payload, so the decode lives here
	// (guarded) instead of running unconditionally for every transfer in the
	// build loop. Entries that arrive already-decoded (empty Payload) are reused.
	if len(e.Payload) > 0 {
		e.ProcessPayload()
	}
	if !e.Payload_RPC.HasValue(rpc.RPC_COMMENT, rpc.DataString) {
		return "", false
	}
	stamp := historyStamp(e.Time)
	direction := "Received"
	if !e.Incoming {
		direction = "Sent    "
	}
	contact := ""
	username := ""
	if e.Payload_RPC.HasValue(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString) {
		contact = e.Payload_RPC.Value(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataString).(string)
		username = clipHistoryField(contact)
	}
	comment := clipHistoryField(e.Payload_RPC.Value(rpc.RPC_COMMENT, rpc.DataString).(string))
	return direction + ";;;" + username + ";;;" + comment + ";;;" + stamp + ";;;" + e.TXID + ";;;" + contact, true
}

// revealRowCap stops progressive snapshots once the list already holds far more
// than a screenful. Beyond it the caller's final Set delivers the remaining rows,
// so a long history avoids copying ever-larger snapshots it would never paint.
const revealRowCap = 2048

// streamHistoryRows builds the display rows from a transfer set, newest first,
// applying `row` as both filter and formatter. When batch > 0 it calls `reveal`
// with a snapshot of the rows built so far on a geometric cadence (up to
// revealRowCap rows), so the caller can paint partial results progressively. It
// returns the full final slice. No Fyne or UI-thread assumptions live here.
func streamHistoryRows(entries []rpc.Entry, row func(rpc.Entry) (string, bool), batch int, reveal func([]string)) []string {
	rows := make([]string, 0, len(entries))
	next := batch
	for i := len(entries) - 1; i >= 0; i-- { // newest first
		r, ok := row(entries[i])
		if !ok {
			continue
		}
		rows = append(rows, r)

		// Reveal partial results on a geometric cadence (batch, 2*batch, 4*batch, ...):
		// the first screenful paints early; past revealRowCap we stop snapshotting so a
		// long history pays neither the O(n^2) fixed-chunk copy nor huge late snapshots.
		if batch > 0 && reveal != nil && len(rows) >= next && len(rows) <= revealRowCap {
			snap := append([]string(nil), rows...)
			reveal(snap)
			next *= 2
		}
	}
	return rows
}

// newHistoryList builds the bound list widget used by the history screen. The
// caller owns the backing slice (`data`); listData.Set writes through it so the
// screen's OnSelected can read data[id]. Extracted from layoutHistory() so the
// render/model path is constructable in a headless test.
func newHistoryList(data *[]string) (*widget.List, binding.StringList) {
	listData := binding.BindStringList(data)

	rect := canvas.NewRectangle(color.Transparent)
	rect.SetMinSize(fyne.NewSize(ui.Width*0.3, 35))
	rectMid := canvas.NewRectangle(color.Transparent)
	rectMid.SetMinSize(fyne.NewSize(ui.Width*0.35, 35))

	listBox := widget.NewListWithData(listData,
		func() fyne.CanvasObject {
			return container.NewHBox(
				container.NewStack(
					rect,
					widget.NewLabel(""),
				),
				container.NewStack(
					rectMid,
					widget.NewLabel(""),
				),
				container.NewStack(
					rect,
					widget.NewLabel(""),
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

			co.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[0])
			co.(*fyne.Container).Objects[1].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[1])
			co.(*fyne.Container).Objects[2].(*fyne.Container).Objects[1].(*widget.Label).SetText(split[3])
		})

	return listBox, listData
}
