package main

import (
	"context"
	"testing"
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

func TestGetHeaderFromVars(t *testing.T) {
	tests := []struct {
		name  string
		vars  map[string]interface{}
		v2Key string
		v1Key string
		want  string
	}{
		{
			name:  "uses v2 key when present",
			vars:  map[string]interface{}{"v2": "6869", "v1": "626f62"},
			v2Key: "v2",
			v1Key: "v1",
			want:  "hi",
		},
		{
			name:  "falls back to v1 key",
			vars:  map[string]interface{}{"v1": "776f726c64"},
			v2Key: "v2",
			v1Key: "v1",
			want:  "world",
		},
		{
			name:  "missing keys return empty",
			vars:  map[string]interface{}{},
			v2Key: "v2",
			v1Key: "v1",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getHeaderFromVars(tt.vars, tt.v2Key, tt.v1Key)
			if got != tt.want {
				t.Fatalf("getHeaderFromVars() = %q, want %q", got, tt.want)
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
