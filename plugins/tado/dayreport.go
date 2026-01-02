package tado

import (
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
	Value     struct {
		Celsius float64 `json:"celsius"`
	} `json:"value"`
}

type dataPointHumidity struct {
	Timestamp string `json:"timestamp"`
	Value     struct {
		Percentage float64 `json:"percentage"`
	} `json:"value"`
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

func parseDayReportTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, nil
	}
	return time.Parse(time.RFC3339, value)
}
