package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/joshp123/gohome/internal/config"
	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/plugins"
	"github.com/joshp123/gohome/internal/router"
	"github.com/joshp123/gohome/internal/server"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	buildVersion = "dev"
	buildCommit  = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "oauth" {
		oauthMain(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "roborock" {
		roborockMain(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "backfill" {
		backfillMain(os.Args[2:])
		return
	}

	flags := flag.NewFlagSet("gohome", flag.ExitOnError)
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	_ = flags.Parse(os.Args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	compiledPlugins := plugins.Compiled(cfg)

	if err := core.ValidatePlugins(compiledPlugins); err != nil {
		log.Fatalf("plugin validation: %v", err)
	}

	enabled := config.EnabledPlugins(cfg)
	if err := core.ValidateEnabledPlugins(compiledPlugins, enabled, false); err != nil {
		log.Fatalf("plugin enablement: %v", err)
	}

	activePlugins := core.FilterPlugins(compiledPlugins, enabled, false)

	if err := core.WriteDashboards(cfg.Core.DashboardDir, activePlugins); err != nil {
		log.Fatalf("write dashboards: %v", err)
	}

	grpcServer, err := server.NewGRPCServer(cfg.Core.GrpcAddr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}

	router.RegisterPlugins(grpcServer.Server, activePlugins)

	metricsRegistry := core.MetricsRegistry(activePlugins)
	metricsRegistry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gohome_build_info",
		Help: "Build information",
		ConstLabels: prometheus.Labels{
			"version": buildVersion,
			"commit":  buildCommit,
		},
	}, func() float64 { return 1 }))

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", server.HealthHandler)
	httpMux.Handle("/metrics", server.MetricsHandler(metricsRegistry))
	httpMux.Handle("/dashboards/", server.DashboardsHandler(core.DashboardsMap(activePlugins)))
	for _, plugin := range activePlugins {
		if registrant, ok := plugin.(core.HTTPRegistrant); ok {
			registrant.RegisterHTTP(httpMux)
		}
	}

	httpServer := server.NewHTTPServer(cfg.Core.HttpAddr, httpMux)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalf("http serve: %v", err)
		}
	}()

	if err := grpcServer.Serve(); err != nil {
		log.Fatalf("grpc serve: %v", err)
	}
}
