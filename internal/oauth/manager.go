package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/oauth2"
)

var ErrScopeMismatch = errors.New("oauth scope mismatch")

// Manager manages OAuth refresh tokens and access token caching.
type Manager struct {
	decl          Declaration
	bootstrapPath string
	blobStore     BlobStore
	httpClient    *http.Client

	mu              sync.Mutex
	accessToken     string
	expiresAt       time.Time
	refreshToken    string
	scope           string
	clientID        string
	clientSecret    string
	refreshInFlight bool
	config          *oauth2.Config
}

func NewManager(decl Declaration, bootstrapPath string, blobStore BlobStore) (*Manager, error) {
	if bootstrapPath == "" {
		return nil, fmt.Errorf("bootstrap path is required")
	}
	bootstrap, err := LoadBootstrap(bootstrapPath)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	return NewManagerFromBootstrap(decl, bootstrap, blobStore)
}

// NewManagerFromBootstrap creates an OAuth manager from an inline Bootstrap (no file needed).
func NewManagerFromBootstrap(decl Declaration, bootstrap Bootstrap, blobStore BlobStore) (*Manager, error) {
	if decl.Provider == "" {
		return nil, fmt.Errorf("provider is required")
	}
	if decl.Scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	if decl.TokenURL == "" {
		return nil, fmt.Errorf("tokenURL is required")
	}
	if decl.StatePath == "" {
		return nil, fmt.Errorf("statePath is required")
	}
	if !filepath.IsAbs(decl.StatePath) {
		return nil, fmt.Errorf("statePath must be absolute")
	}
	if blobStore == nil {
		return nil, fmt.Errorf("blob store is required")
	}
	if err := bootstrap.Validate(); err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}

	m := &Manager{
		decl:          decl,
		blobStore:     blobStore,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		clientID:      bootstrap.ClientID,
		clientSecret:  bootstrap.ClientSecret,
		config: &oauth2.Config{
			ClientID:     bootstrap.ClientID,
			ClientSecret: bootstrap.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  decl.AuthorizeURL,
				TokenURL: decl.TokenURL,
			},
			Scopes: strings.Fields(decl.Scope),
		},
	}

	state, err := m.loadInitialState(bootstrap)
	if err != nil {
		return nil, err
	}

	m.refreshToken = state.RefreshToken
	m.scope = state.Scope

	return m, nil
}

func (m *Manager) Start(ctx context.Context) {
	m.StartWithInterval(ctx, DefaultRefreshInterval)
}

func (m *Manager) StartWithInterval(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	threshold := interval
	if threshold < 30*time.Second {
		threshold = 30 * time.Second
	}
	m.refreshIfNeeded(ctx, threshold)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.refreshIfNeeded(ctx, threshold)
			}
		}
	}()
}

func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.accessToken != "" && time.Until(m.expiresAt) > 30*time.Second {
		return m.accessToken, nil
	}

	tokenValid.WithLabelValues(m.decl.Provider).Set(0)
	return "", fmt.Errorf("oauth token unavailable")
}

func (m *Manager) TriggerRefresh(ctx context.Context) {
	m.mu.Lock()
	if m.refreshInFlight {
		m.mu.Unlock()
		return
	}
	m.refreshInFlight = true
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			m.refreshInFlight = false
			m.mu.Unlock()
		}()
		_ = m.refresh(ctx)
	}()
}

func (m *Manager) refreshIfNeeded(ctx context.Context, threshold time.Duration) {
	m.mu.Lock()
	need := m.accessToken == "" || time.Until(m.expiresAt) <= threshold
	if !need || m.refreshInFlight {
		m.mu.Unlock()
		return
	}
	m.refreshInFlight = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.refreshInFlight = false
		m.mu.Unlock()
	}()

	_ = m.refresh(ctx)
}

func (m *Manager) refresh(ctx context.Context) error {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, m.httpClient)
	source := m.config.TokenSource(ctx, &oauth2.Token{RefreshToken: m.refreshToken})
	token, err := source.Token()
	if err != nil {
		refreshFailure.WithLabelValues(m.decl.Provider).Inc()
		tokenValid.WithLabelValues(m.decl.Provider).Set(0)
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			body := strings.TrimSpace(string(retrieveErr.Body))
			return fmt.Errorf("token refresh failed %d: %s", retrieveErr.Response.StatusCode, body)
		}
		return err
	}

	m.mu.Lock()
	m.accessToken = token.AccessToken
	m.expiresAt = token.Expiry
	if token.RefreshToken != "" {
		m.refreshToken = token.RefreshToken
	}
	m.mu.Unlock()

	state := State{
		SchemaVersion: SchemaVersion,
		ClientID:      m.clientID,
		ClientSecret:  m.clientSecret,
		RefreshToken:  m.refreshToken,
		Scope:         m.scope,
	}

	if err := WriteState(m.decl.StatePath, state); err != nil {
		refreshFailure.WithLabelValues(m.decl.Provider).Inc()
		return fmt.Errorf("persist state: %w", err)
	}
	if err := m.persistBlob(ctx, state); err != nil {
		remotePersistOK.WithLabelValues(m.decl.Provider).Set(0)
		refreshSuccess.WithLabelValues(m.decl.Provider).Inc()
		tokenValid.WithLabelValues(m.decl.Provider).Set(1)
		return nil
	}

	remotePersistOK.WithLabelValues(m.decl.Provider).Set(1)
	refreshSuccess.WithLabelValues(m.decl.Provider).Inc()
	tokenValid.WithLabelValues(m.decl.Provider).Set(1)
	return nil
}

func (m *Manager) loadInitialState(bootstrap Bootstrap) (State, error) {
	local, localErr := LoadState(m.decl.StatePath)
	if localErr == nil {
		if err := checkStateFile(m.decl.StatePath); err != nil {
			return State{}, err
		}
		if local.Scope != "" && local.Scope != m.decl.Scope {
			scopeMismatch.WithLabelValues(m.decl.Provider).Inc()
			return State{}, ErrScopeMismatch
		}
		if local.Scope == "" {
			local.Scope = m.decl.Scope
		}
		local.ClientID = bootstrap.ClientID
		local.ClientSecret = bootstrap.ClientSecret
		if err := m.persistBlob(context.Background(), local); err != nil {
			remotePersistOK.WithLabelValues(m.decl.Provider).Set(0)
		} else {
			remotePersistOK.WithLabelValues(m.decl.Provider).Set(1)
		}
		return local, nil
	}

	blob, blobErr := m.loadFromBlob(context.Background())
	if blobErr == nil {
		blob.ClientID = bootstrap.ClientID
		blob.ClientSecret = bootstrap.ClientSecret
		if blob.Scope == "" {
			blob.Scope = m.decl.Scope
		}
		if blob.Scope != "" && blob.Scope != m.decl.Scope {
			scopeMismatch.WithLabelValues(m.decl.Provider).Inc()
			return State{}, ErrScopeMismatch
		}
		if err := WriteState(m.decl.StatePath, blob); err != nil {
			return State{}, err
		}
		if err := m.persistBlob(context.Background(), blob); err != nil {
			remotePersistOK.WithLabelValues(m.decl.Provider).Set(0)
		} else {
			remotePersistOK.WithLabelValues(m.decl.Provider).Set(1)
		}
		return blob, nil
	}

	if blobErr != nil && !errors.Is(blobErr, ErrBlobNotFound) {
		if !errors.Is(localErr, ErrStateNotFound) {
			return State{}, localErr
		}
		return State{}, blobErr
	}

	if bootstrap.RefreshToken == "" {
		return State{}, fmt.Errorf("bootstrap missing refresh_token; run oauth runner")
	}

	state := State{
		SchemaVersion: SchemaVersion,
		ClientID:      bootstrap.ClientID,
		ClientSecret:  bootstrap.ClientSecret,
		RefreshToken:  bootstrap.RefreshToken,
		Scope:         bootstrap.Scope,
	}
	if state.Scope == "" {
		state.Scope = m.decl.Scope
	}
	if state.Scope != "" && state.Scope != m.decl.Scope {
		scopeMismatch.WithLabelValues(m.decl.Provider).Inc()
		return State{}, ErrScopeMismatch
	}

	if err := WriteState(m.decl.StatePath, state); err != nil {
		return State{}, err
	}
	if err := m.persistBlob(context.Background(), state); err != nil {
		remotePersistOK.WithLabelValues(m.decl.Provider).Set(0)
	} else {
		remotePersistOK.WithLabelValues(m.decl.Provider).Set(1)
	}

	return state, nil
}

func (m *Manager) loadFromBlob(ctx context.Context) (State, error) {
	data, err := m.blobStore.Load(ctx, m.decl.Provider)
	if err != nil {
		return State{}, err
	}
	return DecodeState(data)
}

func (m *Manager) persistBlob(ctx context.Context, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return m.blobStore.Save(ctx, m.decl.Provider, data)
}

func checkStateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("state file %s must have 0600 permissions", path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != os.Geteuid() {
			return fmt.Errorf("state file %s must be owned by uid %d", path, os.Geteuid())
		}
	}
	return nil
}
