package tado

import (
	"encoding/json"
	"time"
)

type dayReport struct {
	Interval struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"interval"`
	MeasuredData struct {
		InsideTemperature struct {
			DataPoints []dataPointTemp `json:"dataPoints"`
		} `json:"insideTemperature"`
		Humidity struct {
			DataPoints []dataPointHumidity `json:"dataPoints"`
		} `json:"humidity"`
	} `json:"measuredData"`
	CallForHeat struct {
		DataIntervals []intervalString `json:"dataIntervals"`
	} `json:"callForHeat"`
	Settings struct {
		DataIntervals []settingInterval `json:"dataIntervals"`
	} `json:"settings"`
	Weather struct {
		Condition struct {
			DataIntervals []weatherInterval `json:"dataIntervals"`
		} `json:"condition"`
		Sunny struct {
			DataIntervals []sunnyInterval `json:"dataIntervals"`
		} `json:"sunny"`
	} `json:"weather"`
}

type dataPointTemp struct {
	Timestamp string `json:"timestamp"`
	Value     tempValue `json:"value"`
}

type dataPointHumidity struct {
	Timestamp string `json:"timestamp"`
	Value     humidityValue `json:"value"`
}

type intervalString struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"`
}

type settingInterval struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value struct {
		Power       string `json:"power"`
		Temperature struct {
			Celsius *float64 `json:"celsius"`
		} `json:"temperature"`
	} `json:"value"`
}

type weatherInterval struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value struct {
		Temperature struct {
			Celsius *float64 `json:"celsius"`
		} `json:"temperature"`
	} `json:"value"`
}

type sunnyInterval struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value bool `json:"value"`
}

type tempValue struct {
	Celsius float64
}

func (v *tempValue) UnmarshalJSON(data []byte) error {
	var asNumber float64
	if err := json.Unmarshal(data, &asNumber); err == nil {
		v.Celsius = asNumber
		return nil
	}
	var asObj struct {
		Celsius *float64 `json:"celsius"`
	}
	if err := json.Unmarshal(data, &asObj); err != nil {
		return err
	}
	if asObj.Celsius != nil {
		v.Celsius = *asObj.Celsius
	}
	return nil
}

type humidityValue struct {
	Percentage float64
}

func (v *humidityValue) UnmarshalJSON(data []byte) error {
	var asNumber float64
	if err := json.Unmarshal(data, &asNumber); err == nil {
		v.Percentage = asNumber
		return nil
	}
	var asObj struct {
		Percentage *float64 `json:"percentage"`
	}
	if err := json.Unmarshal(data, &asObj); err != nil {
		return err
	}
	if asObj.Percentage != nil {
		v.Percentage = *asObj.Percentage
	}
	return nil
}

func parseDayReportTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, nil
	}
	return time.Parse(time.RFC3339, value)
}
