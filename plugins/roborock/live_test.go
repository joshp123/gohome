package roborock

import (
	"context"
	"errors"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type failedWriteConnection struct{ net.Conn }

func (c failedWriteConnection) Write([]byte) (int, error) {
	return 0, errors.New("injected broken pipe")
}

// Read-only integration proof. Credentials and the device address are never stored here.
func TestLiveReadOnly(t *testing.T) {
	bootstrap, host := os.Getenv("ROBOROCK_TEST_BOOTSTRAP"), os.Getenv("ROBOROCK_TEST_HOST")
	if bootstrap == "" || host == "" {
		t.Skip("set ROBOROCK_TEST_BOOTSTRAP and ROBOROCK_TEST_HOST")
	}
	c, err := NewClient(Config{BootstrapFile: bootstrap, IPOverrides: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	devices, err := c.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatal("expected one test vacuum")
	}
	id := devices[0].ID
	c.overrides[id] = host
	defer func() {
		for _, ch := range c.channels {
			ch.Close()
		}
		if c.mqtt != nil {
			c.mqtt.client.Disconnect(0)
		}
	}()
	checkStatus := func() {
		status, err := c.Status(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "" || status.TotalCleaningCount == 0 || status.LastCleanStart.IsZero() || status.LastCleanEnd.IsZero() {
			t.Fatalf("incomplete status: %+v", status)
		}
		t.Logf("state=%s battery=%d error=%s cleaning records=%d latest record complete", status.State, status.BatteryPercent, status.ErrorCode, status.TotalCleaningCount)
	}
	checkStatus()
	original := c.channels[id]
	original.Close()
	checkStatus()
	if c.channels[id] == original {
		t.Fatal("closed channel reused")
	}
	t.Log("reconnected after local socket closed")
	cached := c.channels[id]
	cached.mu.Lock()
	cached.conn = failedWriteConnection{cached.conn}
	cached.mu.Unlock()
	checkStatus()
	if c.channels[id] == cached {
		t.Fatal("failed cached connection reused")
	}
	t.Log("reconnected after write failed on cached connection")
	var workers sync.WaitGroup
	for i := 0; i < 3; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 15; j++ {
				if _, err := c.Status(ctx, id); err != nil {
					t.Error(err)
					return
				}
				time.Sleep(250 * time.Millisecond)
			}
		}()
	}
	workers.Wait()
	if t.Failed() {
		t.FailNow()
	}
	t.Log("concurrent status reads across keepalive succeeded")
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewMetricsCollector(c))
	gather := func(wantData bool) {
		metrics, err := registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		if !wantData && (len(metrics) != 1 || metrics[0].GetName() != "gohome_roborock_scrape_success" || metrics[0].Metric[0].Gauge.GetValue() != 0) {
			t.Fatal("failed scrape exported device values or success")
		}
		for _, family := range metrics {
			if family.GetName() == "gohome_roborock_battery_percent" {
				found = true
			}
		}
		if found != wantData {
			t.Fatalf("device data present=%v, wanted %v", found, wantData)
		}
	}
	gather(true)
	c.channels[id].Close()
	c.overrides[id] = "127.0.0.1"
	gather(false)
	t.Log("failed read exports no previous device values")
	c.cfg.CloudFallback = true
	checkStatus()
	t.Log("live MQTT fallback works with local path unavailable")
	c.cfg.CloudFallback = false
	c.overrides[id] = host
	gather(true)
	t.Log("metrics recover when local path returns")
	mux := http.NewServeMux()
	Plugin{client: c}.RegisterHTTP(mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest("GET", "/roborock/map.png", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("map HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	img, err := png.Decode(recorder.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("map HTTP 200, PNG %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	if output := os.Getenv("ROBOROCK_TEST_MAP"); output != "" {
		f, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}
