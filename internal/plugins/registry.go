package plugins

import "github.com/joshp123/gohome/internal/core"

var compiled []core.Plugin

// Register adds a compiled-in plugin to the registry.
func Register(plugin core.Plugin) {
	compiled = append(compiled, plugin)
}

// Compiled returns the compiled-in plugins.
func Compiled() []core.Plugin {
	out := make([]core.Plugin, len(compiled))
	copy(out, compiled)
	return out
}
