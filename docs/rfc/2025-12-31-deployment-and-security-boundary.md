# RFC: Deployment + Security Boundary (Hetzner + Tailscale Only)

- Date: 2025-12-31
- Status: Implemented
- Audience: operators, infra engineers, security reviewers

## 1) Narrative: what we are building and why

GoHome should run on a low-cost Hetzner VPS with NixOS, accessible **only over
Tailscale**. This minimizes attack surface and avoids building a custom auth
layer for MVP. The deployment flow is reproducible and rollbackable with Nix.

This RFC locks the deployment boundary and the MVP security posture.

## 1.1) Non‑negotiables

- No public ports for GoHome, VictoriaMetrics, or Grafana
- Tailscale is the only access path
- Secrets live in agenix, never in repo
- Rollbacks use `nixos-rebuild --rollback`

## 2) Goals / Non‑goals

Goals:
- Deterministic VPS provisioning with OpenTofu
- NixOS deployment via `nixos-infect` bootstrap
- Tailscale-only access for SSH and APIs

Non‑goals:
- App-level auth tokens or mTLS in MVP
- Multi-user auth and access control
- Public HTTP endpoints

## 3) System overview

OpenTofu provisions a Hetzner VPS. The initial image is Ubuntu for bootstrap,
then `nixos-infect` installs NixOS. The GoHome flake is deployed to the VPS and
Tailscale joins the host to the tailnet. All services bind to localhost or
Tailscale interfaces only.

## 4) Components and responsibilities

- OpenTofu (infra/tofu): VPS provisioning
- NixOS config: GoHome + VM + Grafana + Tailscale
- Tailscale: private network + SSH
- Agenix: secret distribution

## 5) Inputs / workflow profiles

Inputs:
- Hetzner API token (agenix or environment)
- Tailscale auth key (agenix)
- SSH public key for bootstrap access

Validation rules:
- No public firewall openings outside Tailscale interface
- Tailscale must be enabled before GoHome API is used

### 5.1) Default ports and bindings (MVP)

Defaults (override via Nix if needed):
- GoHome gRPC: 9000
- GoHome HTTP (health/metrics/dashboards): 8080
- Grafana: 3000
- VictoriaMetrics: 8428

Services bind to `0.0.0.0` but firewall allows access **only** on `tailscale0`.
There are no public ports.

## 6) Artifacts / outputs

- Hetzner VPS instance (ARM)
- NixOS host configured with GoHome services
- Tailscale-only access path

## 7) State machine (if applicable)

Not applicable. Deployment is declarative; rollback is a Nix operation.

## 8) API surface (protobuf)

No API changes; this RFC defines infrastructure boundaries.

## 9) Interaction model

Operators:
1) `tofu apply` to create VPS
2) Wait for nixos-infect
3) `nixos-rebuild switch --flake ...#gohome`
4) Access via Tailscale only

## 10) System interaction diagram

```
Operator
  | tofu apply
  v
Hetzner VPS (NixOS)
  | tailscale up
  v
Tailnet
  | gRPC/SSH
  v
GoHome + Grafana + VM
```

## 11) API call flow (detailed)

Not applicable. This RFC defines access boundaries, not runtime calls.

## 12) Determinism and validation

- Infrastructure is declarative via OpenTofu
- NixOS build ensures service configuration is reproducible
- Tailscale ACLs are the security boundary in MVP
 - Firewall is configured with `trustedInterfaces = [ "tailscale0" ]`

## 13) Outputs and materialization

Primary outputs:
- Reproducible VPS and service deployment
- Tailscale-only access

## 14) Testing philosophy and trust

- Smoke test: gRPC health check over Tailscale
- Rollback test: `nixos-rebuild --rollback` when health fails

## 15) Incremental delivery plan

1) OpenTofu plan and apply
2) NixOS bootstrap with tailscale enabled
3) Deploy GoHome services

## 16) Implementation order

1) Write infra/tofu with Hetzner provider
2) Add NixOS host config with Tailscale + firewall
3) Deploy GoHome flake to VPS

## 16.1) Dependencies / sequencing

- Dependencies: Nix-native config/build (flake + module)
- Blocks: production deployment and ops validation
- Next: implement infra/tofu + host module

## 17) Brutal self‑review (required)

- Junior engineer: Is the deployment flow documented and reproducible? Add
  a short README if unclear.
- Mid‑level engineer: Are we relying too much on Tailscale without fallback?
  For MVP this is intentional; note the risk.
- Senior/principal engineer: Does this block future public access? No, but any
  exposure should be an explicit later RFC.
- PM: Does this keep scope tight and secure? Yes.
- EM: Is ops burden manageable? A single VPS + Nix rollback keeps it simple.
- External stakeholder: Is the security posture defensible? Tailscale-only
  boundary reduces surface area.
- End user: Will this feel stable? Rollbacks provide safety.
