package p1_homewizard

import (
	"fmt"
	"strings"

	p1v1 "github.com/joshp123/gohome/proto/gen/plugins/p1_homewizard/v1"
)

const (
	defaultBaseURL = "http://192.168.1.48"
)

// Config defines runtime configuration for the P1 Homewizard client.
type Config struct {
	BaseURL string
	Tariffs Tariffs
}

// Tariffs define cost rates in EUR per kWh.
type Tariffs struct {
	ImportT1EurPerKWh float64
	ImportT2EurPerKWh float64
	ExportT1EurPerKWh float64
	ExportT2EurPerKWh float64
}

func (t Tariffs) Configured() bool {
	return t.ImportT1EurPerKWh != 0 || t.ImportT2EurPerKWh != 0 || t.ExportT1EurPerKWh != 0 || t.ExportT2EurPerKWh != 0
}

func ConfigFromProto(cfg *p1v1.P1HomewizardConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("p1_homewizard config is required")
	}

	baseURL := strings.TrimSpace(cfg.BaseUrl)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return Config{
		BaseURL: baseURL,
		Tariffs: Tariffs{
			ImportT1EurPerKWh: cfg.TariffImportT1EurPerKwh,
			ImportT2EurPerKWh: cfg.TariffImportT2EurPerKwh,
			ExportT1EurPerKWh: cfg.TariffExportT1EurPerKwh,
			ExportT2EurPerKWh: cfg.TariffExportT2EurPerKwh,
		},
	}, nil
}
