package core

import (
	context "context"
	"sync"
	"time"

	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RegistryService provides plugin discovery to clients.
type RegistryService struct {
	registryv1.UnimplementedRegistryServer

	plugins []Plugin
	mu      sync.RWMutex
}

func NewRegistryService(plugins []Plugin) *RegistryService {
	return &RegistryService{plugins: plugins}
}

func (r *RegistryService) ListPlugins(ctx context.Context, _ *registryv1.ListPluginsRequest) (*registryv1.ListPluginsResponse, error) {
	_ = ctx

	r.mu.RLock()
	defer r.mu.RUnlock()

	resp := &registryv1.ListPluginsResponse{}
	for _, p := range r.plugins {
		manifest := p.Manifest()
		status, message, lastGood := pluginHealth(p)
		resp.Plugins = append(resp.Plugins, &registryv1.PluginSummary{
			PluginId:      manifest.PluginID,
			DisplayName:   manifest.DisplayName,
			Version:       manifest.Version,
			Status:        string(status),
			HealthMessage: message,
			LastSuccess:   lastGood,
		})
	}

	return resp, nil
}

func (r *RegistryService) DescribePlugin(ctx context.Context, req *registryv1.DescribePluginRequest) (*registryv1.DescribePluginResponse, error) {
	_ = ctx

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.plugins {
		manifest := p.Manifest()
		status, message, lastGood := pluginHealth(p)
		if manifest.PluginID != req.PluginId {
			continue
		}

		descriptor := &registryv1.PluginDescriptor{
			PluginId:      manifest.PluginID,
			DisplayName:   manifest.DisplayName,
			Version:       manifest.Version,
			Services:      manifest.Services,
			AgentsMd:      p.AgentsMD(),
			Status:        string(status),
			HealthMessage: message,
			LastSuccess:   lastGood,
		}

		return &registryv1.DescribePluginResponse{Plugin: descriptor}, nil
	}

	return &registryv1.DescribePluginResponse{}, nil
}

func (r *RegistryService) WatchPlugins(_ *registryv1.WatchPluginsRequest, _ registryv1.Registry_WatchPluginsServer) error {
	// MVP: static plugin set. Streaming updates are deferred.
	return nil
}

func pluginHealth(p Plugin) (HealthStatus, string, *timestamppb.Timestamp) {
	if source, ok := p.(interface {
		SourceHealth() (HealthStatus, string, time.Time)
	}); ok {
		status, message, last := source.SourceHealth()
		if last.IsZero() {
			return status, message, nil
		}
		return status, message, timestamppb.New(last)
	}
	return p.Health(), p.HealthMessage(), nil
}
