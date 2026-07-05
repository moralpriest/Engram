package main

import (
	"testing"
	"time"
)

func TestFormatHashrate(t *testing.T) {
	tests := []struct {
		h    float64
		want string
	}{
		{12.3, "12 H/s"},
		{1231.0, "1.231 KH/s"},
		{1230000, "1.230 MH/s"},
		{12345000000, "12.345 GH/s"},
		{12345000000000, "12.345 TH/s"},
	}

	for _, tt := range tests {
		got := formatHashrate(tt.h)
		if got != tt.want {
			t.Errorf("formatHashrate(%f) = %q, want %q", tt.h, got, tt.want)
		}
	}
}

func TestETAWith10MiniSlots(t *testing.T) {
	stats := MiningStats{
		CurrentHashrate: 20000,    // 20 KH/s
		NetHashrate:     12000000, // 12 MH/s
	}

	eta := stats.ETA()
	expected := time.Duration(12000000.0/20000.0*1.8) * time.Second // 1080 seconds = 18 minutes
	if eta != expected {
		t.Errorf("expected ETA to be %s, got %s", expected, eta)
	}
}
