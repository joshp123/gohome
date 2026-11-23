package actuation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	actuationv1 "github.com/joshp123/gohome/proto/gen/actuation/v1"
)

func floatPtr(v float64) *float64 { return &v }

func TestPolicyEvaluateDenyByDefaultAndBounds(t *testing.T) {
	policy := &actuationv1.ActuationPolicy{
		AgentKeyring: &actuationv1.AgentKeyringApprovalConfig{
			BinaryPath: "agent-keyring",
			LoginId:    "gohome-high-impact",
		},
		Rules: []*actuationv1.DeviceActionRule{
			{
				PluginId:               "tado",
				Action:                 "set_temperature",
				AllowedTargets:         []string{"Living Room", "Bedroom"},
				Tier:                   actuationv1.ActionTier_ACTION_TIER_BOUNDED_REVERSIBLE,
				RequireApprovalWhenAway: true,
				NumericBounds: &actuationv1.NumericBounds{
					MinValue: floatPtr(16.0),
					MaxValue: floatPtr(24.0),
				},
			},
			{
				PluginId:       "roborock",
				Action:         "clean_all",
				AllowedTargets: []string{"*"},
				Tier:           actuationv1.ActionTier_ACTION_TIER_HIGH_IMPACT_APPROVAL_REQUIRED,
			},
		},
	}

	// 1. Bounded reversible action within bounds while home: allowed without approval.
	d := Evaluate(policy, ActionRequest{
		PluginID:     "tado",
		Action:       "set_temperature",
		Target:       "living_room",
		NumericValue: floatPtr(21.0),
		HouseAway:    false,
	})
	if !d.Allowed || d.RequireApproval {
		t.Fatalf("expected bounded home write allowed without approval, got %+v", d)
	}

	// 2. Out-of-bounds temperature rejected.
	d = Evaluate(policy, ActionRequest{
		PluginID:     "tado",
		Action:       "set_temperature",
		Target:       "living_room",
		NumericValue: floatPtr(30.0),
	})
	if d.Allowed {
		t.Fatalf("expected 30 C temperature to be denied, got %+v", d)
	}

	// 3. Unlisted target denied by default.
	d = Evaluate(policy, ActionRequest{
		PluginID:     "tado",
		Action:       "set_temperature",
		Target:       "bathroom",
		NumericValue: floatPtr(20.0),
	})
	if d.Allowed {
		t.Fatalf("expected unlisted zone to be denied, got %+v", d)
	}

	// 4. Away-from-home escalates bounded reversible action to Agent Keyring approval.
	d = Evaluate(policy, ActionRequest{
		PluginID:     "tado",
		Action:       "set_temperature",
		Target:       "bedroom",
		NumericValue: floatPtr(19.0),
		HouseAway:    true,
	})
	if !d.Allowed || !d.RequireApproval || d.ApprovalLoginID != "gohome-high-impact" {
		t.Fatalf("expected away write to require agent-keyring approval, got %+v", d)
	}

	// 5. High-impact rule always requires approval.
	d = Evaluate(policy, ActionRequest{
		PluginID: "roborock",
		Action:   "clean_all",
		Target:   "all",
	})
	if !d.Allowed || !d.RequireApproval {
		t.Fatalf("expected high-impact action to require approval, got %+v", d)
	}
}

func TestRateLimitAndAuditLog(t *testing.T) {
	dir := t.TempDir()
	ratePath := filepath.Join(dir, "rate.json")
	auditPath := filepath.Join(dir, "audit.jsonl")
	limit := &actuationv1.RateLimit{MaxCalls: 2, WindowSeconds: 600}
	now := time.Date(2026, 11, 23, 3, 0, 0, 0, time.UTC)

	if err := CheckAndRecordRateLimit(ratePath, "tado.set_temperature.living_room", limit, now, false); err != nil {
		t.Fatalf("call 1 failed: %v", err)
	}
	if err := CheckAndRecordRateLimit(ratePath, "tado.set_temperature.living_room", limit, now.Add(time.Minute), false); err != nil {
		t.Fatalf("call 2 failed: %v", err)
	}
	if err := CheckAndRecordRateLimit(ratePath, "tado.set_temperature.living_room", limit, now.Add(2*time.Minute), false); err == nil {
		t.Fatalf("expected call 3 within window to hit rate limit")
	}
	if err := CheckAndRecordRateLimit(ratePath, "tado.set_temperature.living_room", limit, now.Add(11*time.Minute), false); err != nil {
		t.Fatalf("call after window expired should succeed: %v", err)
	}

	if err := AppendAuditEntry(auditPath, AuditEntry{
		Actor:    "agent",
		PluginID: "tado",
		Action:   "set_temperature",
		Target:   "living_room",
		Decision: "allowed",
		Outcome:  "dry_run",
		DryRun:   true,
	}); err != nil {
		t.Fatalf("append audit failed: %v", err)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("expected non-empty audit log, err=%v", err)
	}
}
