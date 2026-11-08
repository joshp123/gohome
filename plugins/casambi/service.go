package casambi

import (
	"context"

	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
)

// forwardingService serves the bridge's own API unchanged, including its errors.
type forwardingService struct {
	casambiv1.UnimplementedCasambiServiceServer
	bridge casambiv1.CasambiServiceClient
}

func (s forwardingService) ListLights(ctx context.Context, request *casambiv1.ListLightsRequest) (*casambiv1.ListLightsResponse, error) {
	return s.bridge.ListLights(ctx, request)
}

func (s forwardingService) ListScenes(ctx context.Context, request *casambiv1.ListScenesRequest) (*casambiv1.ListScenesResponse, error) {
	return s.bridge.ListScenes(ctx, request)
}

func (s forwardingService) ListSwitches(ctx context.Context, request *casambiv1.ListSwitchesRequest) (*casambiv1.ListSwitchesResponse, error) {
	return s.bridge.ListSwitches(ctx, request)
}

func (s forwardingService) ListSwitchPresses(ctx context.Context, request *casambiv1.ListSwitchPressesRequest) (*casambiv1.ListSwitchPressesResponse, error) {
	return s.bridge.ListSwitchPresses(ctx, request)
}
