package growatt

import (
	context "context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
)

type service struct {
	v1.UnimplementedGrowattServiceServer
	client *Client
}

func RegisterGrowattService(server *grpc.Server, client *Client) {
	v1.RegisterGrowattServiceServer(server, &service{client: client})
}

func (s *service) ListPlants(ctx context.Context, _ *v1.ListPlantsRequest) (*v1.ListPlantsResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "growatt client not configured")
	}

	plants, err := s.client.ListPlants(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list plants: %v", err)
	}

	resp := &v1.ListPlantsResponse{}
	for _, plant := range plants {
		resp.Plants = append(resp.Plants, &v1.Plant{
			PlantId: plant.ID,
			Name:    plant.Name,
			Status:  plant.Status,
		})
	}
	return resp, nil
}

func (s *service) GetPlantStatus(ctx context.Context, req *v1.GetPlantStatusRequest) (*v1.GetPlantStatusResponse, error) {
	if s.client == nil {
		return nil, status.Error(codes.FailedPrecondition, "growatt client not configured")
	}

	plant, err := s.client.ResolvePlant(ctx, req.GetPlantId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "resolve plant: %v", err)
	}

	energy, err := s.client.EnergyOverview(ctx, plant.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "energy overview: %v", err)
	}

	statusResp := &v1.PlantStatus{
		Plant: &v1.Plant{
			PlantId: plant.ID,
			Name:    plant.Name,
			Status:  plant.Status,
		},
		CurrentPowerWatts: energy.CurrentPowerW,
		TodayEnergyKwh:    energy.TodayEnergyKWh,
		MonthlyEnergyKwh:  energy.MonthlyEnergyKWh,
		YearlyEnergyKwh:   energy.YearlyEnergyKWh,
		TotalEnergyKwh:    energy.TotalEnergyKWh,
		Timezone:          energy.Timezone,
	}
	if energy.LastUpdate != nil {
		statusResp.LastUpdateTime = energy.LastUpdate.Format(timeLayout)
	}

	return &v1.GetPlantStatusResponse{Status: statusResp}, nil
}
