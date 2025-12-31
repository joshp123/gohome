package core

import (
	fmt "fmt"
	"regexp"
)

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]+$`)

// ValidatePlugins enforces basic plugin contract invariants at startup.
func ValidatePlugins(plugins []Plugin) error {
	seen := make(map[string]bool)
	for _, plugin := range plugins {
		id := plugin.ID()
		manifest := plugin.Manifest()
		if id == "" {
			return fmt.Errorf("plugin id is empty")
		}
		if !pluginIDPattern.MatchString(id) {
			return fmt.Errorf("plugin id %q does not match %s", id, pluginIDPattern.String())
		}
		if manifest.PluginID != id {
			return fmt.Errorf("plugin id mismatch: id=%q manifest=%q", id, manifest.PluginID)
		}
		if seen[id] {
			return fmt.Errorf("duplicate plugin id: %s", id)
		}
		seen[id] = true
	}
	return nil
}
