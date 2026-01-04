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
