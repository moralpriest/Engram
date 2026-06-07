package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/DEROFDN/engram/i18n"
	"github.com/civilware/tela"
)

func TestDecodeHex(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "valid lowercase hex", in: "68656c6c6f", want: "hello"},
		{name: "valid mixed hex", in: "48656C6C6F", want: "Hello"},
		{name: "invalid hex returns original", in: "not-hex", want: "not-hex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeHex(tt.in)
			if got != tt.want {
				t.Fatalf("decodeHex(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBatchPrefilterTelaVersionsGuards(t *testing.T) {
	t.Run("empty scid list returns empty result", func(t *testing.T) {
		passed, stats, err := batchPrefilterTelaVersions(context.Background(), nil, 1000, 3, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(passed) != 0 {
			t.Fatalf("expected empty passed map, got %d items", len(passed))
		}
		if stats.VersionHits != 0 || stats.Dropped != 0 || stats.Retries != 0 {
			t.Fatalf("expected zero stats, got %+v", stats)
		}
	})

	t.Run("empty rpc pool errors", func(t *testing.T) {
		_, _, err := batchPrefilterTelaVersions(context.Background(), []string{"abcd"}, 1000, 3, nil, nil)
		if err == nil {
			t.Fatal("expected error for empty rpc pool")
		}
		if err.Error() != "rpc pool is empty" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestBatchFetchINDEXesEmpty(t *testing.T) {
	fetched, ratings, invalid, err := batchFetchINDEXes(context.Background(), nil, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fetched) != 0 {
		t.Fatalf("expected empty fetched map, got %d items", len(fetched))
	}
	if len(ratings) != 0 {
		t.Fatalf("expected empty ratings map, got %d items", len(ratings))
	}
	if len(invalid) != 0 {
		t.Fatalf("expected empty invalid map, got %d items", len(invalid))
	}
}

func TestTelaCandidateCacheHelpers(t *testing.T) {
	cache := telaCandidateCache{}
	cache.set("valid-b", telaCandidateValidIndex, 22)
	cache.set("not-tela", telaCandidateNotTela, 22)
	cache.set("invalid", telaCandidateInvalidIndex, 22)
	cache.set("no-docs", telaCandidateNoDocs, 22)
	cache.set("excluded", telaCandidateExcludedByURL, 22)

	valid := cache.validSCIDs()
	if len(valid) != 1 || valid[0] != "valid-b" {
		t.Fatalf("unexpected valid SCIDs: %#v", valid)
	}

	negative := cache.negativeSet()
	for _, scid := range []string{"not-tela", "invalid", "no-docs"} {
		if !negative[scid] {
			t.Fatalf("expected %q in negative set", scid)
		}
	}
	if negative["valid-b"] {
		t.Fatal("did not expect valid candidate in negative set")
	}
	if negative["excluded"] {
		t.Fatal("did not expect settings-dependent exclusion in negative set")
	}
	if meta := cache["valid-b"]; meta.LastCheckedHeight != 22 || meta.Result != telaCandidateValidIndex {
		t.Fatalf("unexpected metadata stored: %+v", meta)
	}
}

func TestBuildINDEXFromVarsErrors(t *testing.T) {
	t.Run("missing C fails", func(t *testing.T) {
		_, err := buildINDEXFromVars("scid", map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error when C is missing")
		}
	})

	t.Run("missing dURL fails", func(t *testing.T) {
		_, err := buildINDEXFromVars("scid", map[string]interface{}{"C": "invalidhex"})
		if err == nil {
			t.Fatal("expected error when dURL is missing")
		}
	})
}

func TestParseTelaListEntry(t *testing.T) {
	tests := []struct {
		input string
		name  string
		scid  string
	}{
		{"Name;;;scid123", "Name", "scid123"},
		{"OnlyName", "OnlyName", ""},
		{";;;scid456", "", "scid456"},
		{"", "", ""},
	}

	for _, tt := range tests {
		n, s := parseTelaListEntry(tt.input)
		if n != tt.name || s != tt.scid {
			t.Errorf("parseTelaListEntry(%q) = %q, %q; want %q, %q", tt.input, n, s, tt.name, tt.scid)
		}
	}
}

func TestNormalizeTelaSearch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Hello  ", "hello"},
		{"WORLD", "world"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := normalizeTelaSearch(tt.input); got != tt.expected {
			t.Errorf("normalizeTelaSearch(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTelaHexagonColor(t *testing.T) {
	if got := telaHexagonColor(tela.Rating_Result{Average: 7.0}); got != resourceTelaHexagonGreen {
		t.Error("expected green for 7.0")
	}
	if got := telaHexagonColor(tela.Rating_Result{Average: 5.0}); got != resourceTelaHexagonYellow {
		t.Error("expected yellow for 5.0")
	}
	if got := telaHexagonColor(tela.Rating_Result{Average: 2.0}); got != resourceTelaHexagonRed {
		t.Error("expected red for 2.0")
	}
	if got := telaHexagonColor(tela.Rating_Result{Average: 0.0, Likes: 0, Dislikes: 1}); got != resourceTelaHexagonRed {
		t.Error("expected red for unrated app with dislikes")
	}
}

func TestSessionDomainToString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"app.wallet", "Dashboard"},
		{"wallet", "Dashboard"},
		{"app.explorer", "Asset Explorer"},
		{"app.tela", "TELA"},
		{"tela.manager", "TELA"},
		{"app.send", "Send"},
	}

	for _, tt := range tests {
		if got := sessionDomainToString(tt.input); got != tt.expected {
			t.Errorf("sessionDomainToString(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNotificationI18nKeysExist(t *testing.T) {
	keys := []string{
		"notification.send_success",
		"notification.send_failed",
		"notification.incoming",
		"settings.enable_notifications",
		"settings.notifications_desc",
	}
	for _, key := range keys {
		got := i18n.T(key)
		if got == key {
			t.Errorf("i18n key %q not found (returned raw key)", key)
		}
		if got == "" {
			t.Errorf("i18n key %q returned empty string", key)
		}
	}
}

func TestFormatNotificationIncoming(t *testing.T) {
	tmpl := i18n.T("notification.incoming")
	got := fmt.Sprintf(tmpl, "1.50000")
	if got == tmpl {
		t.Error("expected formatted string, got raw template")
	}
	if len(got) == 0 {
		t.Error("expected non-empty formatted string")
	}
}

func TestFormatNotificationSendSuccess(t *testing.T) {
	got := i18n.T("notification.send_success")
	if got == "" || got == "notification.send_success" {
		t.Error("notification.send_success key missing or empty")
	}
}

func TestFormatNotificationSendFailed(t *testing.T) {
	got := i18n.T("notification.send_failed")
	if got == "" || got == "notification.send_failed" {
		t.Error("notification.send_failed key missing or empty")
	}
}
