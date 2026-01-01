package daikin

import (
	"fmt"

	daikinv1 "github.com/joshp123/gohome/proto/gen/plugins/daikin/v1"
)

const (
	defaultBaseURL = "https://api.onecta.daikineurope.com"
)

// Config defines runtime configuration for the Daikin client.
type Config struct {
	BaseURL       string
	BootstrapFile string
}

func ConfigFromProto(cfg *daikinv1.DaikinConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("daikin config is required")
	}
	if cfg.BootstrapFile == "" {
		return Config{}, fmt.Errorf("daikin bootstrap_file is required")
	}

	return Config{
		BaseURL:       defaultBaseURL,
		BootstrapFile: cfg.BootstrapFile,
	}, nil
}
