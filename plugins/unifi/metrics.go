package unifi

import (
	"context"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector exports low-cardinality UniFi network health and gateway/AP metrics.
type MetricsCollector struct {
	client *Client

	mu          sync.Mutex
	scrapeOK    prometheus.Gauge
	lastSuccess prometheus.Gauge

	subsystemUpDesc    *prometheus.Desc
	subsystemTxDesc    *prometheus.Desc
	subsystemRxDesc    *prometheus.Desc
	wanLatencyDesc     *prometheus.Desc
	wanUptimeDesc      *prometheus.Desc
	clientsActiveDesc  *prometheus.Desc
	deviceUpDesc       *prometheus.Desc
	deviceCPUDesc      *prometheus.Desc
	deviceMemoryDesc   *prometheus.Desc
	deviceTempDesc     *prometheus.Desc
	deviceUptimeDesc   *prometheus.Desc
	deviceClientsDesc  *prometheus.Desc
	deviceSatisfyDesc  *prometheus.Desc
}

// NewMetricsCollector creates a UniFi Prometheus collector backed by client.
func NewMetricsCollector(client *Client) *MetricsCollector {
	deviceLabels := []string{"site", "device", "model", "type"}
	return &MetricsCollector{
		client: client,
		scrapeOK: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_unifi_scrape_success",
			Help: "Last UniFi controller scrape success (1=ok, 0=error)",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_unifi_last_success_timestamp_seconds",
			Help: "Last successful UniFi controller read (epoch seconds)",
		}),
		subsystemUpDesc: prometheus.NewDesc(
			"gohome_unifi_subsystem_ok",
			"UniFi subsystem health status (1=ok, 0=degraded or error)",
			[]string{"site", "subsystem"}, nil,
		),
		subsystemTxDesc: prometheus.NewDesc(
			"gohome_unifi_subsystem_tx_bytes_per_second",
			"UniFi subsystem transmit throughput in bytes per second",
			[]string{"site", "subsystem"}, nil,
		),
		subsystemRxDesc: prometheus.NewDesc(
			"gohome_unifi_subsystem_rx_bytes_per_second",
			"UniFi subsystem receive throughput in bytes per second",
			[]string{"site", "subsystem"}, nil,
		),
		wanLatencyDesc: prometheus.NewDesc(
			"gohome_unifi_wan_latency_milliseconds",
			"UniFi reported WAN latency in milliseconds",
			[]string{"site"}, nil,
		),
		wanUptimeDesc: prometheus.NewDesc(
			"gohome_unifi_wan_uptime_seconds",
			"UniFi reported WAN uptime in seconds",
			[]string{"site"}, nil,
		),
		clientsActiveDesc: prometheus.NewDesc(
			"gohome_unifi_clients_active",
			"Active clients connected to the UniFi site by connection medium",
			[]string{"site", "connection"}, nil,
		),
		deviceUpDesc: prometheus.NewDesc(
			"gohome_unifi_device_up",
			"Adopted UniFi network device reachability (1=online, 0=offline)",
			deviceLabels, nil,
		),
		deviceCPUDesc: prometheus.NewDesc(
			"gohome_unifi_device_cpu_percent",
			"Adopted UniFi network device CPU utilisation (percent)",
			deviceLabels, nil,
		),
		deviceMemoryDesc: prometheus.NewDesc(
			"gohome_unifi_device_memory_percent",
			"Adopted UniFi network device memory utilisation (percent)",
			deviceLabels, nil,
		),
		deviceTempDesc: prometheus.NewDesc(
			"gohome_unifi_device_temperature_celsius",
			"Adopted UniFi network device general temperature (celsius)",
			deviceLabels, nil,
		),
		deviceUptimeDesc: prometheus.NewDesc(
			"gohome_unifi_device_uptime_seconds",
			"Adopted UniFi network device uptime (seconds)",
			deviceLabels, nil,
		),
		deviceClientsDesc: prometheus.NewDesc(
			"gohome_unifi_device_clients",
			"Active clients served by an adopted UniFi network device",
			deviceLabels, nil,
		),
		deviceSatisfyDesc: prometheus.NewDesc(
			"gohome_unifi_device_satisfaction_percent",
			"UniFi Wi-Fi/experience satisfaction score (percent)",
			deviceLabels, nil,
		),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.scrapeOK.Describe(ch)
	c.lastSuccess.Describe(ch)
	ch <- c.subsystemUpDesc
	ch <- c.subsystemTxDesc
	ch <- c.subsystemRxDesc
	ch <- c.wanLatencyDesc
	ch <- c.wanUptimeDesc
	ch <- c.clientsActiveDesc
	ch <- c.deviceUpDesc
	ch <- c.deviceCPUDesc
	ch <- c.deviceMemoryDesc
	ch <- c.deviceTempDesc
	ch <- c.deviceUptimeDesc
	ch <- c.deviceClientsDesc
	ch <- c.deviceSatisfyDesc
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), unifiRequestTimeout)
	defer cancel()

	summary, err := c.client.FetchSiteSummary(ctx)

	c.mu.Lock()
	if err != nil {
		c.scrapeOK.Set(0)
		c.scrapeOK.Collect(ch)
		c.lastSuccess.Collect(ch)
		c.mu.Unlock()
		return
	}
	c.scrapeOK.Set(1)
	c.lastSuccess.Set(float64(summary.ObservedAt.Unix()))
	c.mu.Unlock()

	c.scrapeOK.Collect(ch)
	c.lastSuccess.Collect(ch)

	site := summary.Site
	ch <- prometheus.MustNewConstMetric(c.clientsActiveDesc, prometheus.GaugeValue, float64(summary.WiredClients), site, "wired")
	ch <- prometheus.MustNewConstMetric(c.clientsActiveDesc, prometheus.GaugeValue, float64(summary.WirelessClients), site, "wireless")

	for _, sub := range summary.Subsystems {
		okVal := 0.0
		if strings.EqualFold(sub.Status, "ok") {
			okVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.subsystemUpDesc, prometheus.GaugeValue, okVal, site, sub.Subsystem)
		if sub.TxBytesPerSecond != nil {
			ch <- prometheus.MustNewConstMetric(c.subsystemTxDesc, prometheus.GaugeValue, *sub.TxBytesPerSecond, site, sub.Subsystem)
		}
		if sub.RxBytesPerSecond != nil {
			ch <- prometheus.MustNewConstMetric(c.subsystemRxDesc, prometheus.GaugeValue, *sub.RxBytesPerSecond, site, sub.Subsystem)
		}
		if sub.Subsystem == "www" || sub.Subsystem == "wan" {
			if sub.WANLatencyMilliseconds != nil {
				ch <- prometheus.MustNewConstMetric(c.wanLatencyDesc, prometheus.GaugeValue, *sub.WANLatencyMilliseconds, site)
			}
			if sub.WANUptimeSeconds != nil {
				ch <- prometheus.MustNewConstMetric(c.wanUptimeDesc, prometheus.GaugeValue, *sub.WANUptimeSeconds, site)
			}
		}
	}

	for _, dev := range summary.Devices {
		upVal := 0.0
		if dev.Online {
			upVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.deviceUpDesc, prometheus.GaugeValue, upVal, site, dev.Name, dev.Model, dev.DeviceType)
		if dev.CPUPercent != nil {
			ch <- prometheus.MustNewConstMetric(c.deviceCPUDesc, prometheus.GaugeValue, *dev.CPUPercent, site, dev.Name, dev.Model, dev.DeviceType)
		}
		if dev.MemoryPercent != nil {
			ch <- prometheus.MustNewConstMetric(c.deviceMemoryDesc, prometheus.GaugeValue, *dev.MemoryPercent, site, dev.Name, dev.Model, dev.DeviceType)
		}
		if dev.TemperatureCelsius != nil {
			ch <- prometheus.MustNewConstMetric(c.deviceTempDesc, prometheus.GaugeValue, *dev.TemperatureCelsius, site, dev.Name, dev.Model, dev.DeviceType)
		}
		if dev.UptimeSeconds != nil {
			ch <- prometheus.MustNewConstMetric(c.deviceUptimeDesc, prometheus.GaugeValue, *dev.UptimeSeconds, site, dev.Name, dev.Model, dev.DeviceType)
		}
		if dev.ActiveClients != nil {
			ch <- prometheus.MustNewConstMetric(c.deviceClientsDesc, prometheus.GaugeValue, float64(*dev.ActiveClients), site, dev.Name, dev.Model, dev.DeviceType)
		}
		if dev.SatisfactionPercent != nil {
			ch <- prometheus.MustNewConstMetric(c.deviceSatisfyDesc, prometheus.GaugeValue, *dev.SatisfactionPercent, site, dev.Name, dev.Model, dev.DeviceType)
		}
	}
}
