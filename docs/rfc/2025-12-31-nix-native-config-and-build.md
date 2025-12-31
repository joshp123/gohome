# RFC: Nix‑Native Configuration and Build Composition

- Date: 2025-12-31
- Status: Implemented
- Audience: operators, infra engineers, plugin authors

## 1) Narrative: what we are building and why

GoHome must be **Nix-native**, not Nix-compatible. Configuration is immutable,
validated at build time, and changes require `nixos-rebuild`. This eliminates drift,
keeps rollbacks reliable, and makes automation safe for AI operators.

This RFC codifies the Nix flake layout, configuration schema, and the build
composition model that compiles plugins into the binary.

## 1.1) Non‑negotiables

- Nix is the only source of runtime configuration
- Secrets come from agenix (never plaintext in repo)
- Plugins are compiled in based on Nix config
- A rebuild is required for any config change

## 2) Goals / Non‑goals

Goals:
- A single flake that provides packages, NixOS modules, and dev shells
- A clear NixOS module schema under `services.gohome.*`
- Deterministic plugin inclusion based on Nix config

Non‑goals:
- Runtime config editing or UI-based configuration
- Support for non-NixOS deployments in MVP
- Dynamic plugin loading from external binaries

## 3) System overview

The flake exports a GoHome package and a NixOS module. The module defines
`services.gohome` options and plugin submodules. Enabling a plugin in the
NixOS configuration composes the GoHome binary with that plugin linked in.

## 4) Components and responsibilities

- `flake.nix`: pins inputs and defines outputs
- `nix/module.nix`: NixOS module for GoHome service
- `nix/package.nix`: Go build derivation
- `plugins/<name>/module.nix`: plugin-specific options and secret wiring
- `hosts/gohome.nix` (later): host config for VPS
- `users.users.gohome`: system user and group for service ownership

## 5) Inputs / workflow profiles

Inputs:
- NixOS config: `services.gohome` options
- Plugin secrets via `age.secrets.*.path`

Example:
```nix
services.gohome = {
  enable = true;
  listenAddress = "0.0.0.0";
  grpcPort = 9000;
  httpPort = 8080;
  plugins.tado = {
    enable = true;
    tokenFile = config.age.secrets.tado-token.path;
  };
};
```

Validation rules:
- Module must fail evaluation if required plugin secrets are missing
- Enabling a plugin must add it to the build composition
- Service must run as a non-root user (`gohome`)

### 5.1) Runtime config materialization

The NixOS module generates a deterministic environment file consumed by systemd,
avoiding any runtime edits:

```nix
systemd.services.gohome.serviceConfig.EnvironmentFile = "/etc/gohome/env";
```

The environment file includes only paths, ports, and flags (no secrets inline).

## 6) Artifacts / outputs

- GoHome binary with compiled-in plugins
- Systemd service unit for GoHome
- Configured secrets files and permissions
- Optional dashboard asset directory on disk

## 7) State machine (if applicable)

Not applicable. Nix evaluation/build is declarative and idempotent.

## 8) API surface (protobuf)

This RFC does not change gRPC APIs. It only defines configuration and build
composition behavior.

## 9) Interaction model

Operators configure GoHome via Nix and apply changes with `nixos-rebuild
switch`. Rollbacks use `nixos-rebuild switch --rollback`.

## 10) System interaction diagram

```
NixOS config
   | nixos-rebuild
   v
GoHome build (plugins compiled in)
   | systemd service
   v
Running GoHome
```

## 11) API call flow (detailed)

Not applicable. No runtime API changes.

## 12) Determinism and validation

- Nix evaluation validates schema and required secrets
- Build composition is explicit and reproducible
- No runtime mutation means rollbacks are safe
- All secret files must be owned by the service user and be non-world-readable

## 13) Outputs and materialization

Primary outputs:
- NixOS module + Go package
- Reproducible builds for aarch64-linux (Hetzner)

## 14) Testing philosophy and trust

- `nix flake check` for evaluation correctness
- NixOS VM build (optional) for service wiring
- Go unit tests remain separate

## 15) Incremental delivery plan

1) Flake skeleton with package + module stubs
2) Add plugin option schema
3) Wire agenix secrets
4) Enable a first plugin build path (Tado)

## 16) Implementation order

1) Create `flake.nix` with inputs and outputs
2) Add `nix/package.nix` for Go build
3) Add `nix/module.nix` with `services.gohome` options
4) Add plugin submodule wiring

## 16.1) Dependencies / sequencing

- Dependencies: core/plugin contract (plugin list + assets are compiled in)
- Blocks: infra deployment; plugin enablement in Nix
- Next: implement flake + NixOS module skeleton

## 17) Brutal self‑review (required)

- Junior engineer: Is the Nix option schema clear with examples? If not, add a
  minimal sample configuration in README.
- Mid‑level engineer: Are we handling secret file ownership and permissions?
  Yes, service user ownership is required and secrets are not world-readable.
- Senior/principal engineer: Does this block future non-Nix deployments? It
  intentionally does for MVP; revisit later if needed.
- PM: Does this keep scope tight? Yes, rebuild-only configuration is simple.
- EM: Is the build pipeline deterministic across platforms? For MVP, target
  aarch64-linux; others can be added later.
- External stakeholder: Does this expose sensitive info? No; secrets stay in
  agenix.
- End user: Will updates be reliable? Nix rollbacks provide safety.
