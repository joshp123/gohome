//go:build gohome_plugin_daikin

package plugins

import (
	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/plugins/daikin"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

func init() {
	Register(func(cfg *configv1.Config) (core.Plugin, bool) {
		return daikin.NewPlugin(cfg.GetDaikin(), cfg.GetOauth())
	})
}
