package sonos

import (
	"fmt"
	"strings"

	sonosv1 "github.com/joshp123/gohome/proto/gen/plugins/sonos/v1"
)

// Config defines runtime configuration for the Sonos client.
type Config struct {
	BaseURL string
}

func ConfigFromProto(cfg *sonosv1.SonosConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("sonos config is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseUrl), "/")
	if baseURL == "" {
		return Config{}, fmt.Errorf("sonos base_url is required")
	}
	return Config{BaseURL: baseURL}, nil
}
