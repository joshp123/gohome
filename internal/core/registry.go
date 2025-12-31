package core

import (
	context "context"
	"sync"

	registryv1 "github.com/elliot-alderson/gohome/proto/gen/registry/v1"
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
		resp.Plugins = append(resp.Plugins, &registryv1.PluginSummary{
			PluginId:    manifest.PluginID,
			DisplayName: manifest.DisplayName,
			Version:     manifest.Version,
			Status:      string(p.Health()),
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
		if manifest.PluginID != req.PluginId {
			continue
		}

		descriptor := &registryv1.PluginDescriptor{
			PluginId:      manifest.PluginID,
			DisplayName:   manifest.DisplayName,
			Version:       manifest.Version,
			Services:      manifest.Services,
			AgentsMd:      p.AgentsMD(),
			Status:        string(p.Health()),
			HealthMessage: p.HealthMessage(),
		}

		for _, d := range p.Dashboards() {
			descriptor.Dashboards = append(descriptor.Dashboards, &registryv1.Dashboard{
				Name: d.Name,
				Path: "/dashboards/" + manifest.PluginID + "/" + d.Name + ".json",
			})
		}

		return &registryv1.DescribePluginResponse{Plugin: descriptor}, nil
	}

	return &registryv1.DescribePluginResponse{}, nil
}

func (r *RegistryService) WatchPlugins(_ *registryv1.WatchPluginsRequest, _ registryv1.Registry_WatchPluginsServer) error {
	// MVP: static plugin set. Streaming updates are deferred.
	return nil
}
