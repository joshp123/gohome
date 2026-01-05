package roborock

import (
	"fmt"

	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
)

// Config defines runtime configuration for the Roborock client.
type Config struct {
	BootstrapFile  string
	CloudFallback  bool
	IPOverrides    map[string]string
	SegmentNames   map[uint32]string
	DefaultProfile CleanProfile
}

func ConfigFromProto(cfg *roborockv1.RoborockConfig) (Config, error) {
	if cfg == nil {
		return Config{}, fmt.Errorf("roborock config is required")
	}
	if cfg.BootstrapFile == "" {
		return Config{}, fmt.Errorf("roborock bootstrap_file is required")
	}

	return Config{
		BootstrapFile:  cfg.BootstrapFile,
		CloudFallback:  cfg.CloudFallback,
		IPOverrides:    cfg.DeviceIpOverrides,
		SegmentNames:   cfg.SegmentNames,
		DefaultProfile: cleanProfileFromProto(cfg.DefaultProfile),
	}, nil
}

type CleanProfile struct {
	FanPower       int
	MopMode        int
	MopIntensity   int
	Repeat         int
	CleanOrderMode int
}

func (p CleanProfile) HasAny() bool {
	return p.FanPower != 0 || p.MopMode != 0 || p.MopIntensity != 0 || p.Repeat != 0 || p.CleanOrderMode != 0
}

func cleanProfileFromProto(profile *roborockv1.CleanProfile) CleanProfile {
	if profile == nil {
		return CleanProfile{}
	}
	return CleanProfile{
		FanPower:       int(profile.GetFanPower()),
		MopMode:        int(profile.GetMopMode()),
		MopIntensity:   int(profile.GetMopIntensity()),
		Repeat:         int(profile.GetRepeat()),
		CleanOrderMode: int(profile.GetCleanOrderMode()),
	}
}
