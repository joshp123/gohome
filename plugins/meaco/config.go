package meaco

import (
	"encoding/json"
	"fmt"
	meacov1 "github.com/joshp123/gohome/proto/gen/plugins/meaco/v1"
	"net"
	"os"
)

type Config struct {
	SchemaVersion   int    `json:"schema_version"`
	DeviceID        string `json:"device_id"`
	LocalKey        string `json:"local_key"`
	IP              string `json:"ip"`
	ProtocolVersion string `json:"protocol_version"`
	ProductID       string `json:"product_id"`
}

func ConfigFromProto(cfg *meacov1.MeacoConfig) (Config, error) {
	var bootstrap Config
	data, err := os.ReadFile(cfg.GetBootstrapFile())
	if err != nil {
		return bootstrap, fmt.Errorf("read meaco bootstrap: %w", err)
	}
	if json.Unmarshal(data, &bootstrap) != nil {
		return Config{}, fmt.Errorf("invalid meaco bootstrap JSON")
	}
	if cfg.DeviceIp != "" {
		bootstrap.IP = cfg.DeviceIp
	}
	if bootstrap.SchemaVersion != 1 || bootstrap.ProtocolVersion != "3.3" || bootstrap.ProductID != "wuagakhecbqdkczw" {
		return Config{}, fmt.Errorf("meaco requires schema 1, protocol 3.3 and Arete Two 25L product")
	}
	if bootstrap.DeviceID == "" || len(bootstrap.LocalKey) != 16 || net.ParseIP(bootstrap.IP) == nil {
		return Config{}, fmt.Errorf("meaco requires device ID, 16-byte local key and device IP")
	}
	return bootstrap, nil
}
