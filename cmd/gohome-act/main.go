package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joshp123/gohome/internal/actuation"
	"github.com/joshp123/gohome/internal/render"
	actuationv1 "github.com/joshp123/gohome/proto/gen/actuation/v1"
	daikinv1 "github.com/joshp123/gohome/proto/gen/plugins/daikin/v1"
	roborockv1 "github.com/joshp123/gohome/proto/gen/plugins/roborock/v1"
	tadov1 "github.com/joshp123/gohome/proto/gen/plugins/tado/v1"
	registryv1 "github.com/joshp123/gohome/proto/gen/registry/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultActuationPolicyPath = "/etc/gohome/actuation-policy.pbtxt"

type actionSummary struct {
	Plugin          string            `json:"plugin"`
	Action          string            `json:"action"`
	Target          string            `json:"target"`
	Parameters      map[string]string `json:"parameters,omitempty"`
	Tier            string            `json:"tier"`
	Allowed         bool              `json:"allowed"`
	RequireApproval bool              `json:"require_approval"`
	ApprovalLoginID string            `json:"approval_login_id,omitempty"`
	ApprovalLeaseID string            `json:"approval_lease_id,omitempty"`
	DryRun          bool              `json:"dry_run"`
	HouseAway       bool              `json:"house_away"`
	Outcome         string            `json:"outcome"`
	DenyReason      string            `json:"deny_reason,omitempty"`
}

type globalOptions struct {
	policyPath string
	auditPath  string
	addrsRaw   string
	reason     string
	away       bool
	dryRun     bool
	jsonOutput bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, rest, helpRequested, err := parseGlobalFlags(args)
	format := render.FormatText
	if opts.jsonOutput {
		format = render.FormatJSON
	}
	out := render.NewRenderer(os.Stdout, os.Stderr, format)

	if err != nil {
		out.WriteError("gohome-act", err, render.ExitUsageError)
		return render.ExitUsageError
	}
	if helpRequested || len(rest) == 0 {
		writeRootHelp(out)
		if helpRequested {
			return render.ExitOK
		}
		return render.ExitUsageError
	}

	policyPath := resolvePolicyPath(opts.policyPath)
	policy, err := actuation.LoadPolicy(policyPath)
	if err != nil {
		out.WriteError("load policy", err, render.ExitPolicyDenied)
		return render.ExitPolicyDenied
	}

	switch rest[0] {
	case "policy":
		return handlePolicy(out, policy, opts, rest[1:])
	case "tado":
		return handleTado(out, policy, opts, rest[1:])
	case "daikin":
		return handleDaikin(out, policy, opts, rest[1:])
	case "roborock":
		return handleRoborock(out, policy, opts, rest[1:])
	default:
		out.WriteError("gohome-act", fmt.Errorf("unknown command %q", rest[0]), render.ExitUsageError)
		writeRootHelp(out)
		return render.ExitUsageError
	}
}

func parseGlobalFlags(args []string) (globalOptions, []string, bool, error) {
	fs := flag.NewFlagSet("gohome-act", flag.ContinueOnError)
	var opts globalOptions
	fs.StringVar(&opts.policyPath, "policy", "", "Path to ActuationPolicy textproto file (env: GOHOME_ACTUATION_POLICY)")
	fs.StringVar(&opts.auditPath, "audit-log", "", "Override JSONL audit log path")
	fs.StringVar(&opts.addrsRaw, "addrs", "", "Comma-separated GoHome gRPC endpoints (env: GOHOME_GRPC_ADDRS / GOHOME_GRPC_ADDR)")
	fs.StringVar(&opts.reason, "reason", "", "Human reason recorded in audit log and Agent Keyring approval prompt")
	fs.BoolVar(&opts.away, "away", false, "Treat house occupancy as away (escalates away-protected rules to Agent Keyring approval)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Evaluate policy, bounds, rate limits, and approval gate without calling devices")
	fs.BoolVar(&opts.jsonOutput, "json", false, "Emit structured JSON output")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, nil, true, nil
		}
		return opts, nil, false, err
	}
	return opts, fs.Args(), false, nil
}

func handlePolicy(out render.Renderer, policy *actuationv1.ActuationPolicy, opts globalOptions, args []string) int {
	if len(args) < 4 || args[0] != "check" {
		out.WriteHelp("gohome-act policy check <plugin> <action> <target> [numeric_value]", nil)
		return render.ExitUsageError
	}
	req := actuation.ActionRequest{
		PluginID:  args[1],
		Action:    args[2],
		Target:    args[3],
		HouseAway: opts.away,
	}
	if len(args) >= 5 {
		v, err := strconv.ParseFloat(args[4], 64)
		if err != nil {
			out.WriteError("policy check", fmt.Errorf("invalid numeric value %q", args[4]), render.ExitUsageError)
			return render.ExitUsageError
		}
		req.NumericValue = &v
	}

	decision := actuation.Evaluate(policy, req)
	summary := actionSummary{
		Plugin:          req.PluginID,
		Action:          req.Action,
		Target:          req.Target,
		Tier:            formatTier(decision.EffectiveTier),
		Allowed:         decision.Allowed,
		RequireApproval: decision.RequireApproval,
		ApprovalLoginID: decision.ApprovalLoginID,
		DryRun:          true,
		HouseAway:       opts.away,
		Outcome:         "policy_check",
		DenyReason:      decision.DenyReason,
	}
	emitSummary(out, summary)
	if !decision.Allowed {
		return render.ExitPolicyDenied
	}
	return render.ExitOK
}

func handleTado(out render.Renderer, policy *actuationv1.ActuationPolicy, opts globalOptions, args []string) int {
	if len(args) < 3 || args[0] != "set" {
		out.WriteHelp("gohome-act tado set <zone> <temperature_celsius>", nil)
		return render.ExitUsageError
	}
	zoneName := args[1]
	temp, err := strconv.ParseFloat(args[2], 64)
	if err != nil {
		out.WriteError("tado set", fmt.Errorf("invalid temperature %q", args[2]), render.ExitUsageError)
		return render.ExitUsageError
	}
	req := actuation.ActionRequest{
		PluginID:     "tado",
		Action:       "set_temperature",
		Target:       zoneName,
		NumericValue: &temp,
		HouseAway:    opts.away,
		Parameters:   map[string]string{"temperature_celsius": fmt.Sprintf("%.1f", temp)},
	}
	return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
		client := tadov1.NewTadoServiceClient(conn)
		zones, err := client.ListZones(ctx, &tadov1.ListZonesRequest{})
		if err != nil {
			return fmt.Errorf("list tado zones: %w", err)
		}
		zoneMap := make(map[string]string, len(zones.Zones))
		for _, z := range zones.Zones {
			zoneMap[z.Name] = z.Id
		}
		zoneID, err := resolveNamedID("zone", zoneName, zoneMap)
		if err != nil {
			return err
		}
		_, err = client.SetTemperature(ctx, &tadov1.SetTemperatureRequest{
			ZoneId:             zoneID,
			TemperatureCelsius: temp,
		})
		return err
	})
}

func handleDaikin(out render.Renderer, policy *actuationv1.ActuationPolicy, opts globalOptions, args []string) int {
	if len(args) == 0 {
		out.WriteHelp("gohome-act daikin <power|temp> ...", nil)
		return render.ExitUsageError
	}
	switch args[0] {
	case "power":
		if len(args) < 3 {
			out.WriteHelp("gohome-act daikin power <unit> <on|off>", nil)
			return render.ExitUsageError
		}
		unitName := args[1]
		mode := strings.ToLower(strings.TrimSpace(args[2]))
		if mode != "on" && mode != "off" {
			out.WriteError("daikin power", fmt.Errorf("mode must be on or off, got %q", args[2]), render.ExitUsageError)
			return render.ExitUsageError
		}
		req := actuation.ActionRequest{
			PluginID:   "daikin",
			Action:     "set_power",
			Target:     unitName,
			HouseAway:  opts.away,
			Parameters: map[string]string{"on_off_mode": mode},
		}
		return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
			client := daikinv1.NewDaikinServiceClient(conn)
			unit, err := resolveDaikinUnit(ctx, client, unitName)
			if err != nil {
				return err
			}
			_, err = client.SetOnOff(ctx, &daikinv1.SetOnOffRequest{
				UnitId:           unit.Id,
				ClimateControlId: unit.ClimateControlId,
				OnOffMode:        mode,
			})
			return err
		})
	case "temp":
		if len(args) < 4 {
			out.WriteHelp("gohome-act daikin temp <unit> <heating|cooling|auto> <temperature_celsius>", nil)
			return render.ExitUsageError
		}
		unitName := args[1]
		opMode := strings.ToLower(strings.TrimSpace(args[2]))
		temp, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			out.WriteError("daikin temp", fmt.Errorf("invalid temperature %q", args[3]), render.ExitUsageError)
			return render.ExitUsageError
		}
		req := actuation.ActionRequest{
			PluginID:     "daikin",
			Action:       "set_temperature",
			Target:       unitName,
			NumericValue: &temp,
			HouseAway:    opts.away,
			Parameters: map[string]string{
				"operation_mode":      opMode,
				"temperature_celsius": fmt.Sprintf("%.1f", temp),
			},
		}
		return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
			client := daikinv1.NewDaikinServiceClient(conn)
			unit, err := resolveDaikinUnit(ctx, client, unitName)
			if err != nil {
				return err
			}
			_, err = client.SetTemperature(ctx, &daikinv1.SetTemperatureRequest{
				UnitId:             unit.Id,
				ClimateControlId:   unit.ClimateControlId,
				OperationMode:      opMode,
				Setpoint:           "roomTemperature",
				TemperatureCelsius: temp,
			})
			return err
		})
	default:
		out.WriteHelp("gohome-act daikin <power|temp> ...", nil)
		return render.ExitUsageError
	}
}

func handleRoborock(out render.Renderer, policy *actuationv1.ActuationPolicy, opts globalOptions, args []string) int {
	if len(args) == 0 {
		out.WriteHelp("gohome-act roborock <clean|dock|locate> ...", nil)
		return render.ExitUsageError
	}
	switch args[0] {
	case "clean":
		if len(args) < 2 {
			out.WriteHelp("gohome-act roborock clean <room|--all>", nil)
			return render.ExitUsageError
		}
		if args[1] == "--all" {
			req := actuation.ActionRequest{
				PluginID:  "roborock",
				Action:    "clean_all",
				Target:    "all",
				HouseAway: opts.away,
			}
			return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
				client := roborockv1.NewRoborockServiceClient(conn)
				_, err := client.StartClean(ctx, &roborockv1.StartCleanRequest{})
				return err
			})
		}
		room := args[1]
		req := actuation.ActionRequest{
			PluginID:  "roborock",
			Action:    "clean_room",
			Target:    room,
			HouseAway: opts.away,
		}
		return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
			client := roborockv1.NewRoborockServiceClient(conn)
			_, err := client.CleanRoom(ctx, &roborockv1.CleanRoomRequest{
				Room:    room,
				Repeats: 1,
			})
			return err
		})
	case "dock":
		req := actuation.ActionRequest{
			PluginID:  "roborock",
			Action:    "dock",
			Target:    "default",
			HouseAway: opts.away,
		}
		return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
			client := roborockv1.NewRoborockServiceClient(conn)
			_, err := client.Dock(ctx, &roborockv1.DockRequest{})
			return err
		})
	case "locate":
		req := actuation.ActionRequest{
			PluginID:  "roborock",
			Action:    "locate",
			Target:    "default",
			HouseAway: opts.away,
		}
		return executeGatedAction(out, policy, opts, req, func(ctx context.Context, conn *grpc.ClientConn) error {
			client := roborockv1.NewRoborockServiceClient(conn)
			_, err := client.Locate(ctx, &roborockv1.LocateRequest{})
			return err
		})
	default:
		out.WriteHelp("gohome-act roborock <clean|dock|locate> ...", nil)
		return render.ExitUsageError
	}
}

func executeGatedAction(
	out render.Renderer,
	policy *actuationv1.ActuationPolicy,
	opts globalOptions,
	req actuation.ActionRequest,
	execFn func(ctx context.Context, conn *grpc.ClientConn) error,
) int {
	decision := actuation.Evaluate(policy, req)
	auditPath := opts.auditPath
	if auditPath == "" {
		auditPath = policy.GetAuditLogPath()
	}

	entry := actuation.AuditEntry{
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		Actor:        currentActor(),
		Hostname:     currentHostname(),
		PluginID:     req.PluginID,
		Action:       req.Action,
		Target:       req.Target,
		Parameters:   req.Parameters,
		HouseAway:    req.HouseAway,
		DryRun:       opts.dryRun,
		Tier:         formatTier(decision.EffectiveTier),
	}

	if !decision.Allowed {
		entry.Decision = "denied"
		entry.DenyReason = decision.DenyReason
		entry.Outcome = "policy_denied"
		_ = actuation.AppendAuditEntry(auditPath, entry)
		out.WriteError("policy", fmt.Errorf("%s", decision.DenyReason), render.ExitPolicyDenied)
		return render.ExitPolicyDenied
	}

	ruleKey := strings.ToLower(fmt.Sprintf("%s.%s.%s", req.PluginID, req.Action, req.Target))
	if err := actuation.CheckAndRecordRateLimit(
		policy.GetRateLimitStatePath(),
		ruleKey,
		decision.MatchedRule.GetRateLimit(),
		time.Now().UTC(),
		opts.dryRun,
	); err != nil {
		entry.Decision = "denied"
		entry.DenyReason = err.Error()
		entry.Outcome = "rate_limited"
		_ = actuation.AppendAuditEntry(auditPath, entry)
		out.WriteError("rate limit", err, render.ExitPolicyDenied)
		return render.ExitPolicyDenied
	}

	entry.Decision = "allowed"
	if opts.dryRun {
		entry.Outcome = "dry_run_ok"
		if decision.RequireApproval {
			entry.Outcome = "dry_run_requires_agent_keyring_approval"
		}
		_ = actuation.AppendAuditEntry(auditPath, entry)
		emitSummary(out, actionSummary{
			Plugin:          req.PluginID,
			Action:          req.Action,
			Target:          req.Target,
			Parameters:      req.Parameters,
			Tier:            formatTier(decision.EffectiveTier),
			Allowed:         true,
			RequireApproval: decision.RequireApproval,
			ApprovalLoginID: decision.ApprovalLoginID,
			DryRun:          true,
			HouseAway:       req.HouseAway,
			Outcome:         entry.Outcome,
		})
		return render.ExitOK
	}

	if decision.RequireApproval {
		binPath := "agent-keyring"
		if policy.GetAgentKeyring() != nil && policy.GetAgentKeyring().GetBinaryPath() != "" {
			binPath = policy.GetAgentKeyring().GetBinaryPath()
		}
		approver := actuation.AgentKeyringApprover{BinaryPath: binPath}
		reason := strings.TrimSpace(opts.reason)
		if reason == "" {
			reason = fmt.Sprintf("gohome-act %s %s %s", req.PluginID, req.Action, req.Target)
		}
		taskID := fmt.Sprintf("gohome-act-%s-%s-%d", req.PluginID, req.Action, time.Now().Unix())
		approvalCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		leaseID, release, err := approver.RequestApproval(approvalCtx, decision.ApprovalLoginID, taskID, reason)
		cancel()
		if err != nil {
			entry.Outcome = "approval_denied"
			entry.Error = err.Error()
			_ = actuation.AppendAuditEntry(auditPath, entry)
			out.WriteError("approval", err, render.ExitApprovalDenied)
			return render.ExitApprovalDenied
		}
		defer release()
		entry.ApprovalLeaseID = leaseID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := dialPluginNode(ctx, resolveAddrs(opts.addrsRaw), req.PluginID)
	if err != nil {
		entry.Outcome = "dial_failed"
		entry.Error = err.Error()
		_ = actuation.AppendAuditEntry(auditPath, entry)
		out.WriteError("dial", err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}
	defer func() { _ = conn.Close() }()

	if err := execFn(ctx, conn); err != nil {
		entry.Outcome = "rpc_failed"
		entry.Error = err.Error()
		_ = actuation.AppendAuditEntry(auditPath, entry)
		out.WriteError(req.PluginID+" "+req.Action, err, render.ExitRuntimeError)
		return render.ExitRuntimeError
	}

	entry.Outcome = "executed"
	_ = actuation.AppendAuditEntry(auditPath, entry)
	emitSummary(out, actionSummary{
		Plugin:          req.PluginID,
		Action:          req.Action,
		Target:          req.Target,
		Parameters:      req.Parameters,
		Tier:            formatTier(decision.EffectiveTier),
		Allowed:         true,
		RequireApproval: decision.RequireApproval,
		ApprovalLoginID: decision.ApprovalLoginID,
		ApprovalLeaseID: entry.ApprovalLeaseID,
		DryRun:          false,
		HouseAway:       req.HouseAway,
		Outcome:         "executed",
	})
	return render.ExitOK
}

func emitSummary(out render.Renderer, s actionSummary) {
	if out.Format == render.FormatJSON {
		_ = out.WriteJSON(s)
		return
	}
	pairs := []render.KeyValue{
		{Key: "plugin", Value: s.Plugin},
		{Key: "action", Value: s.Action},
		{Key: "target", Value: s.Target},
		{Key: "tier", Value: s.Tier},
		{Key: "allowed", Value: strconv.FormatBool(s.Allowed)},
		{Key: "require_approval", Value: strconv.FormatBool(s.RequireApproval)},
		{Key: "outcome", Value: s.Outcome},
	}
	if s.ApprovalLoginID != "" {
		pairs = append(pairs, render.KeyValue{Key: "approval_login_id", Value: s.ApprovalLoginID})
	}
	if s.ApprovalLeaseID != "" {
		pairs = append(pairs, render.KeyValue{Key: "approval_lease_id", Value: s.ApprovalLeaseID})
	}
	if s.DenyReason != "" {
		pairs = append(pairs, render.KeyValue{Key: "deny_reason", Value: s.DenyReason})
	}
	_ = out.WriteKeyValues(pairs)
}

func dialPluginNode(ctx context.Context, addrs []string, pluginID string) (*grpc.ClientConn, error) {
	var lastErr error
	for _, addr := range addrs {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			lastErr = err
			continue
		}
		reg := registryv1.NewRegistryClient(conn)
		resp, err := reg.ListPlugins(ctx, &registryv1.ListPluginsRequest{})
		if err != nil {
			_ = conn.Close()
			lastErr = fmt.Errorf("%s: %w", addr, err)
			continue
		}
		for _, p := range resp.GetPlugins() {
			if strings.EqualFold(p.GetPluginId(), pluginID) {
				return conn, nil
			}
		}
		_ = conn.Close()
	}
	if lastErr != nil {
		return nil, fmt.Errorf("plugin %q not found on %v (last error: %w)", pluginID, addrs, lastErr)
	}
	return nil, fmt.Errorf("plugin %q not hosted on any configured node (%v)", pluginID, addrs)
}

func resolvePolicyPath(flagPath string) string {
	if strings.TrimSpace(flagPath) != "" {
		return flagPath
	}
	if env := strings.TrimSpace(os.Getenv("GOHOME_ACTUATION_POLICY")); env != "" {
		return env
	}
	return defaultActuationPolicyPath
}

func resolveAddrs(flagAddrs string) []string {
	raw := strings.TrimSpace(flagAddrs)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOHOME_GRPC_ADDRS"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOHOME_GRPC_ADDR"))
	}
	if raw == "" {
		return []string{"gohome:9000"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"gohome:9000"}
	}
	return out
}

func resolveDaikinUnit(ctx context.Context, client daikinv1.DaikinServiceClient, input string) (*daikinv1.Unit, error) {
	resp, err := client.ListUnits(ctx, &daikinv1.ListUnitsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list daikin units: %w", err)
	}
	if len(resp.Units) == 0 {
		return nil, fmt.Errorf("no daikin units available")
	}
	if strings.EqualFold(input, "default") && len(resp.Units) == 1 {
		return resp.Units[0], nil
	}
	options := make(map[string]string, len(resp.Units))
	byID := make(map[string]*daikinv1.Unit, len(resp.Units))
	for _, u := range resp.Units {
		options[u.Name] = u.Id
		byID[u.Id] = u
		if strings.EqualFold(u.Id, input) {
			return u, nil
		}
	}
	id, err := resolveNamedID("daikin unit", input, options)
	if err != nil {
		return nil, err
	}
	return byID[id], nil
}

func resolveNamedID(kind, input string, options map[string]string) (string, error) {
	needle := normalizeName(input)
	for label, id := range options {
		if normalizeName(label) == needle {
			return id, nil
		}
	}
	available := make([]string, 0, len(options))
	for label := range options {
		available = append(available, label)
	}
	sort.Strings(available)
	return "", fmt.Errorf("%s %q not found (available: %s)", kind, input, strings.Join(available, ", "))
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func formatTier(t actuationv1.ActionTier) string {
	switch t {
	case actuationv1.ActionTier_ACTION_TIER_BOUNDED_REVERSIBLE:
		return "bounded_reversible"
	case actuationv1.ActionTier_ACTION_TIER_HIGH_IMPACT_APPROVAL_REQUIRED:
		return "high_impact_approval_required"
	case actuationv1.ActionTier_ACTION_TIER_DENIED:
		return "denied"
	case actuationv1.ActionTier_ACTION_TIER_UNSPECIFIED:
		return "unspecified"
	default:
		return "unspecified"
	}
}

func currentActor() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

func currentHostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

func writeRootHelp(out render.Renderer) {
	out.WriteHelp("gohome-act [flags] <command> [args]", []render.HelpSection{
		{
			Heading: "Commands",
			Lines: []string{
				"policy check <plugin> <action> <target> [value]",
				"tado set <zone> <temperature_celsius>",
				"daikin power <unit> <on|off>",
				"daikin temp <unit> <heating|cooling|auto> <temperature_celsius>",
				"roborock clean <room|--all>",
				"roborock dock",
				"roborock locate",
			},
		},
		{
			Heading: "Flags",
			Lines: []string{
				"--policy <path>     ActuationPolicy textproto (env: GOHOME_ACTUATION_POLICY)",
				"--addrs <a,b>       Comma-separated GoHome gRPC nodes (env: GOHOME_GRPC_ADDRS)",
				"--audit-log <path>  Override append-only JSONL audit log path",
				"--away              Escalate away-protected actions to Agent Keyring approval",
				"--dry-run           Check policy, bounds, rate limit, and approval gate only",
				"--reason <text>     Reason shown in Agent Keyring prompt and audit log",
				"--json              Structured JSON output",
			},
		},
	})
}
