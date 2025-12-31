package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// DashboardsMap materializes dashboard content to URL paths.
func DashboardsMap(plugins []Plugin) map[string][]byte {
	result := make(map[string][]byte)
	for _, plugin := range plugins {
		manifest := plugin.Manifest()
		for _, dash := range plugin.Dashboards() {
			path := "/dashboards/" + manifest.PluginID + "/" + dash.Name + ".json"
			result[path] = dash.JSON
		}
	}
	return result
}

// WriteDashboards writes dashboards to disk for Grafana provisioning.
func WriteDashboards(dir string, plugins []Plugin) error {
	if dir == "" {
		return nil
	}

	for _, plugin := range plugins {
		manifest := plugin.Manifest()
		for _, dash := range plugin.Dashboards() {
			pluginDir := filepath.Join(dir, manifest.PluginID)
			if err := os.MkdirAll(pluginDir, 0o755); err != nil {
				return fmt.Errorf("create dashboard dir: %w", err)
			}
			path := filepath.Join(pluginDir, dash.Name+".json")
			if err := os.WriteFile(path, dash.JSON, 0o644); err != nil {
				return fmt.Errorf("write dashboard %s: %w", path, err)
			}
		}
	}

	return nil
}
