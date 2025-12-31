package tado

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

const (
	defaultBaseURL = "https://my.tado.com/api/v2"
	defaultAuthURL = "https://login.tado.com/oauth2/token"
)

// Config defines runtime configuration for the Tado client.
type Config struct {
	BaseURL      string
	AuthURL      string
	TokenFile    string
	HomeID       int
	ClientID     string
	ClientSecret string
	RefreshToken string
	Scope        string
}

type tokenFile struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// LoadConfigFromEnv builds a config from environment variables and token file.
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:   envOrDefault("GOHOME_TADO_BASE_URL", defaultBaseURL),
		AuthURL:   envOrDefault("GOHOME_TADO_AUTH_URL", defaultAuthURL),
		TokenFile: os.Getenv("GOHOME_TADO_TOKEN_FILE"),
	}

	if cfg.TokenFile == "" {
		return Config{}, fmt.Errorf("GOHOME_TADO_TOKEN_FILE is required")
	}

	if homeID := os.Getenv("GOHOME_TADO_HOME_ID"); homeID != "" {
		parsed, err := strconv.Atoi(homeID)
		if err != nil {
			return Config{}, fmt.Errorf("invalid GOHOME_TADO_HOME_ID: %w", err)
		}
		cfg.HomeID = parsed
	}

	data, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return Config{}, fmt.Errorf("read token file: %w", err)
	}

	var token tokenFile
	if err := json.Unmarshal(data, &token); err != nil {
		return Config{}, fmt.Errorf("parse token file: %w", err)
	}

	if token.ClientID == "" || token.RefreshToken == "" {
		return Config{}, fmt.Errorf("token file missing client_id or refresh_token")
	}

	cfg.ClientID = token.ClientID
	cfg.ClientSecret = token.ClientSecret
	cfg.RefreshToken = token.RefreshToken
	cfg.Scope = token.Scope

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
