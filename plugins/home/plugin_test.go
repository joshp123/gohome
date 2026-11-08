package home

import "testing"

// Every listed House dashboard must be embedded, or gohome stops at start-up.
func TestEveryHouseDashboardIsEmbedded(t *testing.T) {
	for _, name := range houseDashboards {
		if _, err := dashboardFiles.ReadFile("dashboards/house-" + name + ".json"); err != nil {
			t.Errorf("house dashboard %q is listed but not embedded: %v", name, err)
		}
	}
}
