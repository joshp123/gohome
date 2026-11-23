package brother

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector exports Brother printer consumables, page counters, and jam telemetry.
type MetricsCollector struct {
	client *Client

	mu          sync.Mutex
	lastReadAt  time.Time
	lastError   string
	lastStatus  *PrinterStatus
	scrapeOK    prometheus.Gauge
	lastSuccess prometheus.Gauge

	infoDesc         *prometheus.Desc
	tonerDesc        *prometheus.Desc
	drumDesc         *prometheus.Desc
	pagesDesc        *prometheus.Desc
	duplexPagesDesc  *prometheus.Desc
	coverageDesc     *prometheus.Desc
	jamsTotalDesc    *prometheus.Desc
	jamsByLocDesc    *prometheus.Desc
	replacementsDesc *prometheus.Desc
}

// NewMetricsCollector creates a Prometheus collector backed by client.
func NewMetricsCollector(client *Client) *MetricsCollector {
	printerLabels := []string{"model", "serial"}
	return &MetricsCollector{
		client: client,
		scrapeOK: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_brother_scrape_success",
			Help: "Last Brother printer scrape success (1=ok, 0=error)",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_brother_last_success_timestamp_seconds",
			Help: "Last successful Brother printer read (epoch seconds)",
		}),
		infoDesc: prometheus.NewDesc(
			"gohome_brother_info",
			"Brother printer model, serial, node name, and firmware metadata",
			[]string{"model", "serial", "node_name", "firmware"}, nil,
		),
		tonerDesc: prometheus.NewDesc(
			"gohome_brother_toner_remaining_percent",
			"Estimated remaining toner cartridge life (percent)",
			printerLabels, nil,
		),
		drumDesc: prometheus.NewDesc(
			"gohome_brother_drum_remaining_percent",
			"Estimated remaining drum unit life (percent)",
			printerLabels, nil,
		),
		pagesDesc: prometheus.NewDesc(
			"gohome_brother_pages_total",
			"Total printed pages recorded by the printer",
			printerLabels, nil,
		),
		duplexPagesDesc: prometheus.NewDesc(
			"gohome_brother_pages_duplex_total",
			"Total two-sided printed pages recorded by the printer",
			printerLabels, nil,
		),
		coverageDesc: prometheus.NewDesc(
			"gohome_brother_average_coverage_percent",
			"Average toner coverage per page (percent)",
			printerLabels, nil,
		),
		jamsTotalDesc: prometheus.NewDesc(
			"gohome_brother_paper_jams_total",
			"Total paper jams recorded by the printer",
			printerLabels, nil,
		),
		jamsByLocDesc: prometheus.NewDesc(
			"gohome_brother_paper_jams_by_location_total",
			"Paper jams by mechanical path (tray1, inside, rear, duplex)",
			[]string{"model", "serial", "location"}, nil,
		),
		replacementsDesc: prometheus.NewDesc(
			"gohome_brother_consumable_replacements_total",
			"Lifetime consumable replacement count (toner, drum)",
			[]string{"model", "serial", "part"}, nil,
		),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.scrapeOK.Describe(ch)
	c.lastSuccess.Describe(ch)
	ch <- c.infoDesc
	ch <- c.tonerDesc
	ch <- c.drumDesc
	ch <- c.pagesDesc
	ch <- c.duplexPagesDesc
	ch <- c.coverageDesc
	ch <- c.jamsTotalDesc
	ch <- c.jamsByLocDesc
	ch <- c.replacementsDesc
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), maintenanceRequestTimeout)
	defer cancel()

	status, err := c.client.FetchPrinterStatus(ctx)

	c.mu.Lock()
	if err != nil {
		c.lastError = err.Error()
		c.scrapeOK.Set(0)
		c.scrapeOK.Collect(ch)
		c.lastSuccess.Collect(ch)
		c.mu.Unlock()
		return
	}
	c.lastError = ""
	c.lastReadAt = status.ObservedAt
	c.lastStatus = &status
	c.scrapeOK.Set(1)
	c.lastSuccess.Set(float64(status.ObservedAt.Unix()))
	c.mu.Unlock()

	c.scrapeOK.Collect(ch)
	c.lastSuccess.Collect(ch)

	model := status.ModelName
	serial := status.SerialNumber

	ch <- prometheus.MustNewConstMetric(c.infoDesc, prometheus.GaugeValue, 1, model, serial, status.NodeName, status.FirmwareVersion)
	if status.TonerRemainingPercent != nil {
		ch <- prometheus.MustNewConstMetric(c.tonerDesc, prometheus.GaugeValue, *status.TonerRemainingPercent, model, serial)
	}
	if status.DrumRemainingPercent != nil {
		ch <- prometheus.MustNewConstMetric(c.drumDesc, prometheus.GaugeValue, *status.DrumRemainingPercent, model, serial)
	}
	if status.PageCounter != nil {
		ch <- prometheus.MustNewConstMetric(c.pagesDesc, prometheus.CounterValue, float64(*status.PageCounter), model, serial)
	}
	if status.DuplexPageCounter != nil {
		ch <- prometheus.MustNewConstMetric(c.duplexPagesDesc, prometheus.CounterValue, float64(*status.DuplexPageCounter), model, serial)
	}
	if status.AverageCoveragePercent != nil {
		ch <- prometheus.MustNewConstMetric(c.coverageDesc, prometheus.GaugeValue, *status.AverageCoveragePercent, model, serial)
	}
	if status.TotalPaperJams != nil {
		ch <- prometheus.MustNewConstMetric(c.jamsTotalDesc, prometheus.CounterValue, float64(*status.TotalPaperJams), model, serial)
	}
	if status.JamTray1Count != nil {
		ch <- prometheus.MustNewConstMetric(c.jamsByLocDesc, prometheus.CounterValue, float64(*status.JamTray1Count), model, serial, "tray1")
	}
	if status.JamInsideCount != nil {
		ch <- prometheus.MustNewConstMetric(c.jamsByLocDesc, prometheus.CounterValue, float64(*status.JamInsideCount), model, serial, "inside")
	}
	if status.JamRearCount != nil {
		ch <- prometheus.MustNewConstMetric(c.jamsByLocDesc, prometheus.CounterValue, float64(*status.JamRearCount), model, serial, "rear")
	}
	if status.JamDuplexCount != nil {
		ch <- prometheus.MustNewConstMetric(c.jamsByLocDesc, prometheus.CounterValue, float64(*status.JamDuplexCount), model, serial, "duplex")
	}
	if status.TonerReplaceCount != nil {
		ch <- prometheus.MustNewConstMetric(c.replacementsDesc, prometheus.CounterValue, float64(*status.TonerReplaceCount), model, serial, "toner")
	}
	if status.DrumReplaceCount != nil {
		ch <- prometheus.MustNewConstMetric(c.replacementsDesc, prometheus.CounterValue, float64(*status.DrumReplaceCount), model, serial, "drum")
	}
}
