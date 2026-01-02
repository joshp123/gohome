package growatt

import (
	"fmt"
	"strings"

	growattv1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
)

const (
	defaultRegion = "other_regions"
)

var regionEndpoints = map[string]string{
	"other_regions":         "https://openapi.growatt.com/",
	"north_america":         "https://openapi-us.growatt.com/",
	"australia_new_zealand": "https://openapi-au.growatt.com/",
	"china":                 "https://openapi-cn.growatt.com/",
}

// Config defines runtime configuration for the Growatt client.
type Config struct {
	TokenFile string
	Region    string
	PlantID   *int64
	BaseURL   string
}

func ConfigFromProto(cfg *growattv1.GrowattConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("growatt config is required")
	}
	if strings.TrimSpace(cfg.TokenFile) == "" {
		return Config{}, fmt.Errorf("growatt token_file is required")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = defaultRegion
	}

	baseURL, ok := regionEndpoints[region]
	if !ok {
		return Config{}, fmt.Errorf("unknown growatt region %q", region)
	}

	var plantID *int64
	if cfg.PlantId != nil {
		value := cfg.GetPlantId()
		if value <= 0 {
			return Config{}, fmt.Errorf("growatt plant_id must be positive")
		}
		plantID = &value
	}

	return Config{
		TokenFile: cfg.TokenFile,
		Region:    region,
		PlantID:   plantID,
		BaseURL:   baseURL,
	}, nil
}
