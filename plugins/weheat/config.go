package weheat

import (
	"fmt"
	"strings"

	weheatv1 "github.com/joshp123/gohome/proto/gen/plugins/weheat/v1"
)

const defaultBaseURL = "https://api.weheat.nl"

// Config defines runtime configuration for the Weheat client.
type Config struct {
	BaseURL string
}

func ConfigFromProto(cfg *weheatv1.WeheatConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("weheat config is required")
	}

	baseURL := strings.TrimSpace(cfg.GetBaseUrl())
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return Config{BaseURL: baseURL}, nil
}
