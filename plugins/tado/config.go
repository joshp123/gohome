package tado

import (
	"fmt"

	tadov1 "github.com/joshp123/gohome/proto/gen/plugins/tado/v1"
)

const (
	defaultBaseURL = "https://my.tado.com/api/v2"
)

// Config defines runtime configuration for the Tado client.
type Config struct {
	BaseURL       string
	BootstrapFile string
	HomeID        *int
}

func ConfigFromProto(cfg *tadov1.TadoConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("tado config is required")
	}
	if cfg.BootstrapFile == "" {
		return Config{}, fmt.Errorf("tado bootstrap_file is required")
	}

	var homeID *int
	if cfg.HomeId != nil {
		value := int(cfg.GetHomeId())
		if value <= 0 {
			return Config{}, fmt.Errorf("tado home_id must be positive")
		}
		homeID = &value
	}

	return Config{
		BaseURL:       defaultBaseURL,
		BootstrapFile: cfg.BootstrapFile,
		HomeID:        homeID,
	}, nil
}
