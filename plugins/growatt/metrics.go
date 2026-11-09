package growatt

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// The inverter reports every five minutes. VictoriaMetrics scrapes every 15 s, so a fetch happens at the first
	// scrape after this interval: at 5 minutes the fetches fell 5 m 15 s apart and one report in about twenty was
	// never read. Just under five minutes keeps the fetches no further apart than the reports.
	growattMinFetchInterval = 4*time.Minute + 50*time.Second
)

type cachedSnapshot struct {
	plant     Plant
	energy    PlantEnergy
	fetchedAt time.Time
	success   bool
}

// MetricsCollector collects Growatt plant metrics.
type MetricsCollector struct {
	client              *Client
	inverterDescriptors map[v1.InverterQuantity]*prometheus.Desc

	currentPower *prometheus.GaugeVec
	todayEnergy  *prometheus.GaugeVec
	monthEnergy  *prometheus.GaugeVec
	yearEnergy   *prometheus.GaugeVec
	totalEnergy  *prometheus.GaugeVec
	lastUpdated  *prometheus.GaugeVec
	lastSuccess  prometheus.Gauge
	success      prometheus.Gauge

	mu     sync.Mutex
	cached *cachedSnapshot
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"plant_id", "plant_name"}
	return &MetricsCollector{
		client:              client,
		inverterDescriptors: inverterDescriptors(),
		currentPower: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_growatt_current_power_watts",
			Help: "Current power output per plant (watts)",
		}, labels),
		todayEnergy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_growatt_today_energy_kwh",
			Help: "Today's energy per plant (kWh)",
		}, labels),
		monthEnergy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_growatt_monthly_energy_kwh",
			Help: "Monthly energy per plant (kWh)",
		}, labels),
		yearEnergy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_growatt_yearly_energy_kwh",
			Help: "Yearly energy per plant (kWh)",
		}, labels),
		totalEnergy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_growatt_total_energy_kwh",
			Help: "Total lifetime energy per plant (kWh)",
		}, labels),
		lastUpdated: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_growatt_last_update_timestamp_seconds",
			Help: "Last update timestamp per plant (epoch seconds)",
		}, labels),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_growatt_last_success_timestamp_seconds",
			Help: "Last successful Growatt scrape timestamp (epoch seconds)",
		}),
		success: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_growatt_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.inverterDescriptors {
		ch <- desc
	}
	c.currentPower.Describe(ch)
	c.todayEnergy.Describe(ch)
	c.monthEnergy.Describe(ch)
	c.yearEnergy.Describe(ch)
	c.totalEnergy.Describe(ch)
	c.lastUpdated.Describe(ch)
	c.lastSuccess.Describe(ch)
	c.success.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	if c.cached != nil && time.Since(c.cached.fetchedAt) < growattMinFetchInterval {
		snapshot := *c.cached
		c.mu.Unlock()
		c.applySnapshot(snapshot)
		c.collectAll(ch)
		return
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if c.client == nil {
		c.storeSnapshot(cachedSnapshot{fetchedAt: time.Now(), success: false})
		c.collectAll(ch)
		return
	}

	plant, err := c.client.ResolvePlant(ctx, 0)
	if err != nil {
		log.Printf("growatt source read failed: %v", err)
		c.storeSnapshot(cachedSnapshot{fetchedAt: time.Now(), success: false})
		c.collectAll(ch)
		return
	}

	energy, err := c.client.EnergyOverview(ctx, plant.ID)
	if err != nil {
		log.Printf("growatt source read failed: %v", err)
		snapshot := cachedSnapshot{
			plant:     plant,
			energy:    PlantEnergy{PlantID: plant.ID},
			fetchedAt: time.Now(),
			success:   false,
		}
		c.storeSnapshot(snapshot)
		c.collectAll(ch)
		return
	}

	snapshot := cachedSnapshot{
		plant:     plant,
		energy:    energy,
		fetchedAt: time.Now(),
		success:   true,
	}
	c.storeSnapshot(snapshot)
	c.collectAll(ch)
}

func (c *MetricsCollector) storeSnapshot(snapshot cachedSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = &snapshot
	c.applySnapshot(snapshot)
}

func (c *MetricsCollector) applySnapshot(snapshot cachedSnapshot) {
	c.currentPower.Reset()
	c.todayEnergy.Reset()
	c.monthEnergy.Reset()
	c.yearEnergy.Reset()
	c.totalEnergy.Reset()
	c.lastUpdated.Reset()

	if snapshot.success && snapshot.plant.ID != 0 {
		labels := prometheus.Labels{
			"plant_id":   strconv.FormatInt(snapshot.plant.ID, 10),
			"plant_name": snapshot.plant.Name,
		}
		if snapshot.energy.LastUpdate != nil && time.Since(*snapshot.energy.LastUpdate) <= 15*time.Minute {
			c.currentPower.With(labels).Set(snapshot.energy.CurrentPowerW)
		}
		c.todayEnergy.With(labels).Set(snapshot.energy.TodayEnergyKWh)
		c.monthEnergy.With(labels).Set(snapshot.energy.MonthlyEnergyKWh)
		c.yearEnergy.With(labels).Set(snapshot.energy.YearlyEnergyKWh)
		c.totalEnergy.With(labels).Set(snapshot.energy.TotalEnergyKWh)
		if snapshot.energy.LastUpdate != nil {
			c.lastUpdated.With(labels).Set(float64(snapshot.energy.LastUpdate.Unix()))
		}
	}

	if snapshot.success {
		c.success.Set(1)
		c.lastSuccess.Set(float64(snapshot.fetchedAt.Unix()))
	} else {
		c.success.Set(0)
	}
}

func (c *MetricsCollector) collectAll(ch chan<- prometheus.Metric) {
	// Growatt readings are five to nine minutes old when fetched; store them at the
	// inverter's own reading time so they line up with the electricity meter.
	var readAt *time.Time
	c.mu.Lock()
	if c.cached != nil {
		readAt = c.cached.energy.LastUpdate
		if c.cached.success {
			collectInverter(c.cached.energy.Inverter, strconv.FormatInt(c.cached.plant.ID, 10), c.cached.plant.Name, c.inverterDescriptors, ch)
		}
	}
	c.mu.Unlock()
	for _, reading := range []prometheus.Collector{c.currentPower, c.todayEnergy, c.monthEnergy, c.yearEnergy, c.totalEnergy} {
		collectAt(reading, readAt, ch)
	}
	c.lastUpdated.Collect(ch)
	c.lastSuccess.Collect(ch)
	c.success.Collect(ch)
}

func collectAt(collector prometheus.Collector, at *time.Time, ch chan<- prometheus.Metric) {
	if at == nil {
		collector.Collect(ch)
		return
	}
	metrics := make(chan prometheus.Metric)
	go func() { collector.Collect(metrics); close(metrics) }()
	for metric := range metrics {
		ch <- prometheus.NewMetricWithTimestamp(*at, metric)
	}
}
