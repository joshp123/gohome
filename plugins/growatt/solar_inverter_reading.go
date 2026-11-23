package growatt

import (
	"time"

	v1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
)

type Plant struct {
	ID     int64
	Name   string
	Status int32
}

type PlantEnergy struct {
	Inverter         *v1.InverterTelemetry
	PlantID          int64
	CurrentPowerW    float64
	TodayEnergyKWh   float64
	MonthlyEnergyKWh float64
	YearlyEnergyKWh  float64
	TotalEnergyKWh   float64
	LastUpdate       *time.Time
	Timezone         string
}
