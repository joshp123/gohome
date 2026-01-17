package airgradient

import (
	"bufio"
	"bytes"
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector collects AirGradient sensor metrics.
type MetricsCollector struct {
	client *Client

	scrapeSuccess      prometheus.Gauge
	lastSuccess        prometheus.Gauge
	lastUpdated        prometheus.Gauge
	openMetricsSuccess prometheus.Gauge
	configOK           prometheus.Gauge
	postOK             prometheus.Gauge
	info               *prometheus.GaugeVec

	wifiRssiDbm     prometheus.Gauge
	co2Ppm          prometheus.Gauge
	pm01Ugm3        prometheus.Gauge
	pm02Ugm3        prometheus.Gauge
	pm10Ugm3        prometheus.Gauge
	pm02CompUgm3    prometheus.Gauge
	pm01StdUgm3     prometheus.Gauge
	pm02StdUgm3     prometheus.Gauge
	pm10StdUgm3     prometheus.Gauge
	pm003CountPerDl prometheus.Gauge
	pm005CountPerDl prometheus.Gauge
	pm01CountPerDl  prometheus.Gauge
	pm02CountPerDl  prometheus.Gauge
	pm50CountPerDl  prometheus.Gauge
	pm10CountPerDl  prometheus.Gauge
	tempCelsius     prometheus.Gauge
	tempCompCelsius prometheus.Gauge
	humidityPercent prometheus.Gauge
	humidityComp    prometheus.Gauge
	tvocIndex       prometheus.Gauge
	tvocRaw         prometheus.Gauge
	noxIndex        prometheus.Gauge
	noxRaw          prometheus.Gauge
	bootCount       prometheus.Gauge

	satelliteTempCelsius *prometheus.GaugeVec
	satelliteHumidity    *prometheus.GaugeVec
	satelliteWifiRssiDbm *prometheus.GaugeVec
}

func NewMetricsCollector(client *Client) *MetricsCollector {
	labels := []string{"serial", "model", "firmware", "led_mode"}
	satelliteLabels := []string{"satellite_id"}
	return &MetricsCollector{
		client: client,
		scrapeSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_scrape_success",
			Help: "Last scrape success (1=ok, 0=error)",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_last_success_timestamp_seconds",
			Help: "Last successful scrape timestamp (epoch seconds)",
		}),
		lastUpdated: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_last_update_timestamp_seconds",
			Help: "Last update timestamp (epoch seconds)",
		}),
		openMetricsSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_openmetrics_scrape_success",
			Help: "Last openmetrics scrape success (1=ok, 0=error)",
		}),
		configOK: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_config_ok",
			Help: "1 if the AirGradient device reported config fetch success",
		}),
		postOK: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_post_ok",
			Help: "1 if the AirGradient device reported upload success",
		}),
		info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_airgradient_info",
			Help: "AirGradient device info",
		}, labels),
		wifiRssiDbm: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_wifi_rssi_dbm",
			Help: "WiFi signal strength (dBm)",
		}),
		co2Ppm: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_co2_ppm",
			Help: "CO2 concentration (ppm)",
		}),
		pm01Ugm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm01_ugm3",
			Help: "PM1.0 concentration (ug/m3)",
		}),
		pm02Ugm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm02_ugm3",
			Help: "PM2.5 concentration (ug/m3)",
		}),
		pm10Ugm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm10_ugm3",
			Help: "PM10 concentration (ug/m3)",
		}),
		pm02CompUgm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm02_compensated_ugm3",
			Help: "PM2.5 concentration with compensation (ug/m3)",
		}),
		pm01StdUgm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm01_standard_ugm3",
			Help: "PM1.0 concentration (standard particle) (ug/m3)",
		}),
		pm02StdUgm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm02_standard_ugm3",
			Help: "PM2.5 concentration (standard particle) (ug/m3)",
		}),
		pm10StdUgm3: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm10_standard_ugm3",
			Help: "PM10 concentration (standard particle) (ug/m3)",
		}),
		pm003CountPerDl: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm003_count_per_dl",
			Help: "Particle count 0.3um per dL",
		}),
		pm005CountPerDl: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm005_count_per_dl",
			Help: "Particle count 0.5um per dL",
		}),
		pm01CountPerDl: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm01_count_per_dl",
			Help: "Particle count 1.0um per dL",
		}),
		pm02CountPerDl: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm02_count_per_dl",
			Help: "Particle count 2.5um per dL",
		}),
		pm50CountPerDl: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm50_count_per_dl",
			Help: "Particle count 5.0um per dL",
		}),
		pm10CountPerDl: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_pm10_count_per_dl",
			Help: "Particle count 10um per dL",
		}),
		tempCelsius: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_temperature_celsius",
			Help: "Temperature (celsius)",
		}),
		tempCompCelsius: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_temperature_compensated_celsius",
			Help: "Compensated temperature (celsius)",
		}),
		humidityPercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_humidity_percent",
			Help: "Relative humidity (%)",
		}),
		humidityComp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_humidity_compensated_percent",
			Help: "Compensated relative humidity (%)",
		}),
		tvocIndex: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_tvoc_index",
			Help: "TVOC index",
		}),
		tvocRaw: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_tvoc_raw",
			Help: "TVOC raw value",
		}),
		noxIndex: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_nox_index",
			Help: "NOx index",
		}),
		noxRaw: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_nox_raw",
			Help: "NOx raw value",
		}),
		bootCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gohome_airgradient_boot_count",
			Help: "Boot counter",
		}),
		satelliteTempCelsius: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_airgradient_satellite_temperature_celsius",
			Help: "Satellite temperature (celsius)",
		}, satelliteLabels),
		satelliteHumidity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_airgradient_satellite_humidity_percent",
			Help: "Satellite humidity (%)",
		}, satelliteLabels),
		satelliteWifiRssiDbm: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gohome_airgradient_satellite_wifi_rssi_dbm",
			Help: "Satellite WiFi signal strength (dBm)",
		}, satelliteLabels),
	}
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.scrapeSuccess.Describe(ch)
	c.lastSuccess.Describe(ch)
	c.lastUpdated.Describe(ch)
	c.openMetricsSuccess.Describe(ch)
	c.configOK.Describe(ch)
	c.postOK.Describe(ch)
	c.info.Describe(ch)
	c.wifiRssiDbm.Describe(ch)
	c.co2Ppm.Describe(ch)
	c.pm01Ugm3.Describe(ch)
	c.pm02Ugm3.Describe(ch)
	c.pm10Ugm3.Describe(ch)
	c.pm02CompUgm3.Describe(ch)
	c.pm01StdUgm3.Describe(ch)
	c.pm02StdUgm3.Describe(ch)
	c.pm10StdUgm3.Describe(ch)
	c.pm003CountPerDl.Describe(ch)
	c.pm005CountPerDl.Describe(ch)
	c.pm01CountPerDl.Describe(ch)
	c.pm02CountPerDl.Describe(ch)
	c.pm50CountPerDl.Describe(ch)
	c.pm10CountPerDl.Describe(ch)
	c.tempCelsius.Describe(ch)
	c.tempCompCelsius.Describe(ch)
	c.humidityPercent.Describe(ch)
	c.humidityComp.Describe(ch)
	c.tvocIndex.Describe(ch)
	c.tvocRaw.Describe(ch)
	c.noxIndex.Describe(ch)
	c.noxRaw.Describe(ch)
	c.bootCount.Describe(ch)
	c.satelliteTempCelsius.Describe(ch)
	c.satelliteHumidity.Describe(ch)
	c.satelliteWifiRssiDbm.Describe(ch)
}

func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if c.client == nil {
		c.scrapeSuccess.Set(0)
		c.collectAll(ch)
		return
	}

	current, err := c.client.Current(ctx)
	if err != nil {
		c.scrapeSuccess.Set(0)
		c.collectAll(ch)
		return
	}

	now := time.Now()
	c.scrapeSuccess.Set(1)
	c.lastSuccess.Set(float64(now.Unix()))
	c.lastUpdated.Set(float64(now.Unix()))

	c.info.Reset()
	labels := prometheus.Labels{
		"serial":   current.SerialNo,
		"model":    current.Model,
		"firmware": current.Firmware,
		"led_mode": current.LedMode,
	}
	if labels["serial"] != "" || labels["model"] != "" || labels["firmware"] != "" || labels["led_mode"] != "" {
		c.info.With(labels).Set(1)
	}

	setGauge(c.wifiRssiDbm, current.Wifi)
	setGauge(c.co2Ppm, current.RCO2)
	setGauge(c.pm01Ugm3, current.PM01)
	setGauge(c.pm02Ugm3, current.PM02)
	setGauge(c.pm10Ugm3, current.PM10)
	setGauge(c.pm02CompUgm3, current.PM02Compensated)
	setGauge(c.pm01StdUgm3, current.PM01Standard)
	setGauge(c.pm02StdUgm3, current.PM02Standard)
	setGauge(c.pm10StdUgm3, current.PM10Standard)
	setGauge(c.pm003CountPerDl, current.PM003Count)
	setGauge(c.pm005CountPerDl, current.PM005Count)
	setGauge(c.pm01CountPerDl, current.PM01Count)
	setGauge(c.pm02CountPerDl, current.PM02Count)
	setGauge(c.pm50CountPerDl, current.PM50Count)
	setGauge(c.pm10CountPerDl, current.PM10Count)
	setGauge(c.tempCelsius, current.Temperature)
	setGauge(c.tempCompCelsius, current.TemperatureCorrected)
	setGauge(c.humidityPercent, current.Humidity)
	setGauge(c.humidityComp, current.HumidityCorrected)
	setGauge(c.tvocIndex, current.TVOCIndex)
	setGauge(c.tvocRaw, current.TVOCRaw)
	setGauge(c.noxIndex, current.NOxIndex)
	setGauge(c.noxRaw, current.NOxRaw)
	setGauge(c.bootCount, pickBootCount(current))

	c.satelliteTempCelsius.Reset()
	c.satelliteHumidity.Reset()
	c.satelliteWifiRssiDbm.Reset()
	if len(current.Satellites) > 0 {
		ids := make([]string, 0, len(current.Satellites))
		for id := range current.Satellites {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			sat := current.Satellites[id]
			labels := prometheus.Labels{"satellite_id": id}
			setGaugeVec(c.satelliteTempCelsius, labels, sat.Temperature)
			setGaugeVec(c.satelliteHumidity, labels, sat.Humidity)
			setGaugeVec(c.satelliteWifiRssiDbm, labels, sat.Wifi)
		}
	}

	c.applyOpenMetrics(ctx)
	c.collectAll(ch)
}

func (c *MetricsCollector) applyOpenMetrics(ctx context.Context) {
	payload, err := c.client.Metrics(ctx)
	if err != nil {
		c.openMetricsSuccess.Set(0)
		c.configOK.Set(0)
		c.postOK.Set(0)
		return
	}

	metrics := parseOpenMetrics(payload)
	c.openMetricsSuccess.Set(1)
	c.configOK.Set(metrics["airgradient_config_ok"])
	c.postOK.Set(metrics["airgradient_post_ok"])
}

func parseOpenMetrics(payload []byte) map[string]float64 {
	out := make(map[string]float64)
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.SplitN(fields[0], "{", 2)[0]
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		out[name] = value
	}
	return out
}

func (c *MetricsCollector) collectAll(ch chan<- prometheus.Metric) {
	c.scrapeSuccess.Collect(ch)
	c.lastSuccess.Collect(ch)
	c.lastUpdated.Collect(ch)
	c.openMetricsSuccess.Collect(ch)
	c.configOK.Collect(ch)
	c.postOK.Collect(ch)
	c.info.Collect(ch)
	c.wifiRssiDbm.Collect(ch)
	c.co2Ppm.Collect(ch)
	c.pm01Ugm3.Collect(ch)
	c.pm02Ugm3.Collect(ch)
	c.pm10Ugm3.Collect(ch)
	c.pm02CompUgm3.Collect(ch)
	c.pm01StdUgm3.Collect(ch)
	c.pm02StdUgm3.Collect(ch)
	c.pm10StdUgm3.Collect(ch)
	c.pm003CountPerDl.Collect(ch)
	c.pm005CountPerDl.Collect(ch)
	c.pm01CountPerDl.Collect(ch)
	c.pm02CountPerDl.Collect(ch)
	c.pm50CountPerDl.Collect(ch)
	c.pm10CountPerDl.Collect(ch)
	c.tempCelsius.Collect(ch)
	c.tempCompCelsius.Collect(ch)
	c.humidityPercent.Collect(ch)
	c.humidityComp.Collect(ch)
	c.tvocIndex.Collect(ch)
	c.tvocRaw.Collect(ch)
	c.noxIndex.Collect(ch)
	c.noxRaw.Collect(ch)
	c.bootCount.Collect(ch)
	c.satelliteTempCelsius.Collect(ch)
	c.satelliteHumidity.Collect(ch)
	c.satelliteWifiRssiDbm.Collect(ch)
}

func setGauge(g prometheus.Gauge, value *float64) {
	if value == nil {
		return
	}
	g.Set(*value)
}

func setGaugeVec(g *prometheus.GaugeVec, labels prometheus.Labels, value *float64) {
	if value == nil {
		return
	}
	g.With(labels).Set(*value)
}
