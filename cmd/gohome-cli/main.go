package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc/credentials/insecure"

	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"google.golang.org/grpc"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	addr := envOrDefault("GOHOME_GRPC_ADDR", "127.0.0.1:9000")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpcurl.BlockingDial(ctx, "tcp", addr, insecure.NewCredentials())
	if err != nil {
		fatal("dial", err)
	}
	defer conn.Close()

	switch os.Args[1] {
	case "plugins":
		pluginsCmd(ctx, conn, os.Args[2:])
	case "services":
		servicesCmd(ctx, conn)
	case "methods":
		methodsCmd(ctx, conn, os.Args[2:])
	case "call":
		callCmd(ctx, conn, os.Args[2:])
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

func usage() {
	fmt.Println("gohome-cli <command> [args]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  plugins list")
	fmt.Println("  plugins describe <plugin_id>")
	fmt.Println("  services")
	fmt.Println("  methods <service>")
	fmt.Println("  call <service/method> --data '{}' (or pipe JSON via stdin)")
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
