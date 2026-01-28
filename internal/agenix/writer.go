package agenix

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Writer persists secrets into a nix-secrets repo via agenix.
type Writer struct {
	RepoPath   string
	RulesPath  string
	SecretName string
	Recipients []string
	Exec       string
	SkipUpdate bool
}

// Write encrypts plaintext into the configured secret file.
func (w Writer) Write(ctx context.Context, plaintext []byte) (string, error) {
	if w.RepoPath == "" {
		return "", fmt.Errorf("agenix repo path is required")
	}
	secretName := w.SecretName
	if secretName == "" {
		return "", fmt.Errorf("agenix secret name is required")
	}
	if !strings.HasSuffix(secretName, ".age") {
		secretName += ".age"
	}

	rules := w.RulesPath
	if rules == "" {
		rules = filepath.Join(w.RepoPath, "secrets.nix")
	}
	secretPath := filepath.Join(w.RepoPath, secretName)

	if !w.SkipUpdate {
		recipients := w.Recipients
		if len(recipients) == 0 {
			var err error
			recipients, err = DefaultRecipients(rules)
			if err != nil {
				return "", err
			}
		}
		if err := EnsureSecretEntry(rules, secretName, recipients); err != nil {
			return "", err
		}
	}

	execName := w.Exec
	if execName == "" {
		execName = "agenix"
	}

	cmd := exec.CommandContext(ctx, execName, "-e", secretPath)
	cmd.Env = append(os.Environ(),
		"RULES="+rules,
		"EDITOR=cp /dev/stdin",
	)
	cmd.Stdin = bytes.NewReader(plaintext)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("agenix: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return secretPath, nil
}

// EnsureSecretEntry adds a secret entry to secrets.nix if missing.
func EnsureSecretEntry(rulesPath, secretName string, recipients []string) error {
	info, err := os.Stat(rulesPath)
	if err != nil {
		return fmt.Errorf("stat secrets.nix: %w", err)
	}
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		return fmt.Errorf("read secrets.nix: %w", err)
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta("\""+secretName+"\"") + `\s*\.publicKeys`)
	if pattern.Match(content) {
		return nil
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients available for %s", secretName)
	}

	entry := fmt.Sprintf("  %q.publicKeys = [ %s ];\n", secretName, strings.Join(recipients, " "))
	idx := strings.LastIndex(string(content), "\n}")
	if idx == -1 {
		return fmt.Errorf("secrets.nix missing closing brace")
	}
	updated := string(content[:idx]) + "\n" + entry + string(content[idx:])
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}
	return os.WriteFile(rulesPath, []byte(updated), mode)
}

// DefaultRecipients finds a gohome recipient set in secrets.nix.
func DefaultRecipients(rulesPath string) ([]string, error) {
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("read secrets.nix: %w", err)
	}
	re := regexp.MustCompile(`"gohome-[^"]+\.age"\s*\.publicKeys\s*=\s*\[([^\]]+)\]`)
	match := re.FindStringSubmatch(string(content))
	if len(match) < 2 {
		return nil, fmt.Errorf("no gohome recipients found in secrets.nix")
	}
	fields := strings.Fields(match[1])
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty recipient list in secrets.nix")
	}
	return fields, nil
}
