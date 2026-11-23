package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/joshp123/gohome/internal/render"
	brotherv1 "github.com/joshp123/gohome/proto/gen/plugins/brother/v1"
	casambiv1 "github.com/joshp123/gohome/proto/gen/plugins/casambi/v1"
	daikinv1 "github.com/joshp123/gohome/proto/gen/plugins/daikin/v1"
	growattv1 "github.com/joshp123/gohome/proto/gen/plugins/growatt/v1"
	p1v1 "github.com/joshp123/gohome/proto/gen/plugins/p1_homewizard/v1"
	shellyv1 "github.com/joshp123/gohome/proto/gen/plugins/shelly/v1"
	sonosv1 "github.com/joshp123/gohome/proto/gen/plugins/sonos/v1"
	unifiv1 "github.com/joshp123/gohome/proto/gen/plugins/unifi/v1"
	weheatv1 "github.com/joshp123/gohome/proto/gen/plugins/weheat/v1"
	"google.golang.org/grpc"
)

func p1Cmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	client := p1v1.NewP1HomewizardServiceClient(conn)
	switch sub {
	case "status", "reading":
		resp, err := client.GetReading(ctx, &p1v1.GetReadingRequest{})
		if err != nil {
			out.WriteError("p1 status", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		r := resp.GetReading()
		pairs := []render.KeyValue{}
		if r.MeterModel != nil {
			pairs = append(pairs, render.KeyValue{Key: "meter_model", Value: r.GetMeterModel()})
		}
		if r.ActivePowerWatts != nil {
			pairs = append(pairs, render.KeyValue{Key: "active_power_w", Value: fmt.Sprintf("%.0f W", r.GetActivePowerWatts())})
		}
		if r.ActiveCurrentAmperes != nil {
			pairs = append(pairs, render.KeyValue{Key: "active_current_a", Value: fmt.Sprintf("%.1f A", r.GetActiveCurrentAmperes())})
		}
		if r.ActiveTariff != nil {
			pairs = append(pairs, render.KeyValue{Key: "active_tariff", Value: fmt.Sprintf("T%d", r.GetActiveTariff())})
		}
		if r.TotalImportT1Kwh != nil {
			pairs = append(pairs, render.KeyValue{Key: "import_t1_kwh", Value: fmt.Sprintf("%.1f kWh", r.GetTotalImportT1Kwh())})
		}
		if r.TotalImportT2Kwh != nil {
			pairs = append(pairs, render.KeyValue{Key: "import_t2_kwh", Value: fmt.Sprintf("%.1f kWh", r.GetTotalImportT2Kwh())})
		}
		if r.TotalExportT1Kwh != nil {
			pairs = append(pairs, render.KeyValue{Key: "export_t1_kwh", Value: fmt.Sprintf("%.1f kWh", r.GetTotalExportT1Kwh())})
		}
		if r.TotalExportT2Kwh != nil {
			pairs = append(pairs, render.KeyValue{Key: "export_t2_kwh", Value: fmt.Sprintf("%.1f kWh", r.GetTotalExportT2Kwh())})
		}
		if r.LongPowerFailCount != nil {
			pairs = append(pairs, render.KeyValue{Key: "long_power_failures", Value: fmt.Sprintf("%.0f", r.GetLongPowerFailCount())})
		}
		if len(r.PowerFailureEvents) > 0 {
			last := r.PowerFailureEvents[len(r.PowerFailureEvents)-1]
			pairs = append(pairs, render.KeyValue{
				Key:   "last_power_failure",
				Value: fmt.Sprintf("%s (%.0fs)", last.GetRestoredAt().AsTime().Local().Format(time.RFC3339), last.GetDurationSeconds()),
			})
		}
		_ = out.WriteKeyValues(pairs)
		return render.ExitOK
	case "info":
		resp, err := client.GetInfo(ctx, &p1v1.GetInfoRequest{})
		if err != nil {
			out.WriteError("p1 info", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		info := resp.GetInfo()
		_ = out.WriteKeyValues([]render.KeyValue{
			{Key: "product", Value: info.GetProductName()},
			{Key: "type", Value: info.GetProductType()},
			{Key: "firmware", Value: info.GetFirmwareVersion()},
			{Key: "api_version", Value: info.GetApiVersion()},
		})
		return render.ExitOK
	default:
		out.WriteHelp("gohome-cli p1 <status|info>", nil)
		return render.ExitUsageError
	}
}

func weheatCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	client := weheatv1.NewWeheatServiceClient(conn)
	switch sub {
	case "status", "telemetry":
		resp, err := client.GetLatestTelemetry(ctx, &weheatv1.GetLatestTelemetryRequest{})
		if err != nil {
			out.WriteError("weheat status", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		t := resp.GetTelemetry()
		pairs := []render.KeyValue{
			{Key: "heat_pump_id", Value: t.GetHeatPumpId()},
		}
		if t.ObservedAt != nil {
			pairs = append(pairs, render.KeyValue{Key: "observed_at", Value: t.ObservedAt.AsTime().Local().Format(time.RFC3339)})
		}
		if t.OperatingStateCode != nil {
			pairs = append(pairs, render.KeyValue{Key: "state_code", Value: strconv.Itoa(int(t.GetOperatingStateCode()))})
		}
		if t.WaterOutCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "water_out_c", Value: fmt.Sprintf("%.1f C", t.GetWaterOutCelsius())})
		}
		if t.WaterInCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "water_in_c", Value: fmt.Sprintf("%.1f C", t.GetWaterInCelsius())})
		}
		if t.WaterOutCelsius != nil && t.WaterInCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "delta_t_c", Value: fmt.Sprintf("%.1f C", t.GetWaterOutCelsius()-t.GetWaterInCelsius())})
		}
		if t.TankTopCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "tank_top_c", Value: fmt.Sprintf("%.1f C", t.GetTankTopCelsius())})
		}
		if t.TankBottomCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "tank_bottom_c", Value: fmt.Sprintf("%.1f C", t.GetTankBottomCelsius())})
		}
		if t.CompressorRpm != nil {
			pairs = append(pairs, render.KeyValue{Key: "compressor_rpm", Value: fmt.Sprintf("%.0f rpm", t.GetCompressorRpm())})
		}
		if t.ElectricityInWatts != nil {
			pairs = append(pairs, render.KeyValue{Key: "electricity_in_w", Value: fmt.Sprintf("%d W", t.GetElectricityInWatts())})
		}
		if t.HeatOutWatts != nil {
			pairs = append(pairs, render.KeyValue{Key: "heat_out_w", Value: fmt.Sprintf("%d W", t.GetHeatOutWatts())})
		}
		if t.ElectricityInWatts != nil && t.HeatOutWatts != nil && t.GetElectricityInWatts() > 100 && t.GetHeatOutWatts() > 0 {
			cop := float64(t.GetHeatOutWatts()) / float64(t.GetElectricityInWatts())
			pairs = append(pairs, render.KeyValue{Key: "live_cop", Value: fmt.Sprintf("%.2f", cop)})
		}
		_ = out.WriteKeyValues(pairs)
		return render.ExitOK
	case "energy":
		resp, err := client.GetEnergySummary(ctx, &weheatv1.GetEnergySummaryRequest{})
		if err != nil {
			out.WriteError("weheat energy", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		s := resp.GetSummary()
		pairs := []render.KeyValue{
			{Key: "electricity_in_heating_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetElectricityInHeatingKwh())},
			{Key: "heat_out_heating_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetHeatOutHeatingKwh())},
			{Key: "electricity_in_dhw_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetElectricityInDhwKwh())},
			{Key: "heat_out_dhw_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetHeatOutDhwKwh())},
		}
		_ = out.WriteKeyValues(pairs)
		return render.ExitOK
	default:
		out.WriteHelp("gohome-cli weheat <status|energy>", nil)
		return render.ExitUsageError
	}
}

func daikinCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	client := daikinv1.NewDaikinServiceClient(conn)
	switch sub {
	case "list", "units":
		resp, err := client.ListUnits(ctx, &daikinv1.ListUnitsRequest{})
		if err != nil {
			out.WriteError("daikin list", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Units))
		for _, u := range resp.Units {
			rows = append(rows, []string{u.GetName(), u.GetModel(), u.GetId()})
		}
		_ = out.WriteTable([]string{"UNIT", "MODEL", "ID"}, rows)
		return render.ExitOK
	case "status":
		unitID := ""
		if len(args) > 1 {
			unitID = args[1]
		}
		resp, err := client.GetUnitStatus(ctx, &daikinv1.GetUnitStatusRequest{UnitId: unitID})
		if err != nil {
			out.WriteError("daikin status", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		s := resp.GetStatus()
		pairs := []render.KeyValue{
			{Key: "name", Value: s.GetName()},
			{Key: "model", Value: s.GetModel()},
			{Key: "cloud_connected", Value: strconv.FormatBool(s.GetCloudConnected())},
		}
		if s.OnOffMode != nil {
			pairs = append(pairs, render.KeyValue{Key: "power", Value: s.GetOnOffMode()})
		}
		if s.OperationMode != nil {
			pairs = append(pairs, render.KeyValue{Key: "mode", Value: s.GetOperationMode()})
		}
		if s.RoomTemperatureCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "room_temp_c", Value: fmt.Sprintf("%.1f C", s.GetRoomTemperatureCelsius())})
		}
		if s.OutdoorTemperatureCelsius != nil {
			pairs = append(pairs, render.KeyValue{Key: "outdoor_temp_c", Value: fmt.Sprintf("%.1f C", s.GetOutdoorTemperatureCelsius())})
		}
		_ = out.WriteKeyValues(pairs)
		return render.ExitOK
	default:
		out.WriteHelp("gohome-cli daikin <list|status [unit_id]>", nil)
		return render.ExitUsageError
	}
}

func growattCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	client := growattv1.NewGrowattServiceClient(conn)
	resp, err := client.GetPlantStatus(ctx, &growattv1.GetPlantStatusRequest{})
	if err != nil {
		out.WriteError("growatt status", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	if out.Format == render.FormatJSON {
		_ = out.WriteProto(resp)
		return render.ExitOK
	}
	s := resp.GetStatus()
	pairs := []render.KeyValue{
		{Key: "plant", Value: s.GetPlant().GetName()},
		{Key: "today_kwh", Value: fmt.Sprintf("%.2f kWh", s.GetTodayEnergyKwh())},
		{Key: "month_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetMonthlyEnergyKwh())},
		{Key: "year_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetYearlyEnergyKwh())},
		{Key: "total_kwh", Value: fmt.Sprintf("%.1f kWh", s.GetTotalEnergyKwh())},
		{Key: "last_update", Value: s.GetLastUpdateTime()},
	}
	if s.CurrentPowerWatts != nil {
		pairs = append([]render.KeyValue{{Key: "current_power_w", Value: fmt.Sprintf("%.0f W", s.GetCurrentPowerWatts())}}, pairs...)
	}
	_ = out.WriteKeyValues(pairs)
	return render.ExitOK
}

func shellyCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	client := shellyv1.NewShellyServiceClient(conn)
	resp, err := client.ListCovers(ctx, &shellyv1.ListCoversRequest{})
	if err != nil {
		out.WriteError("shelly list", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	if out.Format == render.FormatJSON {
		_ = out.WriteProto(resp)
		return render.ExitOK
	}
	rows := make([][]string, 0, len(resp.Covers))
	for _, c := range resp.Covers {
		pos := "-"
		if c.PositionPercent != nil {
			pos = fmt.Sprintf("%d%%", c.GetPositionPercent())
		}
		state := c.GetState()
		if c.GetReadError() != "" {
			state = "error: " + c.GetReadError()
		}
		rows = append(rows, []string{c.GetName(), pos, state, c.GetAddress()})
	}
	_ = out.WriteTable([]string{"COVER", "POSITION", "STATE", "ADDRESS"}, rows)
	return render.ExitOK
}

func sonosCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	client := sonosv1.NewSonosServiceClient(conn)
	switch sub {
	case "status", "playback":
		resp, err := client.GetPlayback(ctx, &sonosv1.GetPlaybackRequest{})
		if err != nil {
			out.WriteError("sonos status", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Zones))
		for _, z := range resp.Zones {
			track := "-"
			if z.Track != nil && z.Track.Title != nil {
				track = z.Track.GetTitle()
			}
			rows = append(rows, []string{z.GetZoneName(), z.GetState().String(), fmt.Sprintf("%d%%", z.GetVolume()), strconv.FormatBool(z.GetMuted()), track})
		}
		_ = out.WriteTable([]string{"ZONE", "STATE", "VOLUME", "MUTED", "TRACK"}, rows)
		return render.ExitOK
	case "zones":
		resp, err := client.GetZones(ctx, &sonosv1.GetZonesRequest{})
		if err != nil {
			out.WriteError("sonos zones", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Zones))
		for _, z := range resp.Zones {
			rows = append(rows, []string{z.GetName(), strconv.Itoa(len(z.Players)), z.GetCoordinatorUuid()})
		}
		_ = out.WriteTable([]string{"ZONE", "PLAYERS", "COORDINATOR"}, rows)
		return render.ExitOK
	default:
		out.WriteHelp("gohome-cli sonos <status|zones>", nil)
		return render.ExitUsageError
	}
}

func casambiCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "lights"
	if len(args) > 0 {
		sub = args[0]
	}
	client := casambiv1.NewCasambiServiceClient(conn)
	switch sub {
	case "lights":
		resp, err := client.ListLights(ctx, &casambiv1.ListLightsRequest{})
		if err != nil {
			out.WriteError("casambi lights", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Lights))
		for _, l := range resp.Lights {
			power := "unknown"
			if l.On != nil {
				if l.GetOn() {
					power = "on"
				} else {
					power = "off"
				}
			}
			bright := "-"
			if l.BrightnessPercent != nil {
				bright = fmt.Sprintf("%d%%", l.GetBrightnessPercent())
			}
			rows = append(rows, []string{l.GetName(), strconv.FormatBool(l.GetOnline()), power, bright})
		}
		_ = out.WriteTable([]string{"LIGHT", "ONLINE", "POWER", "BRIGHTNESS"}, rows)
		return render.ExitOK
	case "scenes":
		resp, err := client.ListScenes(ctx, &casambiv1.ListScenesRequest{})
		if err != nil {
			out.WriteError("casambi scenes", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Scenes))
		for _, s := range resp.Scenes {
			rows = append(rows, []string{s.GetName(), strconv.FormatUint(uint64(s.GetId()), 10), strconv.FormatBool(s.GetAvailable())})
		}
		_ = out.WriteTable([]string{"SCENE", "ID", "AVAILABLE"}, rows)
		return render.ExitOK
	case "switches":
		resp, err := client.ListSwitches(ctx, &casambiv1.ListSwitchesRequest{})
		if err != nil {
			out.WriteError("casambi switches", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Switches))
		for _, sw := range resp.Switches {
			bat := "-"
			if sw.BatteryLevel != nil {
				bat = strconv.FormatUint(uint64(sw.GetBatteryLevel()), 10)
			}
			rows = append(rows, []string{sw.GetName(), strconv.FormatBool(sw.GetOnline()), bat, strconv.Itoa(len(sw.Buttons))})
		}
		_ = out.WriteTable([]string{"SWITCH", "ONLINE", "BATTERY_RAW", "BUTTONS"}, rows)
		return render.ExitOK
	default:
		out.WriteHelp("gohome-cli casambi <lights|scenes|switches>", nil)
		return render.ExitUsageError
	}
}

func brotherCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	client := brotherv1.NewBrotherServiceClient(conn)
	resp, err := client.GetPrinterStatus(ctx, &brotherv1.GetPrinterStatusRequest{})
	if err != nil {
		out.WriteError("brother status", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	if out.Format == render.FormatJSON {
		_ = out.WriteProto(resp)
		return render.ExitOK
	}
	s := resp.GetStatus()
	pairs := []render.KeyValue{
		{Key: "model", Value: s.GetModelName()},
		{Key: "firmware", Value: s.GetFirmwareVersion()},
	}
	if s.TonerRemainingPercent != nil {
		pairs = append(pairs, render.KeyValue{Key: "toner_remaining", Value: fmt.Sprintf("%.0f%%", s.GetTonerRemainingPercent())})
	}
	if s.DrumRemainingPercent != nil {
		pairs = append(pairs, render.KeyValue{Key: "drum_remaining", Value: fmt.Sprintf("%.0f%%", s.GetDrumRemainingPercent())})
	}
	if s.PageCounter != nil {
		pairs = append(pairs, render.KeyValue{Key: "pages_total", Value: strconv.FormatUint(s.GetPageCounter(), 10)})
	}
	if s.DuplexPageCounter != nil {
		pairs = append(pairs, render.KeyValue{Key: "pages_duplex", Value: strconv.FormatUint(s.GetDuplexPageCounter(), 10)})
	}
	if s.TotalPaperJams != nil {
		pairs = append(pairs, render.KeyValue{Key: "paper_jams", Value: strconv.FormatUint(s.GetTotalPaperJams(), 10)})
	}
	_ = out.WriteKeyValues(pairs)
	return render.ExitOK
}

func unifiCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	client := unifiv1.NewUniFiServiceClient(conn)
	resp, err := client.GetSiteSummary(ctx, &unifiv1.GetSiteSummaryRequest{})
	if err != nil {
		out.WriteError("unifi status", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	if out.Format == render.FormatJSON {
		_ = out.WriteProto(resp)
		return render.ExitOK
	}
	s := resp.GetSummary()
	pairs := []render.KeyValue{
		{Key: "controller", Value: s.GetControllerName()},
		{Key: "hardware", Value: s.GetHardwareShortname()},
		{Key: "site", Value: s.GetSite()},
		{Key: "wired_clients", Value: strconv.FormatUint(s.GetWiredClients(), 10)},
		{Key: "wireless_clients", Value: strconv.FormatUint(s.GetWirelessClients(), 10)},
		{Key: "devices", Value: strconv.Itoa(len(s.Devices))},
	}
	_ = out.WriteKeyValues(pairs)
	return render.ExitOK
}
