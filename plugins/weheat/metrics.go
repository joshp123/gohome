package weheat

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"

	weheatapi "github.com/joshp123/weheat-golang"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	logFetchInterval    = 15 * time.Second
	energyFetchInterval = 30 * time.Minute
)

type logField struct {
	jsonName   string
	metricName string
	index      int
	kind       reflect.Kind
	isPtr      bool
}

// MetricsCollector collects Weheat metrics.
type MetricsCollector struct {
	client *Client

	scrapeSuccess      prometheus.Gauge
	energySuccess      prometheus.Gauge
	lastSuccess        prometheus.Gauge
	lastEnergySuccess  prometheus.Gauge
	lastUpdate         *prometheus.GaugeVec

	logMetrics    map[string]*prometheus.GaugeVec
	energyMetrics map[string]*prometheus.GaugeVec
	logFields     []logField

	mu               sync.Mutex
	cachedPumps      []weheatapi.ReadAllHeatPump
	cachedAt         time.Time
	logFetchedAt     map[string]time.Time
	energyFetchedAt  map[string]time.Time
	logCache         map[string]*weheatapi.RawHeatPumpLog
	energyCache      map[string]*weheatapi.TotalEnergyAggregate
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"heat_pump_id", "serial_number", "name", "model"}
	logFields := buildLogFields()
	logMetrics := make(map[string]*prometheus.GaugeVec, len(logFields))
	for _, field := range logFields {
		logMetrics[field.jsonName] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: field.metricName,
			Help: fmt.Sprintf("Raw Weheat log field %s", field.jsonName),
		}, labels)
	}

	energyMetrics := map[string]*prometheus.GaugeVec{
		"total_ein_heating": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_ein_heating_kwh",
			Help: "Total energy input heating (kWh)",
		}, labels),
		"total_ein_standby": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_ein_standby_kwh",
			Help: "Total energy input standby (kWh)",
		}, labels),
		"total_ein_dhw": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_ein_dhw_kwh",
			Help: "Total energy input DHW (kWh)",
		}, labels),
		"total_ein_heating_defrost": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_ein_heating_defrost_kwh",
			Help: "Total energy input heating defrost (kWh)",
		}, labels),
		"total_ein_dhw_defrost": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_ein_dhw_defrost_kwh",
			Help: "Total energy input DHW defrost (kWh)",
		}, labels),
		"total_ein_cooling": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_ein_cooling_kwh",
			Help: "Total energy input cooling (kWh)",
		}, labels),
		"total_eout_heating": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_eout_heating_kwh",
			Help: "Total energy output heating (kWh)",
		}, labels),
		"total_eout_dhw": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_eout_dhw_kwh",
			Help: "Total energy output DHW (kWh)",
		}, labels),
		"total_eout_heating_defrost": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_eout_heating_defrost_kwh",
			Help: "Total energy output heating defrost (kWh)",
		}, labels),
		"total_eout_dhw_defrost": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_eout_dhw_defrost_kwh",
			Help: "Total energy output DHW defrost (kWh)",
		}, labels),
		"total_eout_cooling": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_total_eout_cooling_kwh",
			Help: "Total energy output cooling (kWh)",
		}, labels),
	}

	return &MetricsCollector{
		client: client,
		scrapeSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_weheat_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
		energySuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_scrape_success",
			Help: "Last energy scrape success (1=ok, 0=error)",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_weheat_last_success_timestamp_seconds",
			Help: "Last successful scrape timestamp (epoch seconds)",
		}),
		lastEnergySuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_weheat_energy_last_success_timestamp_seconds",
			Help: "Last successful energy scrape timestamp (epoch seconds)",
		}),
		lastUpdate: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_weheat_last_update_timestamp_seconds",
			Help: "Last update timestamp per heat pump (epoch seconds)",
		}, labels),
		logMetrics:       logMetrics,
		energyMetrics:    energyMetrics,
		logFields:        logFields,
		logFetchedAt:     make(map[string]time.Time),
		energyFetchedAt:  make(map[string]time.Time),
		logCache:         make(map[string]*weheatapi.RawHeatPumpLog),
		energyCache:      make(map[string]*weheatapi.TotalEnergyAggregate),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.scrapeSuccess.Describe(ch)
	c.energySuccess.Describe(ch)
	c.lastSuccess.Describe(ch)
	c.lastEnergySuccess.Describe(ch)
	c.lastUpdate.Describe(ch)
	for _, gauge := range c.logMetrics {
		gauge.Describe(ch)
	}
	for _, gauge := range c.energyMetrics {
		gauge.Describe(ch)
	}
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if c.client == nil {
		c.scrapeSuccess.Set(0)
		c.energySuccess.Set(0)
		c.collectAll(ch)
		return
	}

	pumps, err := c.fetchPumps(ctx)
	if err != nil {
		c.scrapeSuccess.Set(0)
		c.energySuccess.Set(0)
		c.collectAll(ch)
		return
	}

	logOK := true
	energyOK := true
	now := time.Now()

	for _, pump := range pumps {
		labels := prometheus.Labels{
			"heat_pump_id":  pump.ID,
			"serial_number": pump.SerialNumber,
			"name":          derefString(pump.Name),
			"model":         modelName(pump.Model),
		}

		logEntry, err := c.fetchLatestLog(ctx, pump.ID)
		if err != nil {
			logOK = false
		} else if logEntry != nil {
			c.lastUpdate.With(labels).Set(float64(logEntry.Timestamp.Unix()))
			c.applyLog(labels, logEntry)
		}

		energy, err := c.fetchEnergyTotals(ctx, pump.ID)
		if err != nil {
			energyOK = false
		} else if energy != nil {
			c.applyEnergy(labels, energy)
		}
	}

	if logOK {
		c.scrapeSuccess.Set(1)
		c.lastSuccess.Set(float64(now.Unix()))
	} else {
		c.scrapeSuccess.Set(0)
	}
	if energyOK {
		c.energySuccess.Set(1)
		c.lastEnergySuccess.Set(float64(now.Unix()))
	} else {
		c.energySuccess.Set(0)
	}

	c.collectAll(ch)
}

func (c *MetricsCollector) collectAll(ch chan<- prometheus.Metric) {
	c.scrapeSuccess.Collect(ch)
	c.energySuccess.Collect(ch)
	c.lastSuccess.Collect(ch)
	c.lastEnergySuccess.Collect(ch)
	c.lastUpdate.Collect(ch)
	for _, gauge := range c.logMetrics {
		gauge.Collect(ch)
	}
	for _, gauge := range c.energyMetrics {
		gauge.Collect(ch)
	}
}

func (c *MetricsCollector) fetchPumps(ctx context.Context) ([]weheatapi.ReadAllHeatPump, error) {
	c.mu.Lock()
	if time.Since(c.cachedAt) < logFetchInterval && len(c.cachedPumps) > 0 {
		pumps := append([]weheatapi.ReadAllHeatPump(nil), c.cachedPumps...)
		c.mu.Unlock()
		return pumps, nil
	}
	c.mu.Unlock()

	pumps, err := c.client.ListHeatPumps(ctx, nil)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cachedPumps = append([]weheatapi.ReadAllHeatPump(nil), pumps...)
	c.cachedAt = time.Now()
	c.mu.Unlock()

	return pumps, nil
}

func (c *MetricsCollector) fetchLatestLog(ctx context.Context, id string) (*weheatapi.RawHeatPumpLog, error) {
	c.mu.Lock()
	if last, ok := c.logFetchedAt[id]; ok && time.Since(last) < logFetchInterval {
		log := c.logCache[id]
		c.mu.Unlock()
		return log, nil
	}
	c.mu.Unlock()

	log, err := c.client.LatestLog(ctx, id)
	c.mu.Lock()
	c.logFetchedAt[id] = time.Now()
	if err == nil {
		c.logCache[id] = log
	}
	c.mu.Unlock()
	return log, err
}

func (c *MetricsCollector) fetchEnergyTotals(ctx context.Context, id string) (*weheatapi.TotalEnergyAggregate, error) {
	c.mu.Lock()
	if last, ok := c.energyFetchedAt[id]; ok && time.Since(last) < energyFetchInterval {
		energy := c.energyCache[id]
		c.mu.Unlock()
		return energy, nil
	}
	c.mu.Unlock()

	energy, err := c.client.EnergyTotals(ctx, id)
	c.mu.Lock()
	c.energyFetchedAt[id] = time.Now()
	if err == nil {
		c.energyCache[id] = energy
	}
	c.mu.Unlock()
	return energy, err
}

func (c *MetricsCollector) applyLog(labels prometheus.Labels, log *weheatapi.RawHeatPumpLog) {
	if log == nil {
		return
	}
	value := reflect.ValueOf(log).Elem()
	for _, field := range c.logFields {
		metric := c.logMetrics[field.jsonName]
		if metric == nil {
			continue
		}
		val, ok := fieldValue(value, field)
		if !ok {
			continue
		}
		metric.With(labels).Set(val)
	}
}

func (c *MetricsCollector) applyEnergy(labels prometheus.Labels, energy *weheatapi.TotalEnergyAggregate) {
	if energy == nil {
		return
	}
	setEnergy := func(key string, value *float64) {
		if value == nil {
			return
		}
		if metric, ok := c.energyMetrics[key]; ok {
			metric.With(labels).Set(*value)
		}
	}
	setEnergy("total_ein_heating", energy.TotalEInHeating)
	setEnergy("total_ein_standby", energy.TotalEInStandby)
	setEnergy("total_ein_dhw", energy.TotalEInDHW)
	setEnergy("total_ein_heating_defrost", energy.TotalEInHeatingDefrost)
	setEnergy("total_ein_dhw_defrost", energy.TotalEInDHWDefrost)
	setEnergy("total_ein_cooling", energy.TotalEInCooling)
	setEnergy("total_eout_heating", energy.TotalEOutHeating)
	setEnergy("total_eout_dhw", energy.TotalEOutDHW)
	setEnergy("total_eout_heating_defrost", energy.TotalEOutHeatingDefrost)
	setEnergy("total_eout_dhw_defrost", energy.TotalEOutDHWDefrost)
	setEnergy("total_eout_cooling", energy.TotalEOutCooling)
}

func buildLogFields() []logField {
	var fields []logField
	typeOf := reflect.TypeOf(weheatapi.RawHeatPumpLog{})
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		if jsonTag == "heatPumpId" || jsonTag == "timestamp" {
			continue
		}
		metricName := "gohome_weheat_log_" + toSnake(jsonTag)
		fields = append(fields, logField{
			jsonName:   jsonTag,
			metricName: metricName,
			index:      i,
			kind:       field.Type.Kind(),
			isPtr:      field.Type.Kind() == reflect.Ptr,
		})
	}
	return fields
}

func fieldValue(value reflect.Value, field logField) (float64, bool) {
	fieldValue := value.Field(field.index)
	if field.isPtr {
		if fieldValue.IsNil() {
			return 0, false
		}
		fieldValue = fieldValue.Elem()
	}

	switch fieldValue.Kind() {
	case reflect.Int, reflect.Int64, reflect.Int32:
		return float64(fieldValue.Int()), true
	case reflect.Float64, reflect.Float32:
		return fieldValue.Float(), true
	case reflect.Bool:
		if fieldValue.Bool() {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func toSnake(value string) string {
	if value == "" {
		return value
	}
	var out []rune
	var prevLower bool
	for i, r := range value {
		if r == '_' {
			out = append(out, r)
			prevLower = false
			continue
		}
		isUpper := unicode.IsUpper(r)
		if i > 0 && isUpper && prevLower {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(r))
		prevLower = !isUpper
	}
	return string(out)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func modelName(model *weheatapi.HeatPumpModel) string {
	if model == nil {
		return ""
	}
	return weheatapi.HeatPumpModelName(*model)
}
