package p1_homewizard

import (
	"testing"
	"time"
)

func TestParseTelegramParsesDutchSummerTimestamp(t *testing.T) {
	metrics, ok := ParseTelegram("0-0:1.0.0(260617143002S)\n1-0:1.7.0(00.327*kW)\n1-0:2.7.0(00.000*kW)")
	if !ok {
		t.Fatal("ParseTelegram returned ok=false")
	}
	if metrics.Timestamp == nil {
		t.Fatal("Timestamp is nil")
	}
	if got, want := metrics.Timestamp.UTC().Format(time.RFC3339), "2026-06-17T12:30:02Z"; got != want {
		t.Fatalf("Timestamp UTC = %s, want %s", got, want)
	}
}

func TestParseTelegramParsesDutchWinterTimestamp(t *testing.T) {
	metrics, ok := ParseTelegram("0-0:1.0.0(260117143002W)\n1-0:1.7.0(00.327*kW)\n1-0:2.7.0(00.000*kW)")
	if !ok {
		t.Fatal("ParseTelegram returned ok=false")
	}
	if metrics.Timestamp == nil {
		t.Fatal("Timestamp is nil")
	}
	if got, want := metrics.Timestamp.UTC().Format(time.RFC3339), "2026-01-17T13:30:02Z"; got != want {
		t.Fatalf("Timestamp UTC = %s, want %s", got, want)
	}
}

