//go:build gohome_plugin_hikvision

package plugins

import (
	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/plugins/hikvision"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

func init() {
	Register(func(cfg *configv1.Config) (core.Plugin, bool) {
		return hikvision.NewPlugin(cfg.GetHikvision(), cfg.GetOauth())
	})
}
