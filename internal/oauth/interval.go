package oauth

import (
	"time"

	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

const DefaultRefreshInterval = 10 * time.Minute

func RefreshInterval(cfg *configv1.OAuthConfig) time.Duration {
	if cfg == nil {
		return DefaultRefreshInterval
	}
	if cfg.RefreshEnabled != nil && !cfg.GetRefreshEnabled() {
		return 0
	}
	if cfg.RefreshIntervalSeconds > 0 {
		return time.Duration(cfg.RefreshIntervalSeconds) * time.Second
	}
	return DefaultRefreshInterval
}
