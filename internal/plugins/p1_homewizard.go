//go:build gohome_plugin_p1_homewizard

package plugins

import (
	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/plugins/p1_homewizard"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

func init() {
	Register(func(cfg *configv1.Config) (core.Plugin, bool) {
		return p1_homewizard.NewPlugin(cfg.GetP1Homewizard(), cfg.GetOauth())
	})
}
