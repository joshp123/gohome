package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/agenix"
	"github.com/joshp123/gohome/internal/config"
	"github.com/joshp123/gohome/internal/oauth"
	"github.com/joshp123/gohome/internal/oauthflow"
	"github.com/joshp123/gohome/internal/plugins"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
	"golang.org/x/oauth2"
)

func oauthMain(args []string) {
	if len(args) == 0 {
		oauthUsage()
		os.Exit(2)
	}

	switch args[0] {
	case "auth-code":
		authCodeCmd(args[1:])
	case "device":
		deviceCmd(args[1:])
	case "persist":
		persistCmd(args[1:])
	default:
		oauthUsage()
		os.Exit(2)
	}
}

func oauthUsage() {
	fmt.Println("gohome oauth <command> [args]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  auth-code --provider <id> --redirect-url <url> [--config <path>] [--no-open]")
	fmt.Println("  device --provider <id> [--config <path>] [--no-open]")
	fmt.Println("  persist --provider <id> --state <path> [--config <path>]")
}

func authCodeCmd(args []string) {
	flags := flag.NewFlagSet("auth-code", flag.ExitOnError)
	provider := flags.String("provider", "", "OAuth provider ID")
	redirectURL := flags.String("redirect-url", "", "Redirect URL")
	bootstrapFile := flags.String("bootstrap-file", "", "Override bootstrap file path")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	noOpen := flags.Bool("no-open", false, "Do not open the browser automatically")
	stateOut := flags.String("state-out", "", "Write OAuth state to a temp file")
	statePath := flags.String("state-path", "", "Override persisted state path")
	cleanup := flags.Bool("cleanup", false, "Remove temp state file after successful persist")
	jsonOut := flags.Bool("json", false, "Output JSON to stdout")
	printToken := flags.Bool("print-token", false, "Include refresh token in output")
	persistAgenix := flags.Bool("persist-agenix", true, "Persist bootstrap secret via agenix")
	timeout := flags.Duration("timeout", 5*time.Minute, "Timeout for auth flow")
	agenixRepo := flags.String("agenix-repo", defaultAgenixRepo(), "Path to nix-secrets repo")
	agenixSecret := flags.String("agenix-secret", "", "Override agenix secret name")
	agenixRecipients := flags.String("agenix-recipients", "", "Space-separated recipient override")
	skipBlob := flags.Bool("skip-blob", false, "Skip blob storage persistence")
	_ = flags.Parse(args)

	if *provider == "" || *redirectURL == "" {
		oauthUsage()
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("oauth", err)
	}

	decl, err := lookupDeclaration(cfg, *provider)
	if err != nil {
		fatal("oauth", err)
	}
	if decl.Flow != oauth.FlowAuthCode {
		fatal("oauth", fmt.Errorf("provider %q uses %s flow, not auth-code", decl.Provider, decl.Flow))
	}
	if decl.AuthorizeURL == "" {
		fatal("oauth", fmt.Errorf("provider %q missing authorizeURL", decl.Provider))
	}
	if decl.TokenURL == "" {
		fatal("oauth", fmt.Errorf("provider %q missing tokenURL", decl.Provider))
	}
	if strings.TrimSpace(decl.Scope) == "" {
		fatal("oauth", fmt.Errorf("provider %q missing scope", decl.Provider))
	}

	bootstrapPath, err := resolveBootstrapPath(cfg, *provider, *bootstrapFile)
	if err != nil {
		fatal("oauth", err)
	}
	bootstrap, err := oauth.LoadBootstrap(bootstrapPath)
	if err != nil {
		fatal("oauth", err)
	}

	conf := &oauth2.Config{
		ClientID:     bootstrap.ClientID,
		ClientSecret: bootstrap.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  decl.AuthorizeURL,
			TokenURL: decl.TokenURL,
		},
		RedirectURL: *redirectURL,
		Scopes:      strings.Fields(decl.Scope),
	}

	state, err := randomState(16)
	if err != nil {
		fatal("oauth", err)
	}

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline)
	printAuthPrompt(*jsonOut, "Open this URL to authorize:", authURL, "")

	if !*noOpen {
		_ = openBrowser(authURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	code, err := waitForAuthCode(ctx, *redirectURL, state, *jsonOut)
	if err != nil {
		fatal("oauth", err)
	}

	token, err := conf.Exchange(ctx, code)
	if err != nil {
		fatal("oauth", err)
	}
	if token.RefreshToken == "" {
		fatal("oauth", fmt.Errorf("no refresh_token returned; check scope and redirect URL"))
	}

	output, err := persistOAuthState(ctx, cfg, decl, bootstrap, token.RefreshToken, oauthRunOptions{
		flow:             "auth-code",
		jsonOut:          *jsonOut,
		printToken:       *printToken,
		stateOut:         *stateOut,
		statePath:        *statePath,
		cleanup:          *cleanup,
		persistAgenix:    *persistAgenix,
		agenixRepo:       *agenixRepo,
		agenixSecret:     *agenixSecret,
		agenixRecipients: parseRecipients(*agenixRecipients),
		skipBlob:         *skipBlob,
	})
	if err != nil {
		fatal("oauth", err)
	}

	emitOAuthOutput(output, *jsonOut, *printToken)
}

func deviceCmd(args []string) {
	flags := flag.NewFlagSet("device", flag.ExitOnError)
	provider := flags.String("provider", "", "OAuth provider ID")
	bootstrapFile := flags.String("bootstrap-file", "", "Override bootstrap file path")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	noOpen := flags.Bool("no-open", false, "Do not open the browser automatically")
	stateOut := flags.String("state-out", "", "Write OAuth state to a temp file")
	statePath := flags.String("state-path", "", "Override persisted state path")
	cleanup := flags.Bool("cleanup", false, "Remove temp state file after successful persist")
	jsonOut := flags.Bool("json", false, "Output JSON to stdout")
	printToken := flags.Bool("print-token", false, "Include refresh token in output")
	persistAgenix := flags.Bool("persist-agenix", true, "Persist bootstrap secret via agenix")
	timeout := flags.Duration("timeout", 5*time.Minute, "Timeout for device flow")
	agenixRepo := flags.String("agenix-repo", defaultAgenixRepo(), "Path to nix-secrets repo")
	agenixSecret := flags.String("agenix-secret", "", "Override agenix secret name")
	agenixRecipients := flags.String("agenix-recipients", "", "Space-separated recipient override")
	skipBlob := flags.Bool("skip-blob", false, "Skip blob storage persistence")
	_ = flags.Parse(args)

	if *provider == "" {
		oauthUsage()
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("oauth", err)
	}

	decl, err := lookupDeclaration(cfg, *provider)
	if err != nil {
		fatal("oauth", err)
	}
	if decl.Flow != oauth.FlowDevice {
		fatal("oauth", fmt.Errorf("provider %q uses %s flow, not device", decl.Provider, decl.Flow))
	}
	if decl.DeviceAuthURL == "" || decl.DeviceTokenURL == "" {
		fatal("oauth", fmt.Errorf("provider %q missing device flow endpoints", decl.Provider))
	}
	if strings.TrimSpace(decl.Scope) == "" {
		fatal("oauth", fmt.Errorf("provider %q missing scope", decl.Provider))
	}

	bootstrapPath, err := resolveBootstrapPath(cfg, *provider, *bootstrapFile)
	if err != nil {
		fatal("oauth", err)
	}
	bootstrap, err := oauth.LoadBootstrap(bootstrapPath)
	if err != nil {
		fatal("oauth", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	authResp, err := deviceAuthorize(ctx, decl.DeviceAuthURL, bootstrap.ClientID, decl.Scope)
	if err != nil {
		fatal("oauth", err)
	}

	verifyURL := authResp.VerificationURIComplete
	if verifyURL == "" {
		verifyURL = authResp.VerificationURI
	}

	lines := []string{"Open this URL to authorize:", verifyURL}
	if authResp.UserCode != "" {
		lines = append(lines, fmt.Sprintf("User code: %s", authResp.UserCode))
	}
	lines = append(lines, "")
	printAuthPrompt(*jsonOut, lines...)

	if verifyURL != "" && !*noOpen {
		_ = openBrowser(verifyURL)
	}

	token, err := pollDeviceToken(ctx, decl.DeviceTokenURL, bootstrap, authResp)
	if err != nil {
		fatal("oauth", err)
	}
	if token.RefreshToken == "" {
		fatal("oauth", fmt.Errorf("no refresh_token returned; check scope and client id"))
	}

	output, err := persistOAuthState(ctx, cfg, decl, bootstrap, token.RefreshToken, oauthRunOptions{
		flow:             "device",
		jsonOut:          *jsonOut,
		printToken:       *printToken,
		stateOut:         *stateOut,
		statePath:        *statePath,
		cleanup:          *cleanup,
		persistAgenix:    *persistAgenix,
		agenixRepo:       *agenixRepo,
		agenixSecret:     *agenixSecret,
		agenixRecipients: parseRecipients(*agenixRecipients),
		skipBlob:         *skipBlob,
	})
	if err != nil {
		fatal("oauth", err)
	}
	output.VerifyURL = verifyURL
	if authResp.UserCode != "" {
		output.UserCode = authResp.UserCode
	}

	emitOAuthOutput(output, *jsonOut, *printToken)
}

func persistCmd(args []string) {
	flags := flag.NewFlagSet("persist", flag.ExitOnError)
	provider := flags.String("provider", "", "OAuth provider ID")
	statePath := flags.String("state", "", "Path to OAuth state file")
	configPath := flags.String("config", config.DefaultPath, "Path to config.pbtxt")
	cleanup := flags.Bool("cleanup", false, "Remove temp state file after successful persist")
	jsonOut := flags.Bool("json", false, "Output JSON to stdout")
	printToken := flags.Bool("print-token", false, "Include refresh token in output")
	persistAgenix := flags.Bool("persist-agenix", true, "Persist bootstrap secret via agenix")
	agenixRepo := flags.String("agenix-repo", defaultAgenixRepo(), "Path to nix-secrets repo")
	agenixSecret := flags.String("agenix-secret", "", "Override agenix secret name")
	agenixRecipients := flags.String("agenix-recipients", "", "Space-separated recipient override")
	skipBlob := flags.Bool("skip-blob", false, "Skip blob storage persistence")
	_ = flags.Parse(args)

	if *provider == "" || *statePath == "" {
		oauthUsage()
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("oauth", err)
	}

	decl, err := lookupDeclaration(cfg, *provider)
	if err != nil {
		fatal("oauth", err)
	}

	state, err := oauthflow.LoadState(*statePath)
	if err != nil {
		fatal("oauth", err)
	}

	bootstrap := oauth.Bootstrap{
		ClientID:     state.ClientID,
		ClientSecret: state.ClientSecret,
		RefreshToken: state.RefreshToken,
		Scope:        state.Scope,
	}

	output, err := persistLoadedState(context.Background(), cfg, decl, bootstrap, state, *statePath, false, oauthRunOptions{
		flow:             "persist",
		jsonOut:          *jsonOut,
		printToken:       *printToken,
		stateOut:         *statePath,
		cleanup:          *cleanup,
		persistAgenix:    *persistAgenix,
		agenixRepo:       *agenixRepo,
		agenixSecret:     *agenixSecret,
		agenixRecipients: parseRecipients(*agenixRecipients),
		skipBlob:         *skipBlob,
	})
	if err != nil {
		fatal("oauth", err)
	}

	emitOAuthOutput(output, *jsonOut, *printToken)
}

func lookupDeclaration(cfg *configv1.Config, provider string) (oauth.Declaration, error) {
	available := make([]string, 0)
	for _, plugin := range plugins.Compiled(cfg) {
		decl := plugin.OAuthDeclaration()
		if decl.Provider != "" {
			available = append(available, decl.Provider)
		}
		if decl.Provider == provider {
			return decl, nil
		}
	}

	if len(available) == 0 {
		return oauth.Declaration{}, fmt.Errorf("no providers compiled into this build")
	}

	return oauth.Declaration{}, fmt.Errorf("unknown provider %q (available: %s)", provider, strings.Join(available, ", "))
}

func resolveBootstrapPath(cfg *configv1.Config, provider, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.BootstrapPathForProvider(cfg, provider)
}

func waitForAuthCode(ctx context.Context, redirectURL, state string, jsonOut bool) (string, error) {
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URL: %w", err)
	}

	if isLoopback(parsed.Hostname()) && parsed.Scheme == "http" && parsed.Host != "" {
		code, err := listenForAuthCode(ctx, parsed, state)
		if err == nil {
			return code, nil
		}
		printAuthPrompt(jsonOut, fmt.Sprintf("Warning: failed to listen for callback, falling back to manual paste: %v", err))
	}

	if jsonOut {
		fmt.Fprint(os.Stderr, "Paste the authorization code (or full redirect URL): ")
	} else {
		fmt.Print("Paste the authorization code (or full redirect URL): ")
	}
	return readCodeFromStdin()
}

func listenForAuthCode(ctx context.Context, redirect *url.URL, state string) (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	srv := &http.Server{
		Addr: redirect.Host,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect.Path != "" && r.URL.Path != redirect.Path {
				http.NotFound(w, r)
				return
			}
			query := r.URL.Query()
			if errStr := query.Get("error"); errStr != "" {
				errCh <- fmt.Errorf("authorization error: %s", errStr)
				_, _ = w.Write([]byte("Authorization failed. You can close this window."))
				return
			}
			if got := query.Get("state"); got != "" && got != state {
				errCh <- fmt.Errorf("state mismatch")
				_, _ = w.Write([]byte("State mismatch. You can close this window."))
				return
			}
			code := query.Get("code")
			if code == "" {
				errCh <- fmt.Errorf("missing code in callback")
				_, _ = w.Write([]byte("Missing authorization code. You can close this window."))
				return
			}
			codeCh <- code
			_, _ = w.Write([]byte("Authorization received. You can close this window."))
		}),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !strings.Contains(err.Error(), "Server closed") {
			errCh <- err
		}
	}()
	defer func() {
		_ = srv.Close()
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("authorization timed out")
	case err := <-errCh:
		return "", err
	case code := <-codeCh:
		return code, nil
	}
}

func readCodeFromStdin() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("no code provided")
	}

	if parsed, err := url.Parse(line); err == nil && parsed.Query().Get("code") != "" {
		return parsed.Query().Get("code"), nil
	}

	return line, nil
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func deviceAuthorize(ctx context.Context, url, clientID, scope string) (deviceAuthResponse, error) {
	form := urlValues{
		"client_id": {clientID},
	}
	if scope != "" {
		form["scope"] = []string{scope}
	}
	var resp deviceAuthResponse
	if err := postForm(ctx, url, form, &resp); err != nil {
		return deviceAuthResponse{}, err
	}
	if resp.DeviceCode == "" {
		return deviceAuthResponse{}, fmt.Errorf("device authorization missing device_code")
	}
	if resp.Interval == 0 {
		resp.Interval = 5
	}
	if resp.ExpiresIn == 0 {
		resp.ExpiresIn = 300
	}
	return resp, nil
}

func pollDeviceToken(ctx context.Context, url string, bootstrap oauth.Bootstrap, auth deviceAuthResponse) (deviceTokenResponse, error) {
	start := time.Now()
	for {
		if time.Since(start) > time.Duration(auth.ExpiresIn)*time.Second {
			return deviceTokenResponse{}, fmt.Errorf("device authorization timed out")
		}

		form := urlValues{
			"client_id":   {bootstrap.ClientID},
			"device_code": {auth.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		if bootstrap.ClientSecret != "" {
			form["client_secret"] = []string{bootstrap.ClientSecret}
		}

		var token deviceTokenResponse
		err := postForm(ctx, url, form, &token)
		if err != nil {
			return deviceTokenResponse{}, err
		}
		if token.Error == "" && token.RefreshToken != "" {
			return token, nil
		}
		switch token.Error {
		case "authorization_pending":
			time.Sleep(time.Duration(auth.Interval) * time.Second)
			continue
		case "slow_down":
			time.Sleep(time.Duration(auth.Interval+2) * time.Second)
			continue
		default:
			if token.Error != "" {
				return deviceTokenResponse{}, fmt.Errorf("device token error: %s", token.Error)
			}
			return deviceTokenResponse{}, fmt.Errorf("device token missing refresh_token")
		}
	}
}

type urlValues map[string][]string

func postForm(ctx context.Context, endpoint string, values urlValues, out any) error {
	form := url.Values(values)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		if _, ok := out.(*deviceTokenResponse); ok {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("oauth http %d", resp.StatusCode)
			}
			return nil
		}

		var body deviceTokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error != "" {
			return fmt.Errorf("oauth error %d: %s", resp.StatusCode, body.Error)
		}
		return fmt.Errorf("oauth http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "linux":
		return exec.Command("xdg-open", target).Start()
	default:
		return nil
	}
}

func randomState(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

type oauthRunOptions struct {
	flow             string
	jsonOut          bool
	printToken       bool
	stateOut         string
	statePath        string
	cleanup          bool
	persistAgenix    bool
	skipBlob         bool
	agenixRepo       string
	agenixSecret     string
	agenixRecipients []string
}

type oauthOutput struct {
	Provider        string `json:"provider"`
	Flow            string `json:"flow"`
	VerifyURL       string `json:"verify_url,omitempty"`
	UserCode        string `json:"user_code,omitempty"`
	StatePath       string `json:"state_path,omitempty"`
	StateOut        string `json:"state_out,omitempty"`
	BlobPersisted   bool   `json:"blob_persisted,omitempty"`
	AgenixPersisted bool   `json:"agenix_persisted,omitempty"`
	AgenixPath      string `json:"agenix_path,omitempty"`
	RefreshToken    string `json:"refresh_token,omitempty"`
}

func persistOAuthState(ctx context.Context, cfg *configv1.Config, decl oauth.Declaration, bootstrap oauth.Bootstrap, refreshToken string, opts oauthRunOptions) (oauthOutput, error) {
	if bootstrap.Scope == "" {
		bootstrap.Scope = decl.Scope
	}
	state := oauth.State{
		SchemaVersion: oauth.SchemaVersion,
		ClientID:      bootstrap.ClientID,
		ClientSecret:  bootstrap.ClientSecret,
		RefreshToken:  refreshToken,
		Scope:         decl.Scope,
	}
	return persistLoadedState(ctx, cfg, decl, bootstrap, state, opts.stateOut, true, opts)
}

func persistLoadedState(ctx context.Context, cfg *configv1.Config, decl oauth.Declaration, bootstrap oauth.Bootstrap, state oauth.State, tempPath string, writeTemp bool, opts oauthRunOptions) (oauthOutput, error) {
	output := oauthOutput{Provider: decl.Provider, Flow: opts.flow}
	path := tempPath
	if path == "" {
		path = oauthflow.DefaultTempPath(decl.Provider)
	}
	if writeTemp {
		if _, err := oauthflow.WriteTempState(path, state); err != nil {
			return output, err
		}
	}
	output.StateOut = path

	var blobStore oauth.BlobStore
	if !opts.skipBlob {
		store, err := oauth.NewS3Store(cfg.Oauth)
		if err != nil {
			return output, err
		}
		blobStore = store
	}
	persistResult, err := oauthflow.PersistState(ctx, decl, state, blobStore, oauthflow.PersistOptions{
		SkipBlob:          opts.skipBlob,
		StatePathOverride: opts.statePath,
	})
	if err != nil {
		return output, err
	}
	output.StatePath = persistResult.StatePath
	output.BlobPersisted = persistResult.BlobSaved

	if opts.persistAgenix {
		agenixPath, err := persistAgenixBootstrap(ctx, decl.Provider, bootstrap, opts)
		if err != nil {
			return output, err
		}
		output.AgenixPersisted = true
		output.AgenixPath = agenixPath
	}

	if opts.printToken {
		output.RefreshToken = state.RefreshToken
	}

	if opts.cleanup && output.StateOut != "" {
		if err := os.Remove(output.StateOut); err != nil {
			fmt.Fprintf(os.Stderr, "oauth: cleanup failed: %v\n", err)
		}
	}

	return output, nil
}

func emitOAuthOutput(output oauthOutput, jsonOut bool, printToken bool) {
	if !printToken {
		output.RefreshToken = ""
	}
	if jsonOut {
		payload, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			fatal("oauth", err)
		}
		fmt.Fprintln(os.Stdout, string(payload))
		return
	}

	if output.StatePath != "" {
		fmt.Printf("State file: %s\n", output.StatePath)
	}
	if output.StateOut != "" {
		fmt.Printf("Temp state file: %s\n", output.StateOut)
	}
	fmt.Printf("Blob persisted: %t\n", output.BlobPersisted)
	if output.AgenixPersisted {
		fmt.Printf("Agenix secret: %s\n", output.AgenixPath)
	}
	if printToken && output.RefreshToken != "" {
		fmt.Printf("Refresh token: %s\n", output.RefreshToken)
	}
}

func printAuthPrompt(jsonOut bool, lines ...string) {
	out := os.Stdout
	if jsonOut {
		out = os.Stderr
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
}

func parseRecipients(raw string) []string {
	return strings.Fields(raw)
}

func defaultAgenixRepo() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	repo := filepath.Join(home, "code", "nix", "nix-secrets")
	info, err := os.Stat(repo)
	if err != nil || !info.IsDir() {
		return ""
	}
	return repo
}

func defaultAgenixSecret(provider string) string {
	return fmt.Sprintf("gohome-%s-bootstrap.age", provider)
}

func persistAgenixBootstrap(ctx context.Context, provider string, bootstrap oauth.Bootstrap, opts oauthRunOptions) (string, error) {
	repo := strings.TrimSpace(opts.agenixRepo)
	if repo == "" {
		return "", fmt.Errorf("agenix repo not configured")
	}
	secret := strings.TrimSpace(opts.agenixSecret)
	if secret == "" {
		secret = defaultAgenixSecret(provider)
	}
	payload, err := json.MarshalIndent(bootstrap, "", "  ")
	if err != nil {
		return "", err
	}
	writer := agenix.Writer{
		RepoPath:   repo,
		SecretName: secret,
		Recipients: opts.agenixRecipients,
	}
	return writer.Write(ctx, payload)
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
