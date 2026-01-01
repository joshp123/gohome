package main

import (
	cryptoRand "crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAuthorizeURL = "https://idp.onecta.daikineurope.com/v1/oidc/authorize"
	defaultTokenURL     = "https://idp.onecta.daikineurope.com/v1/oidc/token"
	defaultScope        = "openid onecta:basic.integration"
	defaultRedirectURI  = "http://127.0.0.1:8765/callback"
)

type credentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	Scope        string
}

type credentialsFile struct {
	DaikinOnecta credentials `json:"daikin_onecta"`
	ClientID     string      `json:"client_id"`
	ClientSecret string      `json:"client_secret"`
	RefreshToken string      `json:"refresh_token"`
	Scope        string      `json:"scope"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func main() {
	var (
		credentialsFilePath = flag.String("credentials-file", "", "Path to credentials file (YAML or JSON) with client_id/client_secret")
		clientID            = flag.String("client-id", "", "Daikin client ID")
		clientSecret        = flag.String("client-secret", "", "Daikin client secret")
		scope               = flag.String("scope", defaultScope, "OAuth scope")
		authorizeURL        = flag.String("authorize-url", defaultAuthorizeURL, "Authorize URL")
		tokenURL            = flag.String("token-url", defaultTokenURL, "Token URL")
		redirectURI         = flag.String("redirect-uri", defaultRedirectURI, "Redirect URI for authorization code flow")
		outPath             = flag.String("out", "/tmp/daikin-onecta-refresh.json", "Output path for refresh token JSON")
		nowOpen             = flag.Bool("no-open", false, "Do not open the browser automatically")
		timeoutSeconds      = flag.Int("timeout", 300, "Seconds to wait for authorization")
	)
	flag.Parse()

	creds := credentials{ClientID: strings.TrimSpace(*clientID), ClientSecret: strings.TrimSpace(*clientSecret), Scope: strings.TrimSpace(*scope)}
	if (creds.ClientID == "" || creds.ClientSecret == "") && *credentialsFilePath != "" {
		fileCreds, err := loadCredentials(*credentialsFilePath)
		if err != nil {
			fatal(err)
		}
		if creds.ClientID == "" {
			creds.ClientID = fileCreds.ClientID
		}
		if creds.ClientSecret == "" {
			creds.ClientSecret = fileCreds.ClientSecret
		}
		if creds.Scope == "" {
			creds.Scope = fileCreds.Scope
		}
	}

	if creds.ClientID == "" || creds.ClientSecret == "" {
		fatal(errors.New("client-id and client-secret are required (or provide credentials-file)"))
	}
	if creds.Scope == "" {
		creds.Scope = defaultScope
	}

	parsedRedirect, err := url.Parse(*redirectURI)
	if err != nil || parsedRedirect.Host == "" {
		fatal(fmt.Errorf("invalid redirect-uri: %q", *redirectURI))
	}

	state, err := randomState(16)
	if err != nil {
		fatal(err)
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{
		Addr: parsedRedirect.Host,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query()
			if errStr := query.Get("error"); errStr != "" {
				errCh <- fmt.Errorf("authorization error: %s", errStr)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Authorization failed. You can close this window."))
				return
			}
			if query.Get("state") != state {
				errCh <- fmt.Errorf("state mismatch")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("State mismatch. You can close this window."))
				return
			}
			code := query.Get("code")
			if code == "" {
				errCh <- fmt.Errorf("missing code in callback")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Missing authorization code. You can close this window."))
				return
			}
			codeCh <- code
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Authorization received. You can close this window."))
		}),
	}

	go func() {
		_ = srv.ListenAndServe()
	}()
	defer func() {
		_ = srv.Close()
	}()

	authURL, err := buildAuthorizeURL(*authorizeURL, creds.ClientID, *redirectURI, creds.Scope, state)
	if err != nil {
		fatal(err)
	}

	fmt.Println("Open this URL to authorize Daikin Onecta:")
	fmt.Println(authURL)
	fmt.Println("")
	fmt.Printf("Redirect URI: %s\n", *redirectURI)
	fmt.Println("Make sure this redirect URI is registered in the Daikin Developer Portal app.")

	if !*nowOpen {
		_ = openBrowser(authURL)
	}

	timeout := time.After(time.Duration(*timeoutSeconds) * time.Second)
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		fatal(err)
	case <-timeout:
		fatal(errors.New("timed out waiting for authorization"))
	}

	token, err := exchangeToken(*tokenURL, creds, *redirectURI, code)
	if err != nil {
		fatal(err)
	}

	if token.RefreshToken == "" {
		fatal(errors.New("no refresh_token returned; check scope and redirect URI"))
	}

	out := credentialsFile{DaikinOnecta: credentials{ClientID: creds.ClientID, ClientSecret: creds.ClientSecret, RefreshToken: token.RefreshToken, Scope: creds.Scope}}
	if err := writeJSON(*outPath, out); err != nil {
		fatal(err)
	}

	fmt.Printf("Wrote refresh token JSON to %s\n", *outPath)
}

func buildAuthorizeURL(base, clientID, redirectURI, scope, state string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := parsed.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("state", state)
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func exchangeToken(tokenURL string, creds credentials, redirectURI, code string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", creds.ClientID)
	if creds.ClientSecret != "" {
		form.Set("client_secret", creds.ClientSecret)
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenResponse{}, err
	}

	if resp.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token tokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return tokenResponse{}, err
	}
	return token, nil
}

func randomState(length int) (string, error) {
	b := make([]byte, length)
	if _, err := cryptoRand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func openBrowser(url string) error {
	cmd := execCommand("open", url)
	if cmd == nil {
		return nil
	}
	return cmd.Run()
}

func execCommand(name string, arg string) *exec.Cmd {
	if _, err := os.Stat("/usr/bin/open"); err == nil {
		return exec.Command(name, arg)
	}
	if _, err := os.Stat("/usr/bin/xdg-open"); err == nil {
		return exec.Command("xdg-open", arg)
	}
	return nil
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadCredentials(path string) (credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return credentials{}, err
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return credentials{}, errors.New("credentials file is empty")
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return parseCredentialsJSON(data)
	}
	return parseCredentialsYAML(data)
}

func parseCredentialsJSON(data []byte) (credentials, error) {
	var file credentialsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return credentials{}, err
	}

	if file.DaikinOnecta.ClientID != "" || file.DaikinOnecta.ClientSecret != "" || file.DaikinOnecta.RefreshToken != "" || file.DaikinOnecta.Scope != "" {
		return file.DaikinOnecta, nil
	}

	return credentials{
		ClientID:     file.ClientID,
		ClientSecret: file.ClientSecret,
		RefreshToken: file.RefreshToken,
		Scope:        file.Scope,
	}, nil
}

func parseCredentialsYAML(data []byte) (credentials, error) {
	lines := strings.Split(string(data), "\n")
	var out credentials

	inSection := false
	sectionIndent := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))
		if strings.HasPrefix(trimmed, "daikin_onecta:") {
			inSection = true
			sectionIndent = indent
			continue
		}

		if inSection && indent <= sectionIndent {
			inSection = false
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if hash := strings.Index(value, "#"); hash >= 0 {
			value = strings.TrimSpace(value[:hash])
		}
		value = strings.Trim(value, "\"'")

		switch key {
		case "client_id":
			if inSection || out.ClientID == "" {
				out.ClientID = value
			}
		case "client_secret":
			if inSection || out.ClientSecret == "" {
				out.ClientSecret = value
			}
		case "refresh_token":
			if inSection || out.RefreshToken == "" {
				out.RefreshToken = value
			}
		case "scope":
			if inSection || out.Scope == "" {
				out.Scope = value
			}
		}
	}

	return out, nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
