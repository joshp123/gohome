package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/joshp123/gohome/internal/config"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"google.golang.org/grpc"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	if args[0] == "-h" || args[0] == "--help" {
		usage()
		os.Exit(0)
	}

	jsonOutput := false
	for len(args) > 0 {
		if args[0] == "--json" {
			jsonOutput = true
			args = args[1:]
			continue
		}
		break
	}

	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	addr := resolveAddr()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpcurl.BlockingDial(ctx, "tcp", addr, insecure.NewCredentials())
	if err != nil {
		fatal("dial", err)
	}
	defer conn.Close()

	switch args[0] {
	case "plugins":
		pluginsCmd(ctx, conn, args[1:])
	case "services":
		servicesCmd(ctx, conn)
	case "methods":
		methodsCmd(ctx, conn, args[1:])
	case "call":
		callCmd(ctx, conn, args[1:])
	case "tado":
		tadoCmd(ctx, conn, args[1:], jsonOutput)
	case "roborock":
		roborockCmd(ctx, conn, args[1:], jsonOutput)
	default:
		usage()
		os.Exit(2)
	}
}

func pluginsCmd(ctx context.Context, conn *grpc.ClientConn, args []string) {
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}

	client := registryv1.NewRegistryClient(conn)
	switch args[0] {
	case "list":
		resp, err := client.ListPlugins(ctx, &registryv1.ListPluginsRequest{})
		if err != nil {
			fatal("list plugins", err)
		}
		for _, plugin := range resp.Plugins {
			fmt.Printf("%s\t%s\t%s\t%s\n", plugin.PluginId, plugin.DisplayName, plugin.Version, plugin.Status)
		}
	case "describe":
		if len(args) < 2 {
			fatal("describe", fmt.Errorf("missing plugin id"))
		}
		resp, err := client.DescribePlugin(ctx, &registryv1.DescribePluginRequest{PluginId: args[1]})
		if err != nil {
			fatal("describe plugin", err)
		}
		if resp.Plugin == nil {
			fmt.Println("not found")
			return
		}
		fmt.Printf("id: %s\n", resp.Plugin.PluginId)
		fmt.Printf("name: %s\n", resp.Plugin.DisplayName)
		fmt.Printf("version: %s\n", resp.Plugin.Version)
		fmt.Printf("status: %s\n", resp.Plugin.Status)
		if resp.Plugin.HealthMessage != "" {
			fmt.Printf("health: %s\n", resp.Plugin.HealthMessage)
		}
		fmt.Println("services:")
		for _, svc := range resp.Plugin.Services {
			fmt.Printf("  - %s\n", svc)
		}
		fmt.Println("dashboards:")
		for _, dash := range resp.Plugin.Dashboards {
			fmt.Printf("  - %s (%s)\n", dash.Name, dash.Path)
		}
		fmt.Println("agents_md:")
		fmt.Println(resp.Plugin.AgentsMd)
	default:
		usage()
		os.Exit(2)
	}
}

func servicesCmd(ctx context.Context, conn *grpc.ClientConn) {
	descSource := reflectionSource(ctx, conn)
	services, err := grpcurl.ListServices(descSource)
	if err != nil {
		fatal("list services", err)
	}

	for _, service := range services {
		fmt.Println(service)
	}
}

func methodsCmd(ctx context.Context, conn *grpc.ClientConn, args []string) {
	if len(args) < 1 {
		fatal("methods", fmt.Errorf("missing service name"))
	}

	descSource := reflectionSource(ctx, conn)
	methods, err := grpcurl.ListMethods(descSource, args[0])
	if err != nil {
		fatal("list methods", err)
	}

	for _, method := range methods {
		fmt.Println(method)
	}
}

func callCmd(ctx context.Context, conn *grpc.ClientConn, args []string) {
	flags := flag.NewFlagSet("call", flag.ExitOnError)
	data := flags.String("data", "", "JSON request body")
	_ = flags.Parse(args)
	remaining := flags.Args()
	if len(remaining) < 1 {
		fatal("call", fmt.Errorf("missing method (service/method)"))
	}

	method := remaining[0]
	descSource := reflectionSource(ctx, conn)

	var reader io.Reader
	if *data != "" {
		reader = strings.NewReader(*data)
	} else if isStdinTerminal() {
		reader = strings.NewReader("{}")
	} else {
		reader = os.Stdin
	}

	parser, formatter, err := grpcurl.RequestParserAndFormatter(grpcurl.FormatJSON, descSource, reader, grpcurl.FormatOptions{})
	if err != nil {
		fatal("parse request", err)
	}

	handler := grpcurl.NewDefaultEventHandler(os.Stdout, descSource, formatter, false)
	if err := grpcurl.InvokeRPC(ctx, descSource, conn, method, nil, handler, parser.Next); err != nil {
		fatal("invoke", err)
	}
}

func reflectionSource(ctx context.Context, conn *grpc.ClientConn) grpcurl.DescriptorSource {
	client := grpcreflect.NewClientAuto(ctx, conn)
	return grpcurl.DescriptorSourceFromServer(ctx, client)
}

func isStdinTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func resolveAddr() string {
	if value := os.Getenv("GOHOME_GRPC_ADDR"); value != "" {
		return value
	}
	for _, path := range configSearchPaths() {
		if addr := addrFromConfig(path); addr != "" {
			return addr
		}
	}
	return "gohome:9000"
}

func resolveHTTPBase() string {
	if value := os.Getenv("GOHOME_HTTP_BASE"); value != "" {
		return strings.TrimRight(value, "/")
	}
	if value := os.Getenv("GOHOME_HTTP_ADDR"); value != "" {
		return fmt.Sprintf("http://%s", normalizeHTTPAddr(value))
	}
	for _, path := range configSearchPaths() {
		if addr := httpAddrFromConfig(path); addr != "" {
			return fmt.Sprintf("http://%s", normalizeHTTPAddr(addr))
		}
	}
	return "http://localhost:8080"
}

func httpAddrFromConfig(path string) string {
	cfg, err := config.Load(path)
	if err != nil || cfg == nil || cfg.Core == nil {
		return ""
	}
	return cfg.Core.HttpAddr
}

func normalizeHTTPAddr(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "localhost:8080"
	}
	if strings.HasPrefix(trimmed, "0.0.0.0:") {
		return "127.0.0.1:" + strings.TrimPrefix(trimmed, "0.0.0.0:")
	}
	if strings.HasPrefix(trimmed, "[::]:") {
		return "127.0.0.1:" + strings.TrimPrefix(trimmed, "[::]:")
	}
	if strings.HasPrefix(trimmed, ":") {
		return "127.0.0.1" + trimmed
	}
	return trimmed
}

func configSearchPaths() []string {
	paths := []string{config.DefaultPath}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "gohome", "config.pbtxt"))
	}
	return paths
}

func addrFromConfig(path string) string {
	cfg, err := config.Load(path)
	if err != nil || cfg == nil || cfg.Core == nil {
		return ""
	}
	return cfg.Core.GrpcAddr
}

func usage() {
	fmt.Println("gohome-cli <command> [args]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  tado <zones|set>")
	fmt.Println("  roborock <status|rooms|clean|dock|locate|map>")
	fmt.Println("  plugins list")
	fmt.Println("  plugins describe <plugin_id>")
	fmt.Println("  services")
	fmt.Println("  methods <service>")
	fmt.Println("  call <service/method> --data '{}' (or pipe JSON via stdin)")
	fmt.Println("")
	fmt.Println("Global flags:")
	fmt.Println("  --json  output raw JSON")
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
