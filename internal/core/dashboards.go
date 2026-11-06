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

// WriteDashboards writes dashboards to disk for Grafana provisioning, one directory per
// Grafana folder. It first clears the directory, so dashboards of removed plugins disappear.
func WriteDashboards(dir string, plugins []Plugin) error {
	if dir == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read dashboard dir: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("clear dashboard dir: %w", err)
		}
	}

	for _, plugin := range plugins {
		manifest := plugin.Manifest()
		for _, dash := range plugin.Dashboards() {
			folderDir := filepath.Join(dir, dash.Folder)
			if err := os.MkdirAll(folderDir, 0o755); err != nil {
				return fmt.Errorf("create dashboard dir: %w", err)
			}
			path := filepath.Join(folderDir, manifest.PluginID+"-"+dash.Name+".json")
			if err := os.WriteFile(path, dash.JSON, 0o644); err != nil {
				return fmt.Errorf("write dashboard %s: %w", path, err)
			}
		}
	}

	return nil
}
