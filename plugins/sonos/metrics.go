package sonos

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric label values for each UPnP transport state. Track titles never become labels.
var transportStateLabels = map[string]string{
	"STOPPED":          "stopped",
	"PLAYING":          "playing",
	"PAUSED_PLAYBACK":  "paused",
	"TRANSITIONING":    "transitioning",
	"NO_MEDIA_PRESENT": "no_media",
}

var (
	scrapeSuccessDesc = prometheus.NewDesc("gohome_sonos_scrape_success",
		"1 if the topology and every zone were read, else 0", nil, nil)
	lastSuccessDesc = prometheus.NewDesc("gohome_sonos_last_success_timestamp_seconds",
		"Time of the last fully successful read (epoch seconds)", nil, nil)
	zoneReachableDesc = prometheus.NewDesc("gohome_sonos_zone_reachable",
		"1 if the zone's coordinator answered every read, else 0", []string{"zone"}, nil)
	transportStateDesc = prometheus.NewDesc("gohome_sonos_transport_state",
		"1 for the zone's current transport state, 0 for the others", []string{"zone", "state"}, nil)
	volumeDesc = prometheus.NewDesc("gohome_sonos_volume",
		"Zone coordinator volume, 0 to 100", []string{"zone"}, nil)
	mutedDesc = prometheus.NewDesc("gohome_sonos_muted",
		"1 if the zone coordinator is muted", []string{"zone"}, nil)
)

// MetricsCollector reads the system on every scrape. A failed read emits no values for
// what it failed to read, so dashboards show a gap rather than a stale reading.
type MetricsCollector struct {
	client      *Client
	lastSuccess time.Time
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	return &MetricsCollector{client: client}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{scrapeSuccessDesc, lastSuccessDesc, zoneReachableDesc, transportStateDesc, volumeDesc, mutedDesc} {
		ch <- d
	}
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	success := c.collectZones(ctx, ch)
	if success {
		c.lastSuccess = time.Now()
	}
	ch <- gauge(scrapeSuccessDesc, boolValue(success))
	if !c.lastSuccess.IsZero() {
		ch <- gauge(lastSuccessDesc, float64(c.lastSuccess.Unix()))
	}
}

func (c *MetricsCollector) collectZones(ctx context.Context, ch chan<- prometheus.Metric) bool {
	zones, err := c.client.Zones(ctx)
	if err != nil {
		log.Printf("sonos topology read failed: %v", err)
		return false
	}
	success := true
	for _, zone := range zones {
		status, err := c.client.Status(ctx, zone)
		if err != nil {
			log.Printf("sonos zone %q read failed: %v", zone.Name, err)
			ch <- gauge(zoneReachableDesc, 0, zone.Name)
			success = false
			continue
		}
		ch <- gauge(zoneReachableDesc, 1, zone.Name)
		for upnpState, label := range transportStateLabels {
			ch <- gauge(transportStateDesc, boolValue(status.TransportState == upnpState), zone.Name, label)
		}
		ch <- gauge(volumeDesc, float64(status.Volume), zone.Name)
		ch <- gauge(mutedDesc, boolValue(status.Muted), zone.Name)
	}
	return success
}

func gauge(desc *prometheus.Desc, value float64, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...)
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
