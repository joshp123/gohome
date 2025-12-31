package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joshp123/gohome/internal/core"
	"github.com/joshp123/gohome/internal/router"
	"github.com/joshp123/gohome/internal/server"
	"github.com/joshp123/gohome/plugins/tado"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	grpcAddr := envOrDefault("GOHOME_GRPC_ADDR", ":9000")
	httpAddr := envOrDefault("GOHOME_HTTP_ADDR", ":8080")
	enabledPluginsFile := envOrDefault("GOHOME_ENABLED_PLUGINS_FILE", "/etc/gohome/enabled-plugins")
	dashboardDir := os.Getenv("GOHOME_DASHBOARD_DIR")

	enabled, allowAll := readEnabledPlugins(enabledPluginsFile)
	plugins := buildPlugins(enabled, allowAll)

	if err := core.ValidatePlugins(plugins); err != nil {
		log.Fatalf("plugin validation: %v", err)
	}

	if err := core.WriteDashboards(dashboardDir, plugins); err != nil {
		log.Fatalf("write dashboards: %v", err)
	}

	grpcServer, err := server.NewGRPCServer(grpcAddr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}

	router.RegisterPlugins(grpcServer.Server, plugins)

	metricsRegistry := core.MetricsRegistry(plugins)
	metricsRegistry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gohome_build_info",
		Help: "Build information",
	}, func() float64 { return 1 }))

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", server.HealthHandler)
	httpMux.Handle("/metrics", server.MetricsHandler(metricsRegistry))
	httpMux.Handle("/dashboards/", server.DashboardsHandler(core.DashboardsMap(plugins)))

	httpServer := server.NewHTTPServer(httpAddr, httpMux)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalf("http serve: %v", err)
		}
	}()

	if err := grpcServer.Serve(); err != nil {
		log.Fatalf("grpc serve: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func buildPlugins(enabled map[string]bool, allowAll bool) []core.Plugin {
	plugins := make([]core.Plugin, 0)
	if allowAll || enabled["tado"] {
		plugins = append(plugins, tado.NewPlugin())
	}
	return plugins
}

func readEnabledPlugins(path string) (map[string]bool, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, true
	}
	defer file.Close()

	result := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		result[line] = true
	}

	if err := scanner.Err(); err != nil {
		return nil, true
	}

	return result, false
}
