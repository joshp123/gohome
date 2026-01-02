package growatt

import "time"

type Plant struct {
	ID     int64
	Name   string
	Status int32
}

type PlantEnergy struct {
	PlantID          int64
	CurrentPowerW    float64
	TodayEnergyKWh   float64
	MonthlyEnergyKWh float64
	YearlyEnergyKWh  float64
	TotalEnergyKWh   float64
	LastUpdate       *time.Time
	Timezone         string
}
