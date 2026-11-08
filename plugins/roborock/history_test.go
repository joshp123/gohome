package roborock

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRunMetricsUseDeviceTimeAndAreaUnits(t *testing.T) {
	var record CleaningRecord
	if err := decodeDeviceObject(map[string]any{"begin": 100, "end": 200, "area": 1500000, "error": 0}, &record); err != nil {
		t.Fatal(err)
	}
	ch := make(chan prometheus.Metric, 20)
	emitReadings(ch, Device{ID: "vacuum"}, cleaningRecordMetrics(&record), time.Unix(*record.End, 0))
	close(ch)
	count := 0
	for metric := range ch {
		var sample dto.Metric
		if err := metric.Write(&sample); err != nil {
			t.Fatal(err)
		}
		if sample.GetTimestampMs() != 200000 {
			t.Fatal("run uses scrape timestamp")
		}
		if sample.GetGauge().GetValue() == 1.5 {
			count++
		}
	}
	if count != 1 {
		t.Fatal("area must convert square millimetres to square metres")
	}
	if mapCleaningRecord(&record).FinishReasonCode != nil {
		t.Fatal("missing reason became zero")
	}
}

func TestMapObstacleWithoutPhoto(t *testing.T) {
	// A 28-byte obstacle record can have no photo, even in the photo-capable block.
	raw := make([]byte, 4+12+28)
	binary.LittleEndian.PutUint16(raw[2:], 4)
	binary.LittleEndian.PutUint16(raw[4:], 15)
	binary.LittleEndian.PutUint16(raw[6:], 12)
	binary.LittleEndian.PutUint32(raw[8:], 28)
	binary.LittleEndian.PutUint16(raw[12:], 1)
	binary.LittleEndian.PutUint16(raw[16:], 1234)
	binary.LittleEndian.PutUint16(raw[18:], 5678)
	binary.LittleEndian.PutUint16(raw[20:], 18)
	obstacles, err := mapObstacles(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(obstacles) != 1 || obstacles[0].PhotoId != "" || obstacles[0].XMillimetres != 1234 || obstacles[0].YMillimetres != 5678 {
		t.Fatalf("incorrect obstacle: %v", obstacles)
	}
}
