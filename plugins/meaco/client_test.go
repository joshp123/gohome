package meaco

import (
	"context"
	"github.com/joshp123/gohome/internal/core"
	meacov1 "github.com/joshp123/gohome/proto/gen/plugins/meaco/v1"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"testing"
)

func TestRecordedJuneStatus(t *testing.T) {
	// Actual local read after the same-value target write on June 17.
	state, err := decodeStatus([]byte(`{"dps":{"2":35,"4":"manual","14":false,"16":45,"17":"cancel","19":0,"101":"cancel"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if state.GetHumidityPercent() != 45 || state.GetTargetHumidityPercent() != 35 || state.GetMode() != "manual" || state.ChildLock == nil || state.GetChildLock() || state.FaultCode == nil || state.GetFaultCode() != 0 || state.GetCountdown() != "cancel" || state.GetOnTimer() != "cancel" {
		t.Fatalf("unexpected state: %v", state)
	}
	if state.Power != nil {
		t.Fatal("missing power must not become false")
	}
	partial, err := decodeStatus([]byte(`{"dps":{"16":46}}`))
	if err != nil {
		t.Fatal(err)
	}
	if partial.TargetHumidityPercent != nil || partial.Mode != nil || partial.FaultCode != nil {
		t.Fatal("partial read must not reuse prior fields")
	}
}

// Run explicitly with MEACO_BOOTSTRAP_FILE pointing to a private decrypted file.
// Read-only by default; same-value target control requires a separate opt-in.
func TestLiveDevice(t *testing.T) {
	path := os.Getenv("MEACO_BOOTSTRAP_FILE")
	if path == "" {
		t.Skip("requires real device bootstrap")
	}
	plugin, _ := NewPlugin(&meacov1.MeacoConfig{BootstrapFile: path}, nil)
	if plugin.configError != nil {
		t.Fatal(plugin.configError)
	}
	state, err := plugin.client.Status(context.Background())
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewMetricsCollector(plugin.client))
	if err != nil {
		metrics, gatherErr := registry.Gather()
		if gatherErr != nil {
			t.Fatal(gatherErr)
		}
		if len(metrics) != 0 {
			t.Fatal("offline device exported measurements")
		}
		if plugin.Health() != core.HealthDegraded {
			t.Fatal("offline device reported healthy")
		}
		t.Fatalf("real device read blocked (retry exported nothing and health is degraded): %v", err)
	}
	t.Logf("real status: %v", state)
	if os.Getenv("MEACO_TEST_SAME_TARGET") == "1" {
		if state.TargetHumidityPercent == nil {
			t.Fatal("no target to preserve")
		}
		_, err := (&service{client: plugin.client}).SetTargetHumidity(context.Background(), &meacov1.SetTargetHumidityRequest{HumidityPercent: *state.TargetHumidityPercent})
		if err != nil {
			t.Fatal(err)
		}
		after, err := plugin.client.Status(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if after.TargetHumidityPercent == nil || *after.TargetHumidityPercent != *state.TargetHumidityPercent {
			t.Fatal("target changed after same-value command")
		}
	}
}
