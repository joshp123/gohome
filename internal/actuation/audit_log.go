package actuation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AuditEntry records one attempted device write in append-only JSONL form.
type AuditEntry struct {
	TimestampUTC    string            `json:"timestamp_utc"`
	Actor           string            `json:"actor"`
	Hostname        string            `json:"hostname"`
	PluginID        string            `json:"plugin_id"`
	Action          string            `json:"action"`
	Target          string            `json:"target"`
	Parameters      map[string]string `json:"parameters,omitempty"`
	HouseAway       bool              `json:"house_away"`
	DryRun          bool              `json:"dry_run"`
	Tier            string            `json:"tier"`
	Decision        string            `json:"decision"`
	ApprovalLeaseID string            `json:"approval_lease_id,omitempty"`
	DenyReason      string            `json:"deny_reason,omitempty"`
	Outcome         string            `json:"outcome"`
	Error           string            `json:"error,omitempty"`
}

// AppendAuditEntry appends one typed JSONL record to path (no-op if path is empty).
func AppendAuditEntry(path string, entry AuditEntry) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if entry.TimestampUTC == "" {
		entry.TimestampUTC = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create audit log dir: %w", err)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode audit entry: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write audit log %q: %w", path, err)
	}
	return nil
}
