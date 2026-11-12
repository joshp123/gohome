package airgradient

import (
	"fmt"
	"strings"

	airgradientv1 "github.com/joshp123/gohome/proto/gen/plugins/airgradient/v1"
)

// Config defines runtime configuration for the AirGradient client.
type Config struct {
	BaseURL string
}

func ConfigFromProto(cfg *airgradientv1.AirgradientConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("airgradient config is required")
	}

	baseURL := strings.TrimSpace(cfg.BaseUrl)
	if baseURL == "" {
		return Config{}, fmt.Errorf("airgradient base_url is required")
	}

	return Config{BaseURL: baseURL}, nil
}
