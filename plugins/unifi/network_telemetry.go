package unifi

import (
	"strconv"
	"strings"
	"time"
)

// SubsystemHealth holds one UniFi site health entry (wan, lan, wlan, www).
type SubsystemHealth struct {
	Subsystem              string
	Status                 string
	ActiveUsers            *uint64
	ActiveGuests           *uint64
	AdoptedDevices         *uint64
	DisconnectedDevices    *uint64
	TxBytesPerSecond       *float64
	RxBytesPerSecond       *float64
	WANLatencyMilliseconds *float64
	WANUptimeSeconds       *float64
	WANIP                  string
	ISPName                string
}

// NetworkDevice holds low-cardinality health telemetry for one adopted UniFi gateway, switch, or AP.
type NetworkDevice struct {
	MAC                 string
	Name                string
	Model               string
	DeviceType          string
	IP                  string
	Version             string
	Online              bool
	UptimeSeconds       *float64
	CPUPercent          *float64
	MemoryPercent       *float64
	TemperatureCelsius  *float64
	ActiveClients       *uint64
	SatisfactionPercent *float64
}

// SiteSummary aggregates one read of UniFi OS + UniFi Network health, devices, and active client counts.
type SiteSummary struct {
	ControllerName    string
	HardwareShortname string
	Site              string
	WiredClients      uint64
	WirelessClients   uint64
	Subsystems        []SubsystemHealth
	Devices           []NetworkDevice
	ObservedAt        time.Time
}

type systemResponse struct {
	Name     string `json:"name"`
	Hardware struct {
		Shortname string `json:"shortname"`
	} `json:"hardware"`
}

type healthEnvelope struct {
	Data []healthWireEntry `json:"data"`
}

type healthWireEntry struct {
	Subsystem       string   `json:"subsystem"`
	Status          string   `json:"status"`
	NumUser         *uint64  `json:"num_user"`
	NumGuest        *uint64  `json:"num_guest"`
	NumAdopted      *uint64  `json:"num_adopted"`
	NumDisconnected *uint64  `json:"num_disconnected"`
	TxBytesRate     *float64 `json:"tx_bytes-r"`
	RxBytesRate     *float64 `json:"rx_bytes-r"`
	Latency         *float64 `json:"latency"`
	Uptime          *float64 `json:"uptime"`
	WANIP           string   `json:"wan_ip"`
	ISPName         string   `json:"isp_name"`
}

type deviceEnvelope struct {
	Data []deviceWireEntry `json:"data"`
}

type deviceWireEntry struct {
	MAC                 string   `json:"mac"`
	Name                string   `json:"name"`
	Model               string   `json:"model"`
	Type                string   `json:"type"`
	IP                  string   `json:"ip"`
	Version             string   `json:"version"`
	State               int      `json:"state"`
	Uptime              *float64 `json:"uptime"`
	NumSta              *uint64  `json:"num_sta"`
	Satisfaction        *float64 `json:"satisfaction"`
	GeneralTemperature  *float64 `json:"general_temperature"`
	SystemStats         struct {
		CPU string `json:"cpu"`
		Mem string `json:"mem"`
	} `json:"system-stats"`
}

type clientEnvelope struct {
	Data []clientWireEntry `json:"data"`
}

type clientWireEntry struct {
	IsWired bool `json:"is_wired"`
}

func parsePercentString(raw string) *float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &v
}
