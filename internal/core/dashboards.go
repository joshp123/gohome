package core

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
