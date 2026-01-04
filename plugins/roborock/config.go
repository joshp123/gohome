package roborock

import (
	"fmt"

	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
)

// Config defines runtime configuration for the Roborock client.
type Config struct {
	BootstrapFile string
	CloudFallback bool
	IPOverrides   map[string]string
}

func ConfigFromProto(cfg *roborockv1.RoborockConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("roborock config is required")
	}
	if cfg.BootstrapFile == "" {
		return Config{}, fmt.Errorf("roborock bootstrap_file is required")
	}

	return Config{
		BootstrapFile: cfg.BootstrapFile,
		CloudFallback: cfg.CloudFallback,
		IPOverrides:   cfg.DeviceIpOverrides,
	}, nil
}
