//go:build gohome_plugin_daikin

package plugins

import "github.com/joshp123/gohome/plugins/daikin"

func init() {
	Register(daikin.NewPlugin())
}
