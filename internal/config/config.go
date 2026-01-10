package config

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

const (
	SchemaVersion                      = 1
	DefaultPath                        = "/etc/gohome/config.pbtxt"
	DefaultGRPCAddr                    = "0.0.0.0:9000"
	DefaultHTTPAddr                    = "0.0.0.0:8080"
	DefaultDashboardDir                = "/var/lib/gohome/dashboards"
	DefaultOAuthPrefix                 = "gohome/oauth"
	DefaultOAuthRefreshIntervalSeconds = 600
)

// Load parses the textproto config file, applies defaults, and validates.
func Load(path string) (*configv1.Config, error) {
	var (
		data []byte
		err  error
	)

	data, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &configv1.Config{}
	if err = prototext.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(cfg)
	if err = Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyDefaults(cfg *configv1.Config) {
	if cfg.Core == nil {
		cfg.Core = &configv1.CoreConfig{}
	}
	if cfg.Core.GrpcAddr == "" {
		cfg.Core.GrpcAddr = DefaultGRPCAddr
	}
	if cfg.Core.HttpAddr == "" {
		cfg.Core.HttpAddr = DefaultHTTPAddr
	}
	if cfg.Core.DashboardDir == "" {
		cfg.Core.DashboardDir = DefaultDashboardDir
	}

	if cfg.Oauth == nil {
		cfg.Oauth = &configv1.OAuthConfig{}
	}
	if cfg.Oauth.BlobPrefix == "" {
		cfg.Oauth.BlobPrefix = DefaultOAuthPrefix
	}
	if cfg.Oauth.RefreshEnabled == nil {
		cfg.Oauth.RefreshEnabled = proto.Bool(true)
	}
	if cfg.Oauth.RefreshIntervalSeconds == 0 {
		cfg.Oauth.RefreshIntervalSeconds = DefaultOAuthRefreshIntervalSeconds
	}
}

// Validate enforces required invariants beyond proto typing.
func Validate(cfg *configv1.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if cfg.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}

	if cfg.Core == nil {
		return fmt.Errorf("core config is required")
	}
	if cfg.Core.GrpcAddr == "" {
		return fmt.Errorf("core.grpc_addr is required")
	}
	if cfg.Core.HttpAddr == "" {
		return fmt.Errorf("core.http_addr is required")
	}
	if cfg.Core.DashboardDir == "" {
		return fmt.Errorf("core.dashboard_dir is required")
	}

	if cfg.Oauth == nil {
		return fmt.Errorf("oauth config is required")
	}
	if cfg.Oauth.BlobEndpoint == "" {
		return fmt.Errorf("oauth.blob_endpoint is required")
	}
	if cfg.Oauth.BlobBucket == "" {
		return fmt.Errorf("oauth.blob_bucket is required")
	}
	if cfg.Oauth.BlobAccessKeyFile == "" {
		return fmt.Errorf("oauth.blob_access_key_file is required")
	}
	if cfg.Oauth.BlobSecretKeyFile == "" {
		return fmt.Errorf("oauth.blob_secret_key_file is required")
	}

	if cfg.Tado != nil && cfg.Tado.BootstrapFile == "" {
		return fmt.Errorf("tado.bootstrap_file is required")
	}
	if cfg.Daikin != nil && cfg.Daikin.BootstrapFile == "" {
		return fmt.Errorf("daikin.bootstrap_file is required")
	}
	if cfg.Growatt != nil && cfg.Growatt.TokenFile == "" {
		return fmt.Errorf("growatt.token_file is required")
	}
	if cfg.Roborock != nil && cfg.Roborock.BootstrapFile == "" {
		return fmt.Errorf("roborock.bootstrap_file is required")
	}

	return nil
}

// EnabledPlugins maps enabled plugin IDs based on config presence.
func EnabledPlugins(cfg *configv1.Config) map[string]bool {
	enabled := make(map[string]bool)
	if cfg == nil {
		return enabled
	}
	if cfg.Tado != nil {
		enabled["tado"] = true
	}
	if cfg.Daikin != nil {
		enabled["daikin"] = true
	}
	if cfg.Growatt != nil {
		enabled["growatt"] = true
	}
	if cfg.Roborock != nil {
		enabled["roborock"] = true
	}
	if cfg.P1Homewizard != nil {
		enabled["p1_homewizard"] = true
	}
	return enabled
}

// BootstrapPathForProvider resolves the bootstrap file path from config.
func BootstrapPathForProvider(cfg *configv1.Config, provider string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is required")
	}
	switch provider {
	case "tado":
		if cfg.Tado == nil || cfg.Tado.BootstrapFile == "" {
			return "", fmt.Errorf("tado bootstrap_file is required")
		}
		return cfg.Tado.BootstrapFile, nil
	case "daikin":
		if cfg.Daikin == nil || cfg.Daikin.BootstrapFile == "" {
			return "", fmt.Errorf("daikin bootstrap_file is required")
		}
		return cfg.Daikin.BootstrapFile, nil
	case "roborock":
		if cfg.Roborock == nil || cfg.Roborock.BootstrapFile == "" {
			return "", fmt.Errorf("roborock bootstrap_file is required")
		}
		return cfg.Roborock.BootstrapFile, nil
	default:
		return "", fmt.Errorf("unknown provider %q", provider)
	}
}
