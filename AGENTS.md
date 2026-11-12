# GoHome Agent Guide

## Project Intent

Nix‑native home automation. Replace Home Assistant with a deterministic Go system.

## North Star

- If it compiles, it works
- Nix is source of truth
- ZFC: AI reasons, code executes
- Test in prod, rollback via Nix
- Sane, opinionated defaults; batteries included

## Non‑negotiables

- No runtime config editing
- Protobuf/gRPC is the product
- Plugins own proto, metrics, dashboards, and AGENTS.md
- No UI beyond Grafana
- Secrets via agenix only

## In-flight (living log)




## Data repairs

- Readings deleted from VictoriaMetrics cannot be restored. Save the removed readings first, in a private backup outside this repo.

## Public repo

gohome is public on GitHub. Nothing about the owner's house or household goes in it: not in
files, commit messages or branches, including branches you think are temporary. That
means room and device facts, measurements and analyses of this house, addresses and
network details, installer paperwork, and when people are home. Write code and docs that
would be true of any house, with placeholder identities (Mr-Robot style). Never commit
secrets, tokens or keys.


## Commit Policy

- One logical change per commit
- Format: `🤖 codex: short description (issue-id)`
- No `git add .` / `git add -A`
- Run full test suite before committing (if none, state “not run”)
- Push directly to `main`

## RFC / ADR Policy

- RFCs in `docs/rfc/` using `RFC_TEMPLATE.md`
- ADRs in `docs/adr/` (short, dated)
- Update only when decisions change

## Docs to Read First

- `PHILOSOPHY.md`
- `docs/rfc/INDEX.md`

## Dev Tooling

Use `devenv` to provision local build tools (Go + protoc plugins).

```
devenv shell
bash tools/generate.sh
```

## nix-openclaw Plugin

GoHome exposes an idiomatic nix-openclaw plugin via `openclawPlugin` in `flake.nix`.

- Plugin name: `gohome`
- Skills: `skills/gohome`
- Packages: `gohome` (includes `gohome-cli` on PATH)
- Required env: none (optional `GOHOME_GRPC_ADDR` supported by `gohome-cli`)

## Smoke Test (remote)

Default host: `gohome` (matches `flake.nix` nixosConfigurations). Always deploy over the private network, not the public IP.

Command:
`scripts/deploy.sh gohome`

Notes:
- Runs `sudo nixos-rebuild switch` on the host and rolls back on failure.
- Uses `curl http://localhost:8080/health` as the health check.
- SSH access must be configured for the target host. The default NixOS host config (`nix/hosts/gohome.nix`) only sets root SSH keys, so use `root@gohome` or configure your local SSH config to map `gohome` to the correct user/host.
- The deploy script maps `gohome` to `$GOHOME_DEPLOY_HOST`, the host's ssh target, which is kept in private config.
- The current host uses `/root/gohome` (not `/etc/nixos`) and a secrets checkout. The deploy script detects this and overrides the secrets input accordingly.
- GoHome binaries should be on PATH on the host (via `environment.systemPackages`).
  - If PATH is missing, use systemd to locate them:
    - `ssh root@<host> 'systemctl cat gohome | grep -n ExecStart'`
    - Run the CLI via the ExecStart path, e.g. `ssh root@<host> '<execstart> roborock probe --methods ...'`.

## Bootstrap + Self-Bootstrapping

- The host is expected to have a working checkout of this repo.
  - `scripts/deploy.sh` runs `git pull` only if the checkout has a `.git` directory.
  - If the host has a tarball or bare tree, the deploy script clones the repo to a separate directory and uses that for builds.
- Bootstrap secrets are stored in the secrets repo and materialized by agenix on the host.
  - Roborock bootstrap JSON is generated locally, encrypted into `gohome-roborock-bootstrap.age`,
    and surfaced to GoHome via `services.gohome.plugins.roborock.bootstrapFile`.
- The deploy script should be the only path to update the remote host; it rebuilds and rolls back on failure.
