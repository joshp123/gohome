package hikvision

import (
	"fmt"
	"os"
	"strings"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

// Config is the runtime configuration for one camera.
type Config struct {
	BaseURL  string
	Username string
	Password string
	// AlarmPollInterval is how often the alarm log is read; 0 means it is not polled.
	AlarmPollInterval time.Duration
}

// ConfigFromProto reads the credentials file named in the config.
func ConfigFromProto(cfg *hikvisionv1.HikvisionConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("hikvision config is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseUrl), "/")
	if baseURL == "" {
		return Config{}, fmt.Errorf("hikvision base_url is required")
	}
	if cfg.CredentialsFile == "" {
		return Config{}, fmt.Errorf("hikvision credentials_file is required")
	}
	raw, err := os.ReadFile(cfg.CredentialsFile)
	if err != nil {
		return Config{}, fmt.Errorf("read hikvision credentials_file: %w", err)
	}
	username, password, err := parseCredentials(string(raw))
	if err != nil {
		return Config{}, fmt.Errorf("hikvision credentials_file: %w", err)
	}
	return Config{BaseURL: baseURL, Username: username, Password: password,
		AlarmPollInterval: time.Duration(cfg.AlarmPollSeconds) * time.Second}, nil
}

// parseCredentials reads "username:password". Camera usernames cannot contain
// a colon, so the first colon separates them and the password may contain more.
func parseCredentials(raw string) (string, string, error) {
	line := strings.TrimRight(raw, "\r\n")
	username, password, found := strings.Cut(line, ":")
	if !found || username == "" || password == "" {
		return "", "", fmt.Errorf(`want one line "username:password"`)
	}
	return username, password, nil
}
