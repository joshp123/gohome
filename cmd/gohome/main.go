package main

import (
	"log"
	"net/http"
	"os"

	"github.com/elliot-alderson/gohome/internal/core"
	"github.com/elliot-alderson/gohome/internal/router"
	"github.com/elliot-alderson/gohome/internal/server"
	"github.com/elliot-alderson/gohome/plugins/tado"

	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	grpcAddr := envOrDefault("GOHOME_GRPC_ADDR", ":9000")
	httpAddr := envOrDefault("GOHOME_HTTP_ADDR", ":8080")

	plugins := []core.Plugin{
		tado.Plugin{},
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
