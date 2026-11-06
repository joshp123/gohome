package growatt

import (
	"context"
	"math"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestLivePlantDevicePower(t *testing.T) {
	tokenFile, plant := os.Getenv("GROWATT_LIVE_TOKEN_FILE"), os.Getenv("GROWATT_LIVE_PLANT_ID")
	if tokenFile == "" || plant == "" {
		t.Skip("requires Growatt token file and plant ID")
	}
	plantID, err := strconv.ParseInt(plant, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Config{TokenFile: tokenFile, BaseURL: regionEndpoints[defaultRegion], PlantID: &plantID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	watts, timestamp, err := client.plantLivePower(ctx, plantID, "GMT+1")
	if err != nil {
		t.Fatal(err)
	}
	if timestamp == nil || math.IsNaN(watts) || math.IsInf(watts, 0) {
		t.Fatal("missing device reading")
	}
	t.Logf("plant's inverter returned power; source reading age %s", time.Since(*timestamp).Round(time.Minute))
}
