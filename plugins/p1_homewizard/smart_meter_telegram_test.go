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

func TestParseTelegramParsesPowerFailureEventLog(t *testing.T) {
	raw := "0-0:1.0.0(260117143002W)\n1-0:99.97.0(2)(0-0:96.7.19)(260116191841W)(0000006772*s)(250813114222S)(0000005573*s)\n"
	metrics, ok := ParseTelegram(raw)
	if !ok {
		t.Fatal("ParseTelegram returned ok=false")
	}
	if len(metrics.PowerFailureEvents) != 2 {
		t.Fatalf("len(PowerFailureEvents) = %d, want 2", len(metrics.PowerFailureEvents))
	}
	if got, want := metrics.PowerFailureEvents[0].RestoredAt.UTC().Format(time.RFC3339), "2026-01-16T18:18:41Z"; got != want {
		t.Fatalf("event[0] restored = %s, want %s", got, want)
	}
	if got, want := metrics.PowerFailureEvents[0].DurationSeconds, 6772.0; got != want {
		t.Fatalf("event[0] duration = %v, want %v", got, want)
	}
	if got, want := metrics.PowerFailureEvents[1].RestoredAt.UTC().Format(time.RFC3339), "2025-08-13T09:42:22Z"; got != want {
		t.Fatalf("event[1] restored = %s, want %s", got, want)
	}
	if got, want := metrics.PowerFailureEvents[1].DurationSeconds, 5573.0; got != want {
		t.Fatalf("event[1] duration = %v, want %v", got, want)
	}
}

