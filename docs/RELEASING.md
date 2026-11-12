# Releasing / Deploying GoHome

GoHome ships as a Nix flake: a package and a NixOS module. There is no
separate release artifact and no host in this repo.

## Deploy

In your host's NixOS configuration, add this flake as an input, import
`inputs.gohome.nixosModules.default`, set `services.gohome` (plugins, their
addresses and secret files), pin the input to the commit you want and deploy
that host as usual, for example with `nixos-rebuild switch --flake`.

## Verify

```bash
curl -sSf http://localhost:8080/health
curl -sS http://localhost:8080/metrics | grep _scrape_success
```

Grafana is served under `/grafana/`; dashboards are provisioned from
`/var/lib/gohome/dashboards`.

## Rollback

`nixos-rebuild switch --rollback` on the host, or deploy the previous pin.

## Enable/Disable Plugins

Add or remove `services.gohome.plugins.<name>` in the host's configuration and
deploy it.
