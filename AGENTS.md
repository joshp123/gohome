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

## Security Rules

- NEVER commit secrets, tokens, keys, or personal data
- Use placeholder identities (Mr‑Robot style)
- Keep private repo references out of public docs

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
