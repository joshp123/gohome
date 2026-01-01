package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const SchemaVersion = 1

var ErrStateNotFound = errors.New("oauth state not found")

// State is the persisted OAuth refresh state.
type State struct {
	SchemaVersion int    `json:"schema_version"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RefreshToken  string `json:"refresh_token"`
	Scope         string `json:"scope"`
}

// Bootstrap holds immutable OAuth credentials seeded from Nix.
type Bootstrap struct {
	SchemaVersion int    `json:"schema_version,omitempty"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	Scope         string `json:"scope,omitempty"`
}

func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, ErrStateNotFound
		}
		return State{}, fmt.Errorf("read state: %w", err)
	}
	return DecodeState(data)
}

func LoadBootstrap(path string) (Bootstrap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Bootstrap{}, fmt.Errorf("read bootstrap: %w", err)
	}
	return DecodeBootstrap(data)
}

func DecodeState(data []byte) (State, error) {
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func DecodeBootstrap(data []byte) (Bootstrap, error) {
	var state Bootstrap
	if err := json.Unmarshal(data, &state); err != nil {
		return Bootstrap{}, fmt.Errorf("decode bootstrap: %w", err)
	}
	if err := state.Validate(); err != nil {
		return Bootstrap{}, err
	}
	return state, nil
}

func (s State) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version: %d", s.SchemaVersion)
	}
	if s.ClientID == "" {
		return fmt.Errorf("state missing client_id")
	}
	if s.RefreshToken == "" {
		return fmt.Errorf("state missing refresh_token")
	}
	return nil
}

func (b Bootstrap) Validate() error {
	if b.SchemaVersion != 0 && b.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported bootstrap schema_version: %d", b.SchemaVersion)
	}
	if b.ClientID == "" {
		return fmt.Errorf("bootstrap missing client_id")
	}
	return nil
}

func WriteState(path string, state State) error {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = SchemaVersion
	}
	if err := ensureParent(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func ensureParent(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	return nil
}
