package oauthflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joshp123/gohome/internal/oauth"
)

// PersistResult reports persistence outcomes.
type PersistResult struct {
	StatePath   string
	TempPath    string
	BlobSaved   bool
	AgenixSaved bool
	AgenixPath  string
}

// PersistOptions controls persistence behavior.
type PersistOptions struct {
	StatePathOverride string
	SkipBlob          bool
}

// DefaultTempPath returns a deterministic temp path for OAuth state.
func DefaultTempPath(provider string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("gohome-oauth-%s-%s.json", provider, timestamp)
	return filepath.Join(os.TempDir(), name)
}

// WriteTempState writes OAuth state to a temp file.
func WriteTempState(path string, state oauth.State) (string, error) {
	if path == "" {
		return "", fmt.Errorf("state path required")
	}
	if err := oauth.WriteState(path, state); err != nil {
		return "", err
	}
	return path, nil
}

// LoadState loads OAuth state from a file.
func LoadState(path string) (oauth.State, error) {
	return oauth.LoadState(path)
}

// PersistState writes state to disk and optionally to blob storage.
func PersistState(ctx context.Context, decl oauth.Declaration, state oauth.State, blob oauth.BlobStore, opts PersistOptions) (PersistResult, error) {
	statePath := decl.StatePath
	if opts.StatePathOverride != "" {
		statePath = opts.StatePathOverride
	}
	if statePath == "" {
		return PersistResult{}, fmt.Errorf("state path missing")
	}
	if err := oauth.WriteState(statePath, state); err != nil {
		return PersistResult{}, err
	}

	result := PersistResult{StatePath: statePath}
	if opts.SkipBlob || blob == nil {
		return result, nil
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return result, err
	}
	if err := blob.Save(ctx, decl.Provider, payload); err != nil {
		return result, err
	}
	result.BlobSaved = true
	return result, nil
}
