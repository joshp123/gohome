package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/config"
	"github.com/joshp123/gohome/plugins/roborock"
)

func roborockMain(args []string) {
	if len(args) == 0 {
		roborockUsage()
		os.Exit(2)
	}

	switch args[0] {
	case "bootstrap":
		roborockBootstrapCmd(args[1:])
	case "probe-camera":
		roborockProbeCameraCmd(args[1:])
	default:
		roborockUsage()
		os.Exit(2)
	}
}

func roborockUsage() {
	fmt.Println("gohome roborock <command> [args]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  bootstrap --email user@example.com [--code 123456] [--config path] [--bootstrap-file path]")
	fmt.Println("  probe-camera [--device-id id] [--config path] [--methods name1,name2]")
}

func roborockBootstrapCmd(args []string) {
	flags := flag.NewFlagSet("roborock bootstrap", flag.ExitOnError)
	email := flags.String("email", "", "Roborock account email")
	code := flags.String("code", "", "Email verification code (if omitted, prompt)")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	bootstrapFile := flags.String("bootstrap-file", "", "Override bootstrap file path")
	_ = flags.Parse(args)

	if *email == "" {
		fatal("roborock bootstrap", fmt.Errorf("--email is required"))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("roborock bootstrap", err)
	}

	path := *bootstrapFile
	if path == "" {
		if cfg.Roborock == nil || cfg.Roborock.BootstrapFile == "" {
			fatal("roborock bootstrap", fmt.Errorf("roborock.bootstrap_file is required in config or via --bootstrap-file"))
		}
		path = cfg.Roborock.BootstrapFile
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := roborock.NewRoborockApiClient(*email, "")
	if err := client.RequestCodeV4(ctx); err != nil {
		fatal("roborock bootstrap", err)
	}
	fmt.Println("Verification code sent. Check your email.")

	if *code == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter code: ")
		text, _ := reader.ReadString('\n')
		*code = strings.TrimSpace(text)
	}
	if *code == "" {
		fatal("roborock bootstrap", fmt.Errorf("code is required"))
	}

	userData, err := client.CodeLoginV4(ctx, *code)
	if err != nil {
		userData, err = client.CodeLogin(ctx, *code)
	}
	if err != nil {
		fatal("roborock bootstrap", err)
	}

	baseURL, err := client.BaseURL(ctx)
	if err != nil {
		fatal("roborock bootstrap", err)
	}

	payload := roborock.BootstrapState{
		SchemaVersion: 1,
		Username:      *email,
		BaseURL:       baseURL,
	}
	payload.UserData, err = json.Marshal(userData)
	if err != nil {
		fatal("roborock bootstrap", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal("roborock bootstrap", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fatal("roborock bootstrap", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fatal("roborock bootstrap", err)
	}

	fmt.Printf("Wrote Roborock bootstrap to %s\n", path)
}

type probeMethod struct {
	name   string
	params any
}

func roborockProbeCameraCmd(args []string) {
	flags := flag.NewFlagSet("roborock probe-camera", flag.ExitOnError)
	deviceID := flags.String("device-id", "", "Roborock device id (optional; defaults to first device)")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	methods := flags.String("methods", "", "Comma-separated RPC methods to probe (optional)")
	timeout := flags.Duration("timeout", 30*time.Second, "Overall probe timeout")
	_ = flags.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("roborock probe-camera", err)
	}
	if cfg.Roborock == nil {
		fatal("roborock probe-camera", fmt.Errorf("roborock config is required"))
	}
	roboCfg, err := roborock.ConfigFromProto(cfg.Roborock)
	if err != nil {
		fatal("roborock probe-camera", err)
	}
	client, err := roborock.NewClient(roboCfg)
	if err != nil {
		fatal("roborock probe-camera", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if *deviceID == "" {
		devices, err := client.Devices(ctx)
		if err != nil {
			fatal("roborock probe-camera", err)
		}
		if len(devices) == 0 {
			fatal("roborock probe-camera", fmt.Errorf("no devices found"))
		}
		*deviceID = devices[0].ID
		fmt.Printf("Using device %s (%s)\n", devices[0].Name, *deviceID)
	}

	probes := []probeMethod{
		{name: "get_camera_status"},
		{name: "get_video_status"},
		{name: "get_camera_info"},
		{name: "get_video_info"},
		{name: "get_photo"},
		{name: "get_obstacle_photo"},
		{name: "get_camera_url"},
		{name: "get_video_url"},
		{name: "get_device_sdp"},
		{name: "get_device_ice"},
	}
	if *methods != "" {
		probes = probes[:0]
		for _, method := range strings.Split(*methods, ",") {
			method = strings.TrimSpace(method)
			if method == "" {
				continue
			}
			probes = append(probes, probeMethod{name: method})
		}
	}
	if len(probes) == 0 {
		fatal("roborock probe-camera", fmt.Errorf("no methods to probe"))
	}

	for _, probe := range probes {
		result, err := client.RawRPC(ctx, *deviceID, probe.name, probe.params)
		if err != nil {
			fmt.Printf("%s: error: %v\n", probe.name, err)
			continue
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Printf("%s: ok (non-json result)\n", probe.name)
			continue
		}
		fmt.Printf("%s: ok\n%s\n", probe.name, string(data))
	}
}
