package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/config"
	"github.com/joshp123/gohome/plugins/growatt"
	"github.com/joshp123/gohome/plugins/tado"
)

const defaultTadoBackfillStart = "2024-11-29"

func backfillMain(args []string) {
	if len(args) == 0 {
		backfillUsage()
		os.Exit(2)
	}

	switch args[0] {
	case "tado":
		tadoBackfillCmd(args[1:])
	case "growatt":
		growattBackfillCmd(args[1:])
	default:
		backfillUsage()
		os.Exit(2)
	}
}

func backfillUsage() {
	fmt.Println("gohome backfill <command> [args]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  tado --start YYYY-MM-DD --end YYYY-MM-DD [--zones name1,name2] [--config path]")
	fmt.Println("  growatt [--max-weeks N] [--stop-after-empty-weeks N] [--config path] [--import-url url]")
}

func tadoBackfillCmd(args []string) {
	flags := flag.NewFlagSet("tado", flag.ExitOnError)
	startStr := flags.String("start", defaultTadoBackfillStart, "Backfill start date (YYYY-MM-DD)")
	endStr := flags.String("end", time.Now().Format("2006-01-02"), "Backfill end date (YYYY-MM-DD)")
	zoneFilter := flags.String("zones", "", "Optional comma-separated zone names to include (default: all)")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	importURL := flags.String("import-url", tado.DefaultImportURL, "VictoriaMetrics import URL")
	batchSize := flags.Int("batch-size", 5000, "Samples per import batch")
	throttle := flags.Duration("throttle", 200*time.Millisecond, "Delay between API calls")
	_ = flags.Parse(args)

	start, err := time.Parse("2006-01-02", *startStr)
	if err != nil {
		fatal("backfill tado", fmt.Errorf("invalid start date: %w", err))
	}
	end, err := time.Parse("2006-01-02", *endStr)
	if err != nil {
		fatal("backfill tado", fmt.Errorf("invalid end date: %w", err))
	}
	if end.Before(start) {
		fatal("backfill tado", fmt.Errorf("end date must be >= start date"))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("backfill tado", err)
	}
	if cfg.Tado == nil {
		fatal("backfill tado", fmt.Errorf("tado config missing"))
	}

	runtimeCfg, err := tado.ConfigFromProto(cfg.Tado)
	if err != nil {
		fatal("backfill tado", err)
	}
	client, err := tado.NewClient(runtimeCfg, tado.Plugin{}.OAuthDeclaration(), cfg.Oauth)
	if err != nil {
		fatal("backfill tado", err)
	}

	var zones []string
	if strings.TrimSpace(*zoneFilter) != "" {
		for _, name := range strings.Split(*zoneFilter, ",") {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				zones = append(zones, trimmed)
			}
		}
	}

	opts := tado.BackfillOptions{
		StartDate: start,
		EndDate:   end,
		Zones:     zones,
		ImportURL: *importURL,
		BatchSize: *batchSize,
		Throttle:  *throttle,
	}

	if err := tado.Backfill(context.Background(), client, opts); err != nil {
		fatal("backfill tado", err)
	}
}

func growattBackfillCmd(args []string) {
	flags := flag.NewFlagSet("growatt", flag.ExitOnError)
	maxWeeks := flags.Int("max-weeks", 520, "Maximum weeks to request (backfill stops early after empty weeks)")
	stopAfterEmptyWeeks := flags.Int("stop-after-empty-weeks", 6, "Stop after N consecutive empty weeks (0 disables)")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	importURL := flags.String("import-url", growatt.DefaultImportURL, "VictoriaMetrics import URL")
	_ = flags.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("backfill growatt", err)
	}
	if cfg.Growatt == nil {
		fatal("backfill growatt", fmt.Errorf("growatt config missing"))
	}

	runtimeCfg, err := growatt.ConfigFromProto(cfg.Growatt)
	if err != nil {
		fatal("backfill growatt", err)
	}
	client, err := growatt.NewClient(runtimeCfg)
	if err != nil {
		fatal("backfill growatt", err)
	}

	ctx := context.Background()
	plant, err := client.ResolvePlant(ctx, 0)
	if err != nil {
		fatal("backfill growatt", err)
	}

	opts := growatt.HistoryOptions{
		MaxWeeks:            *maxWeeks,
		StopAfterEmptyWeeks: *stopAfterEmptyWeeks,
		ImportURL:           *importURL,
	}
	if err := client.ImportEnergyHistoryWithOptions(ctx, plant, opts); err != nil {
		fatal("backfill growatt", err)
	}
}
