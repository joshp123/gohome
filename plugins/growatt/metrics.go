package growatt

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	growattMinFetchInterval = 5 * time.Minute
	growattHistoryInterval  = 12 * time.Hour
	growattHistoryTimeout   = 8 * time.Minute
	growattHistoryRetry     = 30 * time.Minute
	growattHistoryRateRetry = 2 * time.Minute
)

type cachedSnapshot struct {
	plant     Plant
	energy    PlantEnergy
	fetchedAt time.Time
	success   bool
}

// MetricsCollector collects Growatt plant metrics.
type MetricsCollector struct {
	client *Client

	currentPower *prometheus.GaugeVec
	todayEnergy  *prometheus.GaugeVec
	monthEnergy  *prometheus.GaugeVec
	yearEnergy   *prometheus.GaugeVec
	totalEnergy  *prometheus.GaugeVec
	lastUpdated  *prometheus.GaugeVec
	lastSuccess  prometheus.Gauge
	success      prometheus.Gauge
	historyLast  prometheus.Gauge
	historyOK    prometheus.Gauge

	mu          sync.Mutex
	cached      *cachedSnapshot
	historyNext time.Time
	historyBusy bool
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"plant_id", "plant_name"}
	return &MetricsCollector{
		client: client,
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
		historyLast: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_growatt_history_last_success_timestamp_seconds",
			Help: "Last successful Growatt history import timestamp (epoch seconds)",
		}),
		historyOK: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_growatt_history_import_success",
			Help: "Last history import success (1=ok, 0=error)",
		}),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.currentPower.Describe(ch)
	c.todayEnergy.Describe(ch)
	c.monthEnergy.Describe(ch)
	c.yearEnergy.Describe(ch)
	c.totalEnergy.Describe(ch)
	c.lastUpdated.Describe(ch)
	c.lastSuccess.Describe(ch)
	c.success.Describe(ch)
	c.historyLast.Describe(ch)
	c.historyOK.Describe(ch)
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
		c.storeSnapshot(cachedSnapshot{fetchedAt: time.Now(), success: false})
		c.collectAll(ch)
		return
	}

	energy, err := c.client.EnergyOverview(ctx, plant.ID)
	if err != nil {
		snapshot := cachedSnapshot{
			plant:     plant,
			energy:    PlantEnergy{PlantID: plant.ID},
			fetchedAt: time.Now(),
			success:   false,
		}
		if isRateLimit(err) {
			c.mu.Lock()
			if c.cached != nil {
				snapshot = *c.cached
				snapshot.fetchedAt = time.Now()
			}
			c.mu.Unlock()
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
	c.applySnapshot(snapshot)
	c.maybeImportHistory(plant)
	c.collectAll(ch)
}

func (c *MetricsCollector) storeSnapshot(snapshot cachedSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = &snapshot
}

func (c *MetricsCollector) applySnapshot(snapshot cachedSnapshot) {
	c.currentPower.Reset()
	c.todayEnergy.Reset()
	c.monthEnergy.Reset()
	c.yearEnergy.Reset()
	c.totalEnergy.Reset()
	c.lastUpdated.Reset()

	if snapshot.plant.ID != 0 {
		labels := prometheus.Labels{
			"plant_id":   strconv.FormatInt(snapshot.plant.ID, 10),
			"plant_name": snapshot.plant.Name,
		}
		c.currentPower.With(labels).Set(snapshot.energy.CurrentPowerW)
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
	c.currentPower.Collect(ch)
	c.todayEnergy.Collect(ch)
	c.monthEnergy.Collect(ch)
	c.yearEnergy.Collect(ch)
	c.totalEnergy.Collect(ch)
	c.lastUpdated.Collect(ch)
	c.lastSuccess.Collect(ch)
	c.success.Collect(ch)
	c.historyLast.Collect(ch)
	c.historyOK.Collect(ch)
}

func (c *MetricsCollector) maybeImportHistory(plant Plant) {
	if c.client == nil {
		return
	}

	c.mu.Lock()
	if c.historyBusy || (!c.historyNext.IsZero() && time.Now().Before(c.historyNext)) {
		c.mu.Unlock()
		return
	}
	c.historyBusy = true
	c.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), growattHistoryTimeout)
		defer cancel()

		err := c.client.ImportEnergyHistory(ctx, plant)
		next := time.Now().Add(growattHistoryRetry)
		c.mu.Lock()
		c.historyBusy = false
		if err == nil {
			now := time.Now()
			c.historyNext = now.Add(growattHistoryInterval)
			c.historyOK.Set(1)
			c.historyLast.Set(float64(now.Unix()))
		} else {
			if isRateLimit(err) {
				next = time.Now().Add(growattHistoryRateRetry)
			}
			c.historyNext = next
			c.historyOK.Set(0)
		}
		c.mu.Unlock()
		if err != nil {
			log.Printf("growatt history import failed: %v", err)
		}
	}()
}
