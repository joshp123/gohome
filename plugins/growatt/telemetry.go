package growatt

import (
	"fmt"
	"time"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
	"github.com/prometheus/client_golang/prometheus"
)

type inverterField struct {
	source   string
	quantity v1.InverterQuantity
	channel  string
}

type inverterMetric struct {
	name string
	help string
}

var inverterMetrics = map[v1.InverterQuantity]inverterMetric{
	v1.InverterQuantity_GRID_FREQUENCY_HERTZ:      {"gohome_growatt_inverter_grid_frequency_hertz", "Grid frequency in Hz"},
	v1.InverterQuantity_OUTPUT_POWER_WATTS:        {"gohome_growatt_inverter_output_power_watts", "AC output power in watts"},
	v1.InverterQuantity_PV_POWER_WATTS:            {"gohome_growatt_inverter_pv_power_watts", "Total PV input power in watts"},
	v1.InverterQuantity_POWER_FACTOR:              {"gohome_growatt_inverter_power_factor", "Reported power factor"},
	v1.InverterQuantity_TODAY_ENERGY_KWH:          {"gohome_growatt_inverter_today_energy_kwh", "Inverter daily AC energy in kWh"},
	v1.InverterQuantity_TOTAL_ENERGY_KWH:          {"gohome_growatt_inverter_total_energy_kwh", "Inverter lifetime AC energy in kWh"},
	v1.InverterQuantity_PV_TOTAL_ENERGY_KWH:       {"gohome_growatt_inverter_pv_total_energy_kwh", "Lifetime PV input energy in kWh"},
	v1.InverterQuantity_BUS_VOLTAGE_VOLTS:         {"gohome_growatt_inverter_bus_voltage_volts", "DC bus voltage in volts"},
	v1.InverterQuantity_OPERATING_TIME_RAW:        {"gohome_growatt_inverter_operating_time_raw", "Raw timeTotal register; unit not verified"},
	v1.InverterQuantity_OUTPUT_LIMIT_RAW:          {"gohome_growatt_inverter_output_limit_raw", "Raw opFullwatt output limit; scale not verified"},
	v1.InverterQuantity_OUTPUT_PERCENT:            {"gohome_growatt_inverter_output_percent", "Reported output percentage"},
	v1.InverterQuantity_CONNECTION_DELAY_RAW:      {"gohome_growatt_inverter_connection_delay_raw", "Raw grid connection countdown; unit not verified"},
	v1.InverterQuantity_DERATING_MODE:             {"gohome_growatt_inverter_derating_mode", "Reported derating mode code"},
	v1.InverterQuantity_STATUS:                    {"gohome_growatt_inverter_status", "Reported inverter status code"},
	v1.InverterQuantity_OPERATING_MODE:            {"gohome_growatt_inverter_operating_mode", "Reported operating mode code"},
	v1.InverterQuantity_SYSTEM_WORK_MODE:          {"gohome_growatt_inverter_system_work_mode", "Reported system work mode code"},
	v1.InverterQuantity_DRY_CONTACT_STATUS:        {"gohome_growatt_inverter_dry_contact_status", "Reported dry contact status code"},
	v1.InverterQuantity_LEAKAGE_CURRENT_RAW:       {"gohome_growatt_inverter_leakage_current_raw", "Raw grid leakage current register; scale not verified"},
	v1.InverterQuantity_INSULATION_RESISTANCE_RAW: {"gohome_growatt_inverter_insulation_resistance_raw", "Raw PV insulation register; scale and sentinel values not verified"},
	v1.InverterQuantity_GRID_VOLTAGE_VOLTS:        {"gohome_growatt_inverter_grid_voltage_volts", "AC phase voltage in volts"},
	v1.InverterQuantity_GRID_CURRENT_AMPERES:      {"gohome_growatt_inverter_grid_current_amperes", "AC phase current in amperes"},
	v1.InverterQuantity_PHASE_APPARENT_POWER_VA:   {"gohome_growatt_inverter_phase_apparent_power_va", "Reported phase apparent power in VA"},
	v1.InverterQuantity_STRING_VOLTAGE_VOLTS:      {"gohome_growatt_inverter_string_voltage_volts", "PV input voltage in volts"},
	v1.InverterQuantity_STRING_CURRENT_AMPERES:    {"gohome_growatt_inverter_string_current_amperes", "PV input current in amperes"},
	v1.InverterQuantity_STRING_POWER_WATTS:        {"gohome_growatt_inverter_string_power_watts", "PV input power in watts"},
	v1.InverterQuantity_STRING_TODAY_ENERGY_KWH:   {"gohome_growatt_inverter_string_today_energy_kwh", "PV input today energy in kWh"},
	v1.InverterQuantity_STRING_TOTAL_ENERGY_KWH:   {"gohome_growatt_inverter_string_total_energy_kwh", "PV input total energy in kWh"},
	v1.InverterQuantity_TEMPERATURE_CELSIUS:       {"gohome_growatt_inverter_temperature_celsius", "Inverter temperature channel in degrees Celsius"},
	v1.InverterQuantity_FAULT_WORD:                {"gohome_growatt_inverter_fault_word", "Raw system fault bitmask"},
	v1.InverterQuantity_DIAGNOSTIC_CODE:           {"gohome_growatt_inverter_diagnostic_code", "Raw vendor diagnostic code"},
}

var inverterFields = []inverterField{
	{"fac", v1.InverterQuantity_GRID_FREQUENCY_HERTZ, ""},
	{"pac", v1.InverterQuantity_OUTPUT_POWER_WATTS, ""},
	{"ppv", v1.InverterQuantity_PV_POWER_WATTS, ""},
	{"pf", v1.InverterQuantity_POWER_FACTOR, ""},
	{"eacToday", v1.InverterQuantity_TODAY_ENERGY_KWH, ""},
	{"eacTotal", v1.InverterQuantity_TOTAL_ENERGY_KWH, ""},
	{"epvTotal", v1.InverterQuantity_PV_TOTAL_ENERGY_KWH, ""},
	{"pBusVoltage", v1.InverterQuantity_BUS_VOLTAGE_VOLTS, ""},
	{"timeTotal", v1.InverterQuantity_OPERATING_TIME_RAW, ""},
	{"opFullwatt", v1.InverterQuantity_OUTPUT_LIMIT_RAW, ""},
	{"realOPPercent", v1.InverterQuantity_OUTPUT_PERCENT, ""},
	{"invDelayTime", v1.InverterQuantity_CONNECTION_DELAY_RAW, ""},
	{"deratingMode", v1.InverterQuantity_DERATING_MODE, ""},
	{"status", v1.InverterQuantity_STATUS, ""},
	{"operatingMode", v1.InverterQuantity_OPERATING_MODE, ""},
	{"uwSysWorkMode", v1.InverterQuantity_SYSTEM_WORK_MODE, ""},
	{"dryContactStatus", v1.InverterQuantity_DRY_CONTACT_STATUS, ""},
	{"gfci", v1.InverterQuantity_LEAKAGE_CURRENT_RAW, ""},
	{"iso", v1.InverterQuantity_INSULATION_RESISTANCE_RAW, ""},
	{"vac1", v1.InverterQuantity_GRID_VOLTAGE_VOLTS, "1"},
	{"iac1", v1.InverterQuantity_GRID_CURRENT_AMPERES, "1"},
	{"pac1", v1.InverterQuantity_PHASE_APPARENT_POWER_VA, "1"},
	{"vac2", v1.InverterQuantity_GRID_VOLTAGE_VOLTS, "2"},
	{"iac2", v1.InverterQuantity_GRID_CURRENT_AMPERES, "2"},
	{"pac2", v1.InverterQuantity_PHASE_APPARENT_POWER_VA, "2"},
	{"vac3", v1.InverterQuantity_GRID_VOLTAGE_VOLTS, "3"},
	{"iac3", v1.InverterQuantity_GRID_CURRENT_AMPERES, "3"},
	{"pac3", v1.InverterQuantity_PHASE_APPARENT_POWER_VA, "3"},
	{"vpv1", v1.InverterQuantity_STRING_VOLTAGE_VOLTS, "1"},
	{"ipv1", v1.InverterQuantity_STRING_CURRENT_AMPERES, "1"},
	{"ppv1", v1.InverterQuantity_STRING_POWER_WATTS, "1"},
	{"epv1Today", v1.InverterQuantity_STRING_TODAY_ENERGY_KWH, "1"},
	{"epv1Total", v1.InverterQuantity_STRING_TOTAL_ENERGY_KWH, "1"},
	{"vpv2", v1.InverterQuantity_STRING_VOLTAGE_VOLTS, "2"},
	{"ipv2", v1.InverterQuantity_STRING_CURRENT_AMPERES, "2"},
	{"ppv2", v1.InverterQuantity_STRING_POWER_WATTS, "2"},
	{"epv2Today", v1.InverterQuantity_STRING_TODAY_ENERGY_KWH, "2"},
	{"epv2Total", v1.InverterQuantity_STRING_TOTAL_ENERGY_KWH, "2"},
	{"vpv3", v1.InverterQuantity_STRING_VOLTAGE_VOLTS, "3"},
	{"ipv3", v1.InverterQuantity_STRING_CURRENT_AMPERES, "3"},
	{"ppv3", v1.InverterQuantity_STRING_POWER_WATTS, "3"},
	{"epv3Today", v1.InverterQuantity_STRING_TODAY_ENERGY_KWH, "3"},
	{"epv3Total", v1.InverterQuantity_STRING_TOTAL_ENERGY_KWH, "3"},
	{"vpv4", v1.InverterQuantity_STRING_VOLTAGE_VOLTS, "4"},
	{"ipv4", v1.InverterQuantity_STRING_CURRENT_AMPERES, "4"},
	{"ppv4", v1.InverterQuantity_STRING_POWER_WATTS, "4"},
	{"epv4Today", v1.InverterQuantity_STRING_TODAY_ENERGY_KWH, "4"},
	{"epv4Total", v1.InverterQuantity_STRING_TOTAL_ENERGY_KWH, "4"},
	{"temp1", v1.InverterQuantity_TEMPERATURE_CELSIUS, "1"},
	{"temp2", v1.InverterQuantity_TEMPERATURE_CELSIUS, "2"},
	{"temp3", v1.InverterQuantity_TEMPERATURE_CELSIUS, "3"},
	{"temp4", v1.InverterQuantity_TEMPERATURE_CELSIUS, "4"},
	{"temp5", v1.InverterQuantity_TEMPERATURE_CELSIUS, "5"},
	{"sysFaultWord", v1.InverterQuantity_FAULT_WORD, "0"},
	{"sysFaultWord1", v1.InverterQuantity_FAULT_WORD, "1"},
	{"sysFaultWord2", v1.InverterQuantity_FAULT_WORD, "2"},
	{"sysFaultWord3", v1.InverterQuantity_FAULT_WORD, "3"},
	{"sysFaultWord4", v1.InverterQuantity_FAULT_WORD, "4"},
	{"sysFaultWord5", v1.InverterQuantity_FAULT_WORD, "5"},
	{"sysFaultWord6", v1.InverterQuantity_FAULT_WORD, "6"},
	{"sysFaultWord7", v1.InverterQuantity_FAULT_WORD, "7"},
	{"faultType", v1.InverterQuantity_DIAGNOSTIC_CODE, "faultType"},
	{"faultType1", v1.InverterQuantity_DIAGNOSTIC_CODE, "faultType1"},
	{"warnCode", v1.InverterQuantity_DIAGNOSTIC_CODE, "warnCode"},
	{"warnCode1", v1.InverterQuantity_DIAGNOSTIC_CODE, "warnCode1"},
	{"newWarnCode", v1.InverterQuantity_DIAGNOSTIC_CODE, "newWarnCode"},
	{"newWarnSubCode", v1.InverterQuantity_DIAGNOSTIC_CODE, "newWarnSubCode"},
}

func decodeInverterTelemetry(row map[string]any, serial, timezone string) (*v1.InverterTelemetry, error) {
	at, err := parseGrowattTimestamp(parseString(row["time"]), timezone)
	if err != nil || at == nil {
		return nil, fmt.Errorf("inverter source timestamp missing or invalid")
	}
	result := &v1.InverterTelemetry{Serial: serial, ObservedAt: at.Format(time.RFC3339)}
	for _, field := range inverterFields {
		// The supported MIN has one AC phase and two PV inputs. Select hardware
		// channels, not nonzero values: a grid outage must retain a real zero.
		switch field.quantity {
		case v1.InverterQuantity_GRID_VOLTAGE_VOLTS, v1.InverterQuantity_GRID_CURRENT_AMPERES, v1.InverterQuantity_PHASE_APPARENT_POWER_VA:
			if field.channel != "1" {
				continue
			}
		case v1.InverterQuantity_STRING_VOLTAGE_VOLTS, v1.InverterQuantity_STRING_CURRENT_AMPERES, v1.InverterQuantity_STRING_POWER_WATTS, v1.InverterQuantity_STRING_TODAY_ENERGY_KWH, v1.InverterQuantity_STRING_TOTAL_ENERGY_KWH:
			if field.channel != "1" && field.channel != "2" {
				continue
			}
		case v1.InverterQuantity_TEMPERATURE_CELSIUS:
			// This MIN's populated channels are 1, 2 and 5. Channels 3/4 are placeholders.
			if field.channel == "3" || field.channel == "4" {
				continue
			}
		}
		if value, ok := parseFloatOK(row[field.source]); ok {
			result.Measurements = append(result.Measurements, &v1.InverterMeasurement{Quantity: field.quantity, Channel: field.channel, Value: value})
		}
	}
	return result, nil
}

func inverterDescriptors() map[v1.InverterQuantity]*prometheus.Desc {
	result := make(map[v1.InverterQuantity]*prometheus.Desc)
	for quantity, metric := range inverterMetrics {
		result[quantity] = prometheus.NewDesc(metric.name, metric.help, []string{"plant_id", "plant_name", "serial", "channel"}, nil)
	}
	return result
}

func collectInverter(telemetry *v1.InverterTelemetry, plantID, plantName string, descriptors map[v1.InverterQuantity]*prometheus.Desc, ch chan<- prometheus.Metric) {
	if telemetry == nil {
		return
	}
	at, err := time.Parse(time.RFC3339, telemetry.ObservedAt)
	if err != nil || time.Since(at) > 15*time.Minute {
		return
	}
	for _, reading := range telemetry.Measurements {
		metric := prometheus.MustNewConstMetric(descriptors[reading.Quantity], prometheus.GaugeValue, reading.Value, plantID, plantName, telemetry.Serial, reading.Channel)
		ch <- prometheus.NewMetricWithTimestamp(at, metric)
	}
}
