package p1_homewizard

import (
	"context"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/joshp123/gohome/internal/core"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	powerFailureDurationDesc = prometheus.NewDesc(
		"gohome_p1_homewizard_power_failure_duration_seconds",
		"Duration of a long power failure from the DSMR 1-0:99.97.0 event log, timestamped at restoration (seconds)",
		nil,
		nil,
	)
	deviceInfoDesc = prometheus.NewDesc(
		"gohome_p1_homewizard_info",
		"P1 Homewizard meter and dongle firmware metadata",
		[]string{"firmware", "smr_version", "meter_model"},
		nil,
	)
)

// MetricsCollector collects P1 Homewizard power metrics.
type MetricsCollector struct {
	client  *Client
	tariffs Tariffs

	infoMu      sync.Mutex
	cachedInfo  *Info
	infoFetched time.Time

	wifiStrengthPercent prometheus.Gauge
	activePowerW        prometheus.Gauge
	activePowerL1W      prometheus.Gauge
	importPowerW        prometheus.Gauge
	exportPowerW        prometheus.Gauge
	activeCurrentA      prometheus.Gauge
	activeCurrentL1A    prometheus.Gauge
	activeTariff        prometheus.Gauge
	totalImportKWh      prometheus.Gauge
	totalImportT1KWh    prometheus.Gauge
	totalImportT2KWh    prometheus.Gauge
	totalExportKWh      prometheus.Gauge
	totalExportT1KWh    prometheus.Gauge
	totalExportT2KWh    prometheus.Gauge
	voltageSagL1Count   prometheus.Gauge
	voltageSwellL1Count prometheus.Gauge
	anyPowerFailCount   prometheus.Gauge
	longPowerFailCount  prometheus.Gauge

	lastPowerFailTimestamp prometheus.Gauge
	lastPowerFailDuration  prometheus.Gauge

	tariffImportT1  prometheus.Gauge
	tariffImportT2  prometheus.Gauge
	tariffExportT1  prometheus.Gauge
	tariffExportT2  prometheus.Gauge
	costConfigured  prometheus.Gauge
	importCostEUR   prometheus.Gauge
	exportCreditEUR prometheus.Gauge
	totalCostEUR    prometheus.Gauge

	lastUpdated     prometheus.Gauge
	lastSuccess     prometheus.Gauge
	success         prometheus.Gauge
	telegramSuccess prometheus.Gauge
}

func NewMetricsCollector(client *Client, tariffs Tariffs) *MetricsCollector {
	return &MetricsCollector{
		client:  client,
		tariffs: tariffs,
		wifiStrengthPercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_wifi_strength_percent",
			Help: "Wi-Fi signal strength in percent",
		}),
		activePowerW: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_active_power_w",
			Help: "Active power (net) in watts",
		}),
		activePowerL1W: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_active_power_l1_w",
			Help: "Active power L1 in watts",
		}),
		importPowerW: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_import_power_w",
			Help: "Instant import power in watts (from telegram)",
		}),
		exportPowerW: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_export_power_w",
			Help: "Instant export power in watts (from telegram)",
		}),
		activeCurrentA: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_active_current_a",
			Help: "Active current in amps",
		}),
		activeCurrentL1A: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_active_current_l1_a",
			Help: "Active current L1 in amps",
		}),
		activeTariff: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_active_tariff",
			Help: "Active tariff (1 or 2)",
		}),
		totalImportKWh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_import_kwh",
			Help: "Total imported energy (kWh)",
		}),
		totalImportT1KWh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_import_t1_kwh",
			Help: "Total imported energy tariff 1 (kWh)",
		}),
		totalImportT2KWh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_import_t2_kwh",
			Help: "Total imported energy tariff 2 (kWh)",
		}),
		totalExportKWh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_export_kwh",
			Help: "Total exported energy (kWh)",
		}),
		totalExportT1KWh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_export_t1_kwh",
			Help: "Total exported energy tariff 1 (kWh)",
		}),
		totalExportT2KWh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_export_t2_kwh",
			Help: "Total exported energy tariff 2 (kWh)",
		}),
		voltageSagL1Count: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_voltage_sag_l1_count",
			Help: "Voltage sag count L1",
		}),
		voltageSwellL1Count: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_voltage_swell_l1_count",
			Help: "Voltage swell count L1",
		}),
		anyPowerFailCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_any_power_fail_count",
			Help: "Total power fail count",
		}),
		longPowerFailCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_long_power_fail_count",
			Help: "Long power fail count",
		}),
		lastPowerFailTimestamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_last_power_failure_timestamp_seconds",
			Help: "Restoration timestamp of the latest long power failure from DSMR 1-0:99.97.0 (epoch seconds)",
		}),
		lastPowerFailDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_last_power_failure_duration_seconds",
			Help: "Duration of the latest long power failure from DSMR 1-0:99.97.0 (seconds)",
		}),
		tariffImportT1: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_tariff_import_t1_eur_per_kwh",
			Help: "Configured import tariff 1 (EUR/kWh)",
		}),
		tariffImportT2: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_tariff_import_t2_eur_per_kwh",
			Help: "Configured import tariff 2 (EUR/kWh)",
		}),
		tariffExportT1: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_tariff_export_t1_eur_per_kwh",
			Help: "Configured export tariff 1 (EUR/kWh)",
		}),
		tariffExportT2: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_tariff_export_t2_eur_per_kwh",
			Help: "Configured export tariff 2 (EUR/kWh)",
		}),
		costConfigured: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_cost_configured",
			Help: "Whether cost tariffs are configured (1=yes, 0=no)",
		}),
		importCostEUR: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_import_cost_eur",
			Help: "Total import cost (EUR)",
		}),
		exportCreditEUR: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_export_credit_eur",
			Help: "Total export credit (EUR)",
		}),
		totalCostEUR: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_total_cost_eur",
			Help: "Total net cost (EUR)",
		}),
		lastUpdated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_last_update_timestamp_seconds",
			Help: "Last update timestamp (epoch seconds)",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_last_success_timestamp_seconds",
			Help: "Last successful scrape timestamp (epoch seconds)",
		}),
		success: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
		telegramSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_p1_homewizard_telegram_success",
			Help: "Last telegram parse success (1=ok, 0=error)",
		}),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.wifiStrengthPercent.Describe(ch)
	c.activePowerW.Describe(ch)
	c.activePowerL1W.Describe(ch)
	c.importPowerW.Describe(ch)
	c.exportPowerW.Describe(ch)
	c.activeCurrentA.Describe(ch)
	c.activeCurrentL1A.Describe(ch)
	c.activeTariff.Describe(ch)
	c.totalImportKWh.Describe(ch)
	c.totalImportT1KWh.Describe(ch)
	c.totalImportT2KWh.Describe(ch)
	c.totalExportKWh.Describe(ch)
	c.totalExportT1KWh.Describe(ch)
	c.totalExportT2KWh.Describe(ch)
	c.voltageSagL1Count.Describe(ch)
	c.voltageSwellL1Count.Describe(ch)
	c.anyPowerFailCount.Describe(ch)
	ch <- powerFailureDurationDesc
	ch <- deviceInfoDesc
	c.lastPowerFailTimestamp.Describe(ch)
	c.lastPowerFailDuration.Describe(ch)
	c.longPowerFailCount.Describe(ch)
	c.tariffImportT1.Describe(ch)
	c.tariffImportT2.Describe(ch)
	c.tariffExportT1.Describe(ch)
	c.tariffExportT2.Describe(ch)
	c.costConfigured.Describe(ch)
	c.importCostEUR.Describe(ch)
	c.exportCreditEUR.Describe(ch)
	c.totalCostEUR.Describe(ch)
	c.lastUpdated.Describe(ch)
	c.lastSuccess.Describe(ch)
	c.success.Describe(ch)
	c.telegramSuccess.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := c.client.Data(ctx)
	if err != nil {
		log.Printf("p1_homewizard source read failed: %v", err)
		c.success.Set(0)
		c.telegramSuccess.Set(0)
		c.success.Collect(ch)
		c.lastSuccess.Collect(ch)
		return
	}

	c.success.Set(1)
	c.lastSuccess.Set(float64(time.Now().Unix()))

	setGauge(c.wifiStrengthPercent, data.WifiStrengthPercent)
	setGauge(c.activePowerW, data.ActivePowerW)
	setGauge(c.activePowerL1W, data.ActivePowerL1W)
	setGauge(c.activeCurrentA, data.ActiveCurrentA)
	setGauge(c.activeCurrentL1A, data.ActiveCurrentL1A)
	setGaugeInt(c.activeTariff, data.ActiveTariff)
	setGauge(c.totalImportKWh, data.TotalImportKWh)
	setGauge(c.totalImportT1KWh, data.TotalImportT1KWh)
	setGauge(c.totalImportT2KWh, data.TotalImportT2KWh)
	setGauge(c.totalExportKWh, data.TotalExportKWh)
	setGauge(c.totalExportT1KWh, data.TotalExportT1KWh)
	setGauge(c.totalExportT2KWh, data.TotalExportT2KWh)
	setGauge(c.voltageSagL1Count, data.VoltageSagL1Count)
	setGauge(c.voltageSwellL1Count, data.VoltageSwellL1Count)
	setGauge(c.anyPowerFailCount, data.AnyPowerFailCount)
	setGauge(c.longPowerFailCount, data.LongPowerFailCount)

	c.tariffImportT1.Set(c.tariffs.ImportT1EurPerKWh)
	c.tariffImportT2.Set(c.tariffs.ImportT2EurPerKWh)
	c.tariffExportT1.Set(c.tariffs.ExportT1EurPerKWh)
	c.tariffExportT2.Set(c.tariffs.ExportT2EurPerKWh)

	if c.tariffs.Configured() {
		c.costConfigured.Set(1)
		importCost, exportCredit, totalCost := costBreakdown(data, c.tariffs)
		c.importCostEUR.Set(importCost)
		c.exportCreditEUR.Set(exportCredit)
		c.totalCostEUR.Set(totalCost)
	} else {
		c.costConfigured.Set(0)
		c.importCostEUR.Set(0)
		c.exportCreditEUR.Set(0)
		c.totalCostEUR.Set(0)
	}

	lastUpdated := time.Now()
	c.lastPowerFailTimestamp.Set(math.NaN())
	c.lastPowerFailDuration.Set(math.NaN())
	var powerFailureEvents []PowerFailureEvent
	telegram, tErr := c.client.Telegram(ctx)
	if tErr != nil {
		c.telegramSuccess.Set(0)
		c.importPowerW.Set(math.NaN())
		c.exportPowerW.Set(math.NaN())
	} else {
		metrics, ok := ParseTelegram(telegram)
		if ok {
			c.telegramSuccess.Set(1)
			setGauge(c.importPowerW, metrics.ImportPowerW)
			setGauge(c.exportPowerW, metrics.ExportPowerW)
			if metrics.Timestamp != nil {
				lastUpdated = *metrics.Timestamp
			}
			powerFailureEvents = metrics.PowerFailureEvents
			if len(powerFailureEvents) > 0 {
				latest := powerFailureEvents[0]
				for _, ev := range powerFailureEvents[1:] {
					if ev.RestoredAt.After(latest.RestoredAt) {
						latest = ev
					}
				}
				c.lastPowerFailTimestamp.Set(float64(latest.RestoredAt.Unix()))
				c.lastPowerFailDuration.Set(latest.DurationSeconds)
			}
		} else {
			c.telegramSuccess.Set(0)
			c.importPowerW.Set(math.NaN())
			c.exportPowerW.Set(math.NaN())
		}
	}

	c.lastUpdated.Set(float64(lastUpdated.Unix()))
	c.collectAll(ch)
	seen := make(map[int64]struct{}, len(powerFailureEvents))
	for _, ev := range powerFailureEvents {
		ts := ev.RestoredAt.Unix()
		if _, exists := seen[ts]; exists {
			continue
		}
		seen[ts] = struct{}{}
		metric := prometheus.MustNewConstMetric(powerFailureDurationDesc, prometheus.GaugeValue, ev.DurationSeconds)
		ch <- prometheus.NewMetricWithTimestamp(ev.RestoredAt, metric)
	}
	if info := c.deviceInfo(ctx, data); info != nil {
		ch <- prometheus.MustNewConstMetric(
			deviceInfoDesc,
			prometheus.GaugeValue,
			1,
			info.Firmware,
			strconv.Itoa(info.SMRVersion),
			info.MeterModel,
		)
	}
}

func (c *MetricsCollector) deviceInfo(ctx context.Context, data Data) *Info {
	c.infoMu.Lock()
	defer c.infoMu.Unlock()
	if c.cachedInfo != nil && time.Since(c.infoFetched) < time.Hour {
		return c.cachedInfo
	}
	info, err := c.client.Info(ctx)
	if err != nil {
		return c.cachedInfo
	}
	if info.SMRVersion == 0 && data.SMRVersion != nil {
		info.SMRVersion = *data.SMRVersion
	}
	if info.MeterModel == "" && data.MeterModel != nil {
		info.MeterModel = *data.MeterModel
	}
	c.cachedInfo = &info
	c.infoFetched = time.Now()
	return c.cachedInfo
}

func (c *MetricsCollector) collectAll(ch chan<- prometheus.Metric) {
	core.CollectPresent(c.wifiStrengthPercent, ch)
	core.CollectPresent(c.activePowerW, ch)
	core.CollectPresent(c.activePowerL1W, ch)
	core.CollectPresent(c.importPowerW, ch)
	core.CollectPresent(c.exportPowerW, ch)
	core.CollectPresent(c.activeCurrentA, ch)
	core.CollectPresent(c.activeCurrentL1A, ch)
	core.CollectPresent(c.activeTariff, ch)
	core.CollectPresent(c.totalImportKWh, ch)
	core.CollectPresent(c.totalImportT1KWh, ch)
	core.CollectPresent(c.totalImportT2KWh, ch)
	core.CollectPresent(c.totalExportKWh, ch)
	core.CollectPresent(c.totalExportT1KWh, ch)
	core.CollectPresent(c.totalExportT2KWh, ch)
	core.CollectPresent(c.voltageSagL1Count, ch)
	core.CollectPresent(c.voltageSwellL1Count, ch)
	core.CollectPresent(c.anyPowerFailCount, ch)
	core.CollectPresent(c.longPowerFailCount, ch)
	core.CollectPresent(c.lastPowerFailTimestamp, ch)
	core.CollectPresent(c.lastPowerFailDuration, ch)
	c.tariffImportT1.Collect(ch)
	c.tariffImportT2.Collect(ch)
	c.tariffExportT1.Collect(ch)
	c.tariffExportT2.Collect(ch)
	c.costConfigured.Collect(ch)
	c.importCostEUR.Collect(ch)
	c.exportCreditEUR.Collect(ch)
	c.totalCostEUR.Collect(ch)
	c.lastUpdated.Collect(ch)
	c.lastSuccess.Collect(ch)
	c.success.Collect(ch)
	c.telegramSuccess.Collect(ch)
}

func setGauge(g prometheus.Gauge, value *float64) {
	if value == nil {
		g.Set(math.NaN())
		return
	}
	g.Set(*value)
}

func setGaugeInt(g prometheus.Gauge, value *int) {
	if value == nil {
		g.Set(math.NaN())
		return
	}
	g.Set(float64(*value))
}

func value(input *float64) float64 {
	if input == nil {
		return 0
	}
	return *input
}

func costBreakdown(data Data, tariffs Tariffs) (importCostEUR, exportCreditEUR, totalCostEUR float64) {
	importCostEUR = value(data.TotalImportT1KWh)*tariffs.ImportT1EurPerKWh + value(data.TotalImportT2KWh)*tariffs.ImportT2EurPerKWh
	exportCreditEUR = value(data.TotalExportT1KWh)*math.Abs(tariffs.ExportT1EurPerKWh) + value(data.TotalExportT2KWh)*math.Abs(tariffs.ExportT2EurPerKWh)
	totalCostEUR = importCostEUR - exportCreditEUR
	return importCostEUR, exportCreditEUR, totalCostEUR
}
