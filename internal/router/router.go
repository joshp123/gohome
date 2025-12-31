package router

import (
	"google.golang.org/grpc"

	"github.com/elliot-alderson/gohome/internal/core"
	registryv1 "github.com/elliot-alderson/gohome/proto/gen/registry/v1"
)

// RegisterPlugins registers plugin services and core services on the gRPC server.
func RegisterPlugins(server *grpc.Server, plugins []core.Plugin) {
	registryv1.RegisterRegistryServer(server, core.NewRegistryService(plugins))

	for _, p := range plugins {
		p.RegisterGRPC(server)
	}
}
