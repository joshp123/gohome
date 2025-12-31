//go:build gohome_plugin_tado

package plugins

import "github.com/joshp123/gohome/plugins/tado"

func init() {
	Register(tado.NewPlugin())
}
