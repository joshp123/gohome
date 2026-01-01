package tado

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultBaseURL = "https://my.tado.com/api/v2"
)

// Config defines runtime configuration for the Tado client.
type Config struct {
	BaseURL       string
	BootstrapFile string
	HomeID        int
}

// LoadConfigFromEnv builds a config from environment variables and bootstrap file.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:       envOrDefault("GOHOME_TADO_BASE_URL", defaultBaseURL),
		BootstrapFile: os.Getenv("GOHOME_TADO_BOOTSTRAP_FILE"),
	}

	if cfg.BootstrapFile == "" {
		return Config{}, fmt.Errorf("GOHOME_TADO_BOOTSTRAP_FILE is required")
	}

	if homeID := os.Getenv("GOHOME_TADO_HOME_ID"); homeID != "" {
		parsed, err := strconv.Atoi(homeID)
		if err != nil {
			return Config{}, fmt.Errorf("invalid GOHOME_TADO_HOME_ID: %w", err)
		}
		cfg.HomeID = parsed
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
