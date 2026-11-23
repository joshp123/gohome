package actuation

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Approver requests interactive human approval for high-impact device writes.
type Approver interface {
	RequestApproval(ctx context.Context, loginID, taskID, reason string) (leaseID string, release func(), err error)
}

// AgentKeyringApprover uses the installed agent-keyring CLI to request a human
// Touch ID / Face ID approval lease and release it once the write completes.
type AgentKeyringApprover struct {
	BinaryPath string
}

// RequestApproval executes `agent-keyring lease <loginID> --task <taskID> --reason <reason>`
// and returns the granted lease ID plus a release callback.
func (a AgentKeyringApprover) RequestApproval(ctx context.Context, loginID, taskID, reason string) (string, func(), error) {
	if strings.TrimSpace(loginID) == "" {
		return "", func() {}, fmt.Errorf("high-impact action requires an agent-keyring login_id in policy")
	}
	binary := strings.TrimSpace(a.BinaryPath)
	if binary == "" {
		binary = "agent-keyring"
	}

	cmd := exec.CommandContext(ctx, binary, "lease", loginID, "--task", taskID, "--reason", reason)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", func() {}, fmt.Errorf("agent-keyring approval denied or failed: %s (%w)", detail, err)
		}
		return "", func() {}, fmt.Errorf("agent-keyring approval denied or failed: %w", err)
	}

	leaseID := strings.TrimSpace(stdout.String())
	if leaseID == "" {
		return "", func() {}, fmt.Errorf("agent-keyring returned an empty lease id")
	}

	release := func() {
		releaseCmd := exec.Command(binary, "release", leaseID)
		_ = releaseCmd.Run()
	}
	return leaseID, release, nil
}
