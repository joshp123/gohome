package airgradient

type CurrentMeasures struct {
	Wifi                 *float64                     `json:"wifi"`
	SerialNo             string                       `json:"serialno"`
	RCO2                 *float64                     `json:"rco2"`
	PM01                 *float64                     `json:"pm01"`
	PM02                 *float64                     `json:"pm02"`
	PM10                 *float64                     `json:"pm10"`
	PM01Standard         *float64                     `json:"pm01Standard"`
	PM02Standard         *float64                     `json:"pm02Standard"`
	PM10Standard         *float64                     `json:"pm10Standard"`
	PM003Count           *float64                     `json:"pm003Count"`
	PM005Count           *float64                     `json:"pm005Count"`
	PM01Count            *float64                     `json:"pm01Count"`
	PM02Count            *float64                     `json:"pm02Count"`
	PM50Count            *float64                     `json:"pm50Count"`
	PM10Count            *float64                     `json:"pm10Count"`
	PM02Compensated      *float64                     `json:"pm02Compensated"`
	Temperature          *float64                     `json:"atmp"`
	TemperatureCorrected *float64                     `json:"atmpCompensated"`
	Humidity             *float64                     `json:"rhum"`
	HumidityCorrected    *float64                     `json:"rhumCompensated"`
	TVOCIndex            *float64                     `json:"tvocIndex"`
	TVOCRaw              *float64                     `json:"tvocRaw"`
	NOxIndex             *float64                     `json:"noxIndex"`
	NOxRaw               *float64                     `json:"noxRaw"`
	Boot                 *float64                     `json:"boot"`
	BootCount            *float64                     `json:"bootCount"`
	LedMode              string                       `json:"ledMode"`
	Firmware             string                       `json:"firmware"`
	Model                string                       `json:"model"`
	Satellites           map[string]SatelliteMeasures `json:"satellites"`
}

type SatelliteMeasures struct {
	Temperature *float64 `json:"atmp"`
	Humidity    *float64 `json:"rhum"`
	Wifi        *float64 `json:"wifi"`
}
