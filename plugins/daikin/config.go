package daikin

import (
	"fmt"
	"os"
)

const (
	defaultBaseURL = "https://api.onecta.daikineurope.com"
)

// Config defines runtime configuration for the Daikin client.
type Config struct {
	BaseURL       string
	BootstrapFile string
}

// LoadConfigFromEnv builds a config from environment variables and bootstrap file.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:       envOrDefault("GOHOME_DAIKIN_BASE_URL", defaultBaseURL),
		BootstrapFile: os.Getenv("GOHOME_DAIKIN_BOOTSTRAP_FILE"),
	}

	if cfg.BootstrapFile == "" {
		return Config{}, fmt.Errorf("GOHOME_DAIKIN_BOOTSTRAP_FILE is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
