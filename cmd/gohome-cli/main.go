package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/joshp123/gohome/internal/config"
	"github.com/joshp123/gohome/internal/render"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type pluginNodeEntry struct {
	Node        string `json:"node"`
	PluginID    string `json:"plugin_id"`
	DisplayName string `json:"display_name"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	jsonOutput := false
	addrsFlag := ""
	for len(args) > 0 {
		switch {
		case args[0] == "-h" || args[0] == "--help":
			out := render.NewRenderer(os.Stdout, os.Stderr, render.FormatText)
			writeRootUsage(out)
			return render.ExitOK
		case args[0] == "--json":
			jsonOutput = true
			args = args[1:]
		case args[0] == "--addrs" && len(args) > 1:
			addrsFlag = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "--addrs="):
			addrsFlag = strings.TrimPrefix(args[0], "--addrs=")
			args = args[1:]
		default:
			goto parsed
		}
	}

parsed:
	format := render.FormatText
	if jsonOutput {
		format = render.FormatJSON
	}
	out := render.NewRenderer(os.Stdout, os.Stderr, format)

	if len(args) == 0 {
		writeRootUsage(out)
		return render.ExitUsageError
	}

	addrs := resolveAddrs(addrsFlag)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch args[0] {
	case "plugins":
		return pluginsCmd(ctx, out, addrs, args[1:])
	case "services":
		return servicesCmd(ctx, out, addrs)
	case "methods":
		return methodsCmd(ctx, out, addrs, args[1:])
	case "call":
		return callCmd(ctx, out, addrs[0], args[1:])
	case "tado":
		return withPluginConn(ctx, out, addrs, "tado", func(conn *grpc.ClientConn) int {
			return tadoCmd(ctx, conn, out, args[1:])
		})
	case "roborock":
		return withPluginConn(ctx, out, addrs, "roborock", func(conn *grpc.ClientConn) int {
			return roborockCmd(ctx, conn, out, args[1:])
		})
	case "airgradient":
		return withPluginConn(ctx, out, addrs, "airgradient", func(conn *grpc.ClientConn) int {
			return airgradientCmd(ctx, conn, out, args[1:])
		})
	case "hikvision":
		return withPluginConn(ctx, out, addrs, "hikvision", func(conn *grpc.ClientConn) int {
			return hikvisionCmd(ctx, conn, out, args[1:])
		})
	case "p1":
		return withPluginConn(ctx, out, addrs, "p1_homewizard", func(conn *grpc.ClientConn) int {
			return p1Cmd(ctx, conn, out, args[1:])
		})
	case "weheat":
		return withPluginConn(ctx, out, addrs, "weheat", func(conn *grpc.ClientConn) int {
			return weheatCmd(ctx, conn, out, args[1:])
		})
	case "daikin":
		return withPluginConn(ctx, out, addrs, "daikin", func(conn *grpc.ClientConn) int {
			return daikinCmd(ctx, conn, out, args[1:])
		})
	case "growatt":
		return withPluginConn(ctx, out, addrs, "growatt", func(conn *grpc.ClientConn) int {
			return growattCmd(ctx, conn, out, args[1:])
		})
	case "shelly":
		return withPluginConn(ctx, out, addrs, "shelly", func(conn *grpc.ClientConn) int {
			return shellyCmd(ctx, conn, out, args[1:])
		})
	case "sonos":
		return withPluginConn(ctx, out, addrs, "sonos", func(conn *grpc.ClientConn) int {
			return sonosCmd(ctx, conn, out, args[1:])
		})
	case "casambi":
		return withPluginConn(ctx, out, addrs, "casambi", func(conn *grpc.ClientConn) int {
			return casambiCmd(ctx, conn, out, args[1:])
		})
	case "brother":
		return withPluginConn(ctx, out, addrs, "brother", func(conn *grpc.ClientConn) int {
			return brotherCmd(ctx, conn, out, args[1:])
		})
	case "unifi":
		return withPluginConn(ctx, out, addrs, "unifi", func(conn *grpc.ClientConn) int {
			return unifiCmd(ctx, conn, out, args[1:])
		})
	default:
		out.WriteError("gohome-cli", fmt.Errorf("unknown command %q", args[0]), render.ExitUsageError)
		writeRootUsage(out)
		return render.ExitUsageError
	}
}

func withPluginConn(ctx context.Context, out render.Renderer, addrs []string, pluginID string, fn func(*grpc.ClientConn) int) int {
	conn, err := dialPluginNode(ctx, addrs, pluginID)
	if err != nil {
		out.WriteError(pluginID, err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	defer func() { _ = conn.Close() }()
	return fn(conn)
}

func dialPluginNode(ctx context.Context, addrs []string, pluginID string) (*grpc.ClientConn, error) {
	if len(addrs) == 1 {
		return dialNode(ctx, addrs[0])
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := dialNode(ctx, addr)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", addr, err)
			continue
		}
		client := registryv1.NewRegistryClient(conn)
		resp, err := client.ListPlugins(ctx, &registryv1.ListPluginsRequest{})
		if err != nil {
			_ = conn.Close()
			lastErr = fmt.Errorf("%s: %w", addr, err)
			continue
		}
		for _, plugin := range resp.GetPlugins() {
			if strings.EqualFold(plugin.GetPluginId(), pluginID) {
				return conn, nil
			}
		}
		_ = conn.Close()
	}
	if lastErr != nil {
		return nil, fmt.Errorf("plugin %q not found on %v (last error: %w)", pluginID, addrs, lastErr)
	}
	return nil, fmt.Errorf("plugin %q not hosted on any configured node (%v)", pluginID, addrs)
}

func dialNode(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	return grpcurl.BlockingDial(ctx, "tcp", addr, insecure.NewCredentials())
}

func pluginsCmd(ctx context.Context, out render.Renderer, addrs []string, args []string) int {
	if len(args) < 1 {
		out.WriteHelp("gohome-cli plugins <list|describe <plugin_id>>", nil)
		return render.ExitUsageError
	}

	switch args[0] {
	case "list":
		var entries []pluginNodeEntry
		var dialErrors []string
		for _, addr := range addrs {
			conn, err := dialNode(ctx, addr)
			if err != nil {
				dialErrors = append(dialErrors, fmt.Sprintf("%s: %v", addr, err))
				continue
			}
			client := registryv1.NewRegistryClient(conn)
			resp, err := client.ListPlugins(ctx, &registryv1.ListPluginsRequest{})
			_ = conn.Close()
			if err != nil {
				dialErrors = append(dialErrors, fmt.Sprintf("%s: %v", addr, err))
				continue
			}
			for _, plugin := range resp.GetPlugins() {
				entries = append(entries, pluginNodeEntry{
					Node:        addr,
					PluginID:    plugin.GetPluginId(),
					DisplayName: plugin.GetDisplayName(),
					Version:     plugin.GetVersion(),
					Status:      plugin.GetStatus(),
				})
			}
		}
		if len(entries) == 0 && len(dialErrors) > 0 {
			out.WriteError("plugins list", fmt.Errorf("%s", strings.Join(dialErrors, "; ")), render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].PluginID == entries[j].PluginID {
				return entries[i].Node < entries[j].Node
			}
			return entries[i].PluginID < entries[j].PluginID
		})
		if out.Format == render.FormatJSON {
			_ = out.WriteJSON(entries)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{e.PluginID, e.DisplayName, e.Status, e.Node})
		}
		_ = out.WriteTable([]string{"PLUGIN", "NAME", "STATUS", "NODE"}, rows)
		return render.ExitOK

	case "describe":
		if len(args) < 2 {
			out.WriteError("plugins describe", fmt.Errorf("missing plugin id"), render.ExitUsageError)
			return render.ExitUsageError
		}
		pluginID := args[1]
		return withPluginConn(ctx, out, addrs, pluginID, func(conn *grpc.ClientConn) int {
			client := registryv1.NewRegistryClient(conn)
			resp, err := client.DescribePlugin(ctx, &registryv1.DescribePluginRequest{PluginId: pluginID})
			if err != nil {
				out.WriteError("describe plugin", err, render.ExitRuntimeError)
				return render.ExitRuntimeError
			}
			if resp.Plugin == nil {
				out.WriteError("describe plugin", fmt.Errorf("plugin %q not found", pluginID), render.ExitRuntimeError)
				return render.ExitRuntimeError
			}
			if out.Format == render.FormatJSON {
				_ = out.WriteProto(resp.Plugin)
				return render.ExitOK
			}
			pairs := []render.KeyValue{
				{Key: "id", Value: resp.Plugin.GetPluginId()},
				{Key: "name", Value: resp.Plugin.GetDisplayName()},
				{Key: "version", Value: resp.Plugin.GetVersion()},
				{Key: "status", Value: resp.Plugin.GetStatus()},
			}
			if resp.Plugin.GetHealthMessage() != "" {
				pairs = append(pairs, render.KeyValue{Key: "health", Value: resp.Plugin.GetHealthMessage()})
			}
			if resp.Plugin.LastSuccess != nil {
				pairs = append(pairs, render.KeyValue{Key: "last_success", Value: resp.Plugin.LastSuccess.AsTime().Format(time.RFC3339)})
			}
			if len(resp.Plugin.Services) > 0 {
				pairs = append(pairs, render.KeyValue{Key: "services", Value: strings.Join(resp.Plugin.Services, ", ")})
			}
			_ = out.WriteKeyValues(pairs)
			if resp.Plugin.GetAgentsMd() != "" {
				_ = out.WriteLine("")
				_ = out.WriteLine(resp.Plugin.GetAgentsMd())
			}
			return render.ExitOK
		})
	default:
		out.WriteHelp("gohome-cli plugins <list|describe <plugin_id>>", nil)
		return render.ExitUsageError
	}
}

func servicesCmd(ctx context.Context, out render.Renderer, addrs []string) int {
	rows := make([][]string, 0)
	type svcEntry struct {
		Node    string `json:"node"`
		Service string `json:"service"`
	}
	var entries []svcEntry
	for _, addr := range addrs {
		conn, err := dialNode(ctx, addr)
		if err != nil {
			continue
		}
		descSource := reflectionSource(ctx, conn)
		services, err := grpcurl.ListServices(descSource)
		_ = conn.Close()
		if err != nil {
			continue
		}
		for _, svc := range services {
			entries = append(entries, svcEntry{Node: addr, Service: svc})
			rows = append(rows, []string{svc, addr})
		}
	}
	if len(entries) == 0 {
		out.WriteError("services", fmt.Errorf("no services discovered on %v", addrs), render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	if out.Format == render.FormatJSON {
		_ = out.WriteJSON(entries)
		return render.ExitOK
	}
	_ = out.WriteTable([]string{"SERVICE", "NODE"}, rows)
	return render.ExitOK
}

func methodsCmd(ctx context.Context, out render.Renderer, addrs []string, args []string) int {
	if len(args) < 1 {
		out.WriteError("methods", fmt.Errorf("missing service name"), render.ExitUsageError)
		return render.ExitUsageError
	}
	serviceName := args[0]
	for _, addr := range addrs {
		conn, err := dialNode(ctx, addr)
		if err != nil {
			continue
		}
		descSource := reflectionSource(ctx, conn)
		methods, err := grpcurl.ListMethods(descSource, serviceName)
		_ = conn.Close()
		if err != nil {
			continue
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteJSON(methods)
			return render.ExitOK
		}
		for _, method := range methods {
			_ = out.WriteLine(method)
		}
		return render.ExitOK
	}
	out.WriteError("methods", fmt.Errorf("service %q not found on %v", serviceName, addrs), render.ExitRuntimeError)
	return render.ExitRuntimeError
}

func callCmd(ctx context.Context, out render.Renderer, addr string, args []string) int {
	flags := flag.NewFlagSet("call", flag.ContinueOnError)
	data := flags.String("data", "", "JSON request body")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		out.WriteError("call", err, render.ExitUsageError)
		return render.ExitUsageError
	}
	remaining := flags.Args()
	if len(remaining) < 1 {
		out.WriteError("call", fmt.Errorf("missing method (service/method)"), render.ExitUsageError)
		return render.ExitUsageError
	}

	conn, err := dialNode(ctx, addr)
	if err != nil {
		out.WriteError("call dial", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	defer func() { _ = conn.Close() }()

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
		out.WriteError("parse request", err, render.ExitUsageError)
		return render.ExitUsageError
	}

	handler := &grpcurl.DefaultEventHandler{
		Out:       os.Stdout,
		Formatter: formatter,
	}
	if err := grpcurl.InvokeRPC(ctx, descSource, conn, method, nil, handler, parser.Next); err != nil {
		out.WriteError("invoke", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	if err := handler.Status.Err(); err != nil {
		out.WriteError(method, err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	return render.ExitOK
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

func resolveAddrs(flagValue string) []string {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOHOME_GRPC_ADDRS"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOHOME_GRPC_ADDR"))
	}
	if raw != "" {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	for _, path := range configSearchPaths() {
		if addr := addrFromConfig(path); addr != "" {
			return []string{addr}
		}
	}
	return []string{"gohome:9000"}
}

func resolveHTTPBase() string {
	if value := os.Getenv("GOHOME_HTTP_BASE"); value != "" {
		return strings.TrimRight(value, "/")
	}
	httpAddr := ""
	grpcAddr := ""
	for _, path := range configSearchPaths() {
		if httpAddr == "" {
			httpAddr = httpAddrFromConfig(path)
		}
		if grpcAddr == "" {
			grpcAddr = addrFromConfig(path)
		}
	}
	if httpAddr == "" {
		httpAddr = "0.0.0.0:8080"
	}
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return "http://gohome:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "127.0.0.1" {
		if grpcAddr != "" {
			if grpcHost, _, err := net.SplitHostPort(grpcAddr); err == nil && grpcHost != "" && grpcHost != "0.0.0.0" {
				host = grpcHost
			}
		}
		if host == "" || host == "0.0.0.0" {
			host = "gohome"
		}
	}
	return "http://" + net.JoinHostPort(host, port)
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

func httpAddrFromConfig(path string) string {
	cfg, err := config.Load(path)
	if err != nil || cfg == nil || cfg.Core == nil {
		return ""
	}
	return cfg.Core.HttpAddr
}

func writeRootUsage(out render.Renderer) {
	out.WriteHelp("gohome-cli [--json] [--addrs host1:9000,host2:9000] <command> [args]", []render.HelpSection{
		{
			Heading: "Read-only device commands (writes live in gohome-act)",
			Lines: []string{
				"p1 <status|info>",
				"weheat <status|energy>",
				"tado <zones>",
				"daikin <list|status [unit_id]>",
				"growatt <status>",
				"roborock <status|rooms|map>",
				"airgradient <current|metrics>",
				"shelly <list>",
				"sonos <status|zones>",
				"casambi <lights|scenes|switches>",
				"hikvision <camera|snapshot|events|alarms|watch-alarms|frame>",
				"brother <status>",
				"unifi <status>",
			},
		},
		{
			Heading: "Discovery",
			Lines: []string{
				"plugins list",
				"plugins describe <plugin_id>",
				"services",
				"methods <service>",
				"call <service/method> --data '{}'",
			},
		},
	})
}
