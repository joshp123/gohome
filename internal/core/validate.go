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

// ValidateEnabledPlugins ensures enabled plugin IDs exist in the compiled list.
func ValidateEnabledPlugins(compiled []Plugin, enabled map[string]bool, allowAll bool) error {
	if allowAll || len(enabled) == 0 {
		return nil
	}

	compiledIDs := make(map[string]bool, len(compiled))
	for _, plugin := range compiled {
		compiledIDs[plugin.ID()] = true
	}

	for id := range enabled {
		if !compiledIDs[id] {
			return fmt.Errorf("enabled plugin %q is not compiled into this build", id)
		}
	}

	return nil
}

// FilterPlugins selects the active plugins based on the enabled set.
func FilterPlugins(compiled []Plugin, enabled map[string]bool, allowAll bool) []Plugin {
	if allowAll {
		return compiled
	}
	if len(enabled) == 0 {
		return nil
	}

	active := make([]Plugin, 0, len(compiled))
	for _, plugin := range compiled {
		if enabled[plugin.ID()] {
			active = append(active, plugin)
		}
	}

	return active
}
