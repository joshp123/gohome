package plugins

import (
	"github.com/joshp123/gohome/internal/core"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

// Factory builds a plugin instance from the loaded config.
type Factory func(*configv1.Config) (core.Plugin, bool)

var compiled []Factory

// Register adds a compiled-in plugin factory to the registry.
func Register(factory Factory) {
	compiled = append(compiled, factory)
}

// Compiled returns the configured plugin instances for this build.
func Compiled(cfg *configv1.Config) []core.Plugin {
	if cfg == nil {
		return nil
	}
	out := make([]core.Plugin, 0, len(compiled))
	for _, factory := range compiled {
		plugin, ok := factory(cfg)
		if !ok {
			continue
		}
		out = append(out, plugin)
	}
	return out
}
