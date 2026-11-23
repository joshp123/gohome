package actuation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	actuationv1 "github.com/joshp123/gohome/proto/gen/actuation/v1"
	"google.golang.org/protobuf/encoding/prototext"
)

// ActionRequest describes one proposed device write before execution.
type ActionRequest struct {
	PluginID     string
	Action       string
	Target       string
	NumericValue *float64
	HouseAway    bool
	Parameters   map[string]string
}

// PolicyDecision is the deterministic outcome of evaluating an ActionRequest
// against a Nix-declared ActuationPolicy.
type PolicyDecision struct {
	Allowed         bool
	RequireApproval bool
	EffectiveTier   actuationv1.ActionTier
	ApprovalLoginID string
	DenyReason      string
	MatchedRule     *actuationv1.DeviceActionRule
}

// LoadPolicy loads an ActuationPolicy textproto file from disk.
func LoadPolicy(path string) (*actuationv1.ActuationPolicy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read actuation policy %q: %w", path, err)
	}
	policy := &actuationv1.ActuationPolicy{}
	if err := prototext.Unmarshal(raw, policy); err != nil {
		return nil, fmt.Errorf("parse actuation policy %q: %w", path, err)
	}
	return policy, nil
}

// Evaluate applies deny-by-default rules, target allowlists, hard numeric
// bounds, and away-from-home escalation.
func Evaluate(policy *actuationv1.ActuationPolicy, req ActionRequest) PolicyDecision {
	if policy == nil {
		return PolicyDecision{
			Allowed:       false,
			EffectiveTier: actuationv1.ActionTier_ACTION_TIER_DENIED,
			DenyReason:    "no actuation policy loaded (writes are denied by default)",
		}
	}

	var matched *actuationv1.DeviceActionRule
	for _, rule := range policy.GetRules() {
		if !strings.EqualFold(rule.GetPluginId(), req.PluginID) {
			continue
		}
		if !strings.EqualFold(rule.GetAction(), req.Action) {
			continue
		}
		if !targetAllowed(rule.GetAllowedTargets(), req.Target) {
			continue
		}
		matched = rule
		break
	}

	if matched == nil {
		return PolicyDecision{
			Allowed:       false,
			EffectiveTier: actuationv1.ActionTier_ACTION_TIER_DENIED,
			DenyReason:    fmt.Sprintf("no policy rule permits %s.%s on target %q", req.PluginID, req.Action, req.Target),
		}
	}

	tier := matched.GetTier()
	if tier == actuationv1.ActionTier_ACTION_TIER_UNSPECIFIED || tier == actuationv1.ActionTier_ACTION_TIER_DENIED {
		return PolicyDecision{
			Allowed:       false,
			EffectiveTier: actuationv1.ActionTier_ACTION_TIER_DENIED,
			MatchedRule:   matched,
			DenyReason:    fmt.Sprintf("policy explicitly denies %s.%s on target %q", req.PluginID, req.Action, req.Target),
		}
	}

	if bounds := matched.GetNumericBounds(); bounds != nil {
		if req.NumericValue == nil {
			return PolicyDecision{
				Allowed:       false,
				EffectiveTier: actuationv1.ActionTier_ACTION_TIER_DENIED,
				MatchedRule:   matched,
				DenyReason:    fmt.Sprintf("%s.%s requires a numeric value within policy bounds", req.PluginID, req.Action),
			}
		}
		val := *req.NumericValue
		if bounds.MinValue != nil && val < bounds.GetMinValue() {
			return PolicyDecision{
				Allowed:       false,
				EffectiveTier: actuationv1.ActionTier_ACTION_TIER_DENIED,
				MatchedRule:   matched,
				DenyReason:    fmt.Sprintf("value %.2f is below policy minimum %.2f for %s.%s", val, bounds.GetMinValue(), req.PluginID, req.Action),
			}
		}
		if bounds.MaxValue != nil && val > bounds.GetMaxValue() {
			return PolicyDecision{
				Allowed:       false,
				EffectiveTier: actuationv1.ActionTier_ACTION_TIER_DENIED,
				MatchedRule:   matched,
				DenyReason:    fmt.Sprintf("value %.2f exceeds policy maximum %.2f for %s.%s", val, bounds.GetMaxValue(), req.PluginID, req.Action),
			}
		}
	}

	effectiveTier := tier
	if tier == actuationv1.ActionTier_ACTION_TIER_BOUNDED_REVERSIBLE && req.HouseAway && matched.GetRequireApprovalWhenAway() {
		effectiveTier = actuationv1.ActionTier_ACTION_TIER_HIGH_IMPACT_APPROVAL_REQUIRED
	}

	approvalLogin := strings.TrimSpace(matched.GetApprovalLoginId())
	if approvalLogin == "" && policy.GetAgentKeyring() != nil {
		approvalLogin = strings.TrimSpace(policy.GetAgentKeyring().GetLoginId())
	}

	return PolicyDecision{
		Allowed:         true,
		RequireApproval: effectiveTier == actuationv1.ActionTier_ACTION_TIER_HIGH_IMPACT_APPROVAL_REQUIRED,
		EffectiveTier:   effectiveTier,
		ApprovalLoginID: approvalLogin,
		MatchedRule:     matched,
	}
}

func targetAllowed(allowed []string, target string) bool {
	if len(allowed) == 0 {
		return false
	}
	needle := normalizeTarget(target)
	for _, entry := range allowed {
		if strings.TrimSpace(entry) == "*" {
			return true
		}
		if normalizeTarget(entry) == needle {
			return true
		}
	}
	return false
}

func normalizeTarget(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

type rateLimitState struct {
	Buckets map[string][]int64 `json:"buckets"`
}

// CheckAndRecordRateLimit enforces a rolling-window call cap stored in a local JSON file.
func CheckAndRecordRateLimit(statePath, ruleKey string, limit *actuationv1.RateLimit, now time.Time, dryRun bool) error {
	if limit == nil || limit.GetMaxCalls() == 0 || limit.GetWindowSeconds() == 0 {
		return nil
	}
	if strings.TrimSpace(statePath) == "" {
		return nil
	}

	state := rateLimitState{Buckets: make(map[string][]int64)}
	if raw, err := os.ReadFile(statePath); err == nil && len(raw) > 0 {
		if unmarshalErr := json.Unmarshal(raw, &state); unmarshalErr != nil {
			return fmt.Errorf("parse rate-limit state %q: %w", statePath, unmarshalErr)
		}
		if state.Buckets == nil {
			state.Buckets = make(map[string][]int64)
		}
	}

	cutoff := now.Add(-time.Duration(limit.GetWindowSeconds()) * time.Second).Unix()
	existing := state.Buckets[ruleKey]
	active := make([]int64, 0, len(existing)+1)
	for _, ts := range existing {
		if ts >= cutoff {
			active = append(active, ts)
		}
	}

	if uint32(len(active)) >= limit.GetMaxCalls() {
		return fmt.Errorf("rate limit exceeded for %s: %d calls in last %ds (max %d)",
			ruleKey, len(active), limit.GetWindowSeconds(), limit.GetMaxCalls())
	}

	if dryRun {
		return nil
	}

	active = append(active, now.Unix())
	state.Buckets[ruleKey] = active

	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return fmt.Errorf("create rate-limit state dir: %w", err)
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, encoded, 0o600)
}
