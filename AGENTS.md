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

Notes, facts and analyses about a specific house live in private repos, never here.

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

## Deploying

This repo is the product only and holds no host. gohome is a NixOS module:
a host imports `nixosModules.default`, sets `services.gohome`, and is deployed
from that host's own configuration, which lives outside this repo.

- Health check on a host: `curl http://localhost:8080/health`.
- `gohome` and `gohome-cli` are on the host's PATH. If not, find them with
  `systemctl cat gohome | grep -n ExecStart`.
- Bootstrap secrets (for example the Roborock bootstrap JSON) are encrypted
  with agenix in the host's secrets repo and passed to the module as file
  paths, such as `services.gohome.plugins.roborock.bootstrapFile`.
