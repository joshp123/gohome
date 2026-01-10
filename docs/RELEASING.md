# Releasing / Deploying GoHome

GoHome is deployed directly to the `gohome` host via Nix; there is no separate release artifact.

## Preconditions

- Local changes are committed and pushed to `main`.
- Secrets are available on the host via agenix (`/root/nix-secrets`).
- Tailscale access to `gohome` is working.

## Deploy

```bash
scripts/deploy.sh gohome
```

Notes:
- The script SSHes to `root@100.108.102.95` (Tailscale) and pulls `main`.
- It runs `nixos-rebuild switch` and rolls back on failure.
- Health check: `curl http://localhost:8080/health`.

## Verify

```bash
# Health
curl -sSf http://gohome/health

# Metrics (example)
curl -sS http://gohome/metrics | grep gohome_
```

Grafana (MagicDNS):
- `http://gohome/grafana/`
- Dashboards are provisioned from `/var/lib/gohome/dashboards`.

## Rollback

Automatic rollback is triggered by the deploy script if rebuild or health check fails.

## Enable/Disable Plugins

- Edit `nix/hosts/gohome.nix` to add or remove `services.gohome.plugins.<name>`.
- Re-deploy with `scripts/deploy.sh gohome`.
