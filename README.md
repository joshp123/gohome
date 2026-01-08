# GoHome

> Home automation for people who hate home automation software.
>
> <sub>[skip to agent copypasta](#give-this-to-your-ai-agent)</sub>

![GoHome Grafana Dashboard](demo-grafana.png)

## The Magic

- **No UI, ever.** Clawdbot is your interface. Talk to your house in Telegram. Your AI agent figures out the rest.

- **Plugins self-declare everything.** Each plugin ships an `AGENTS.md` and proto definitions. Your agent discovers capabilities at compile time, not through trial and error.

- **It's just a dumb API.** We expose gRPC endpoints and a CLI. Your SOTA agent (Claude, GPT, whatever) figures out how to use them. We don't try to be smart.

- **Plugins bootstrap themselves.** Metrics shape, Grafana dashboards, OAuth flows - all declared by the plugin. Enable it in Nix, it just works.

- **Fully declarative.** Once you have credentials wired up, you shouldn't have to care. `nixos-rebuild switch` to update. `nixos-rebuild --rollback` if something breaks. Your AI agent manages the rest.

## Why I built this

I tried to set up Home Assistant. It broke me.

First it was YAML config. Fine, I can live with YAML. Then they banned YAML and made you click through a UI instead. Then they got into a fight with the NixOS community and told everyone maintaining declarative configs to fuck off. So now if you want to configure HA declaratively, you're on your own patching the whole thing.

You can't wire up secrets properly. You can't set admin credentials without running a dozen manual commands. The Prometheus integration spits out metrics with unhinged names unless you patch it. And the UI - the one they force you to use because "YAML is scary" - has graphs that look like they were designed in 2003 by someone who hates you.

I wanted to control my heating, not mass-assign my weekends to "wontfix".

Instead they gave us three ways to configure everything (YAML, UI, automations), deprecated two of them, and made the third require a mass-market tablet bolted to a wall. In 2026 I can talk to an AI that controls my heating via Telegram, but Home Assistant still can't figure out how to load a config file.

GoHome is what happens when you give up on fixing Home Assistant and just write something that works.

## The Stack

1. **[Clawdbot](https://docs.clawd.bot/)** - AI agent gateway for Telegram/WhatsApp/etc. If you're not letting Claude control your house via chat in 2026, you're NGMI. This is how you talk to GoHome without touching a UI.

2. **A cheap VM** - I use [Hetzner](https://hetzner.com). It's cheaper than AWS, VMs are simple, no 47 services to configure. A €4/month box runs everything.

3. **S3-compatible blob storage** - For OAuth refresh tokens to survive restarts. Hetzner Object Storage, Backblaze B2, or AWS S3 all work.

4. **[Tailscale](https://tailscale.com)** (recommended) - Zero-config VPN so your bot can reach your home server without exposing ports. Just works.

5. **[agenix](https://github.com/ryantm/agenix)** - Encrypted secrets wired into Nix. Your API keys, OAuth tokens, and plugin credentials live here, not in plaintext YAML like savages.

6. **NixOS** (recommended) - I use [Determinate Nix](https://determinate.systems/posts/determinate-nix-installer) for the installer, and started from [dustinlyons/nixos-config](https://github.com/dustinlyons/nixos-config) templates. Give your coding agent the template repo and let it set you up.

### What we manage vs what you manage

| Component | Who manages it | How |
| --- | --- | --- |
| VM provisioning | We do | OpenTofu in `infra/` - just run `tofu apply` |
| NixOS config | We do | Flake modules, `nixos-rebuild switch` |
| Grafana + VictoriaMetrics | We do | Bundled in the NixOS module |
| S3 bucket creation | We do | OpenTofu handles it |
| GoHome service | We do | systemd unit, auto-configured |
| **Tailscale auth** | You do | One-time `tailscale up` on your server |
| **Tailscale ACLs** | You do | If you want to restrict access |
| **Your device credentials** | You do | Plugin bootstrap tokens via agenix |

Basically: clone, configure secrets, `tofu apply && nixos-rebuild switch`, done. Tailscale is the only manual step.

## The API (gRPC + Proto)

Everything is gRPC with Protocol Buffers. Each plugin declares its own proto definitions in `plugins/<name>/proto/`.

Why gRPC:
- Type-safe API contracts (no guessing JSON shapes)
- `grpcurl` for CLI access (like curl but for gRPC)
- Easy service discovery via reflection
- Agents can introspect available methods at runtime

Example proto (Tado):
```protobuf
service TadoService {
  rpc ListZones(ListZonesRequest) returns (ListZonesResponse);
  rpc SetTemperature(SetTemperatureRequest) returns (SetTemperatureResponse);
}

message SetTemperatureRequest {
  string zone_id = 1;
  double temperature_celsius = 2;
}
```

Your AI agent can discover what's available:
```bash
grpcurl -plaintext localhost:9000 describe
```

## What it actually does

```
Me: "set heating to 19"
Bot: "DONE! Living room is now set to a BEAUTIFUL 19°C."

Me: "home status"
Bot: 
  🏠 HOME STATUS
  Living Room   21.3°C  (set: 19°C)  🔥 ON
  Bedroom       20.7°C               ⚪ OFF
  Bathroom      20.3°C               ⚪ OFF
```

That's it. I talk to my Telegram bot, it controls my heating. No app, no cloud dependency, no 47-step YAML ritual.

## Give this to your AI agent

Copy this entire block and paste it to Claude, Cursor, or whatever agent you use:

```text
I want to set up GoHome on my NixOS server for home automation.

Repository: github:joshp123/gohome

What GoHome is:
- A Nix-native home automation server (Go, not Python)
- Control via gRPC API + CLI
- Metrics in VictoriaMetrics, dashboards in Grafana
- Currently supports Tado + Roborock (more plugins coming)

What I need you to do:
1. Add the gohome flake input to my NixOS config
2. Enable services.gohome with my plugin config
3. Set up secrets via agenix (OAuth blob storage + plugin bootstrap tokens)
4. Deploy with nixos-rebuild switch
5. Verify: Grafana loads, /metrics returns plugin data (Tado/Roborock/etc)

My setup:
- [FILL IN: your NixOS host details]
- [FILL IN: which plugins you want - tado, daikin, etc]
- [FILL IN: your S3-compatible blob storage for OAuth state]

Reference the README and nix/module.nix in the repo for config options.
```

## Why we're better than Home Assistant

| Aspect | Home Assistant | GoHome |
| --- | --- | --- |
| Language | Python (slow, async spaghetti) | Go (fast, boring) |
| Config | YAML (runtime, mutable, cursed) | Nix (declarative, immutable) |
| Storage | SQLite (wrong tool) | VictoriaMetrics (right tool) |
| UI | Lovelace (maintain it yourself) | Grafana (already good) |
| Control | HA app, web UI | Telegram bot, CLI, grpcurl |
| Plugins | HACS, pip, Docker, prayers | Nix flakes |
| Secrets | YAML plaintext (lol) | agenix / sops |
| Rollback | Hope you have a backup | `nixos-rebuild --rollback` |
| Updates | Pray nothing breaks | Nix pins everything |
| RAM | 1-2GB typical | ~256MB |
| Deploy | Docker, HAOS, Supervised, tears | `nixos-rebuild switch` |

## How it works

1. Single Go binary exposes gRPC + HTTP (metrics/health)
2. Plugins compiled in at build time (no runtime loading)
3. Each plugin brings: proto definitions, Prometheus metrics, Grafana dashboards
4. OAuth tokens persisted locally + mirrored to S3 for disaster recovery
5. Config is pure Nix - no YAML, no env vars, no runtime mutations

## Minimal setup

```nix
# flake.nix
{
  inputs.gohome.url = "github:joshp123/gohome";

  outputs = { self, nixpkgs, gohome }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      modules = [
        gohome.nixosModules.default
        {
          services.gohome = {
            enable = true;
            oauth = {
              blobEndpoint = "https://s3.eu-central-1.amazonaws.com";
              blobBucket = "my-gohome-oauth";
              blobAccessKeyFile = config.age.secrets.gohome-blob-access.path;
              blobSecretKeyFile = config.age.secrets.gohome-blob-secret.path;
            };
            plugins.tado = {
              enable = true;
              bootstrapFile = config.age.secrets.tado-token.path;
            };
            plugins.roborock = {
              enable = true;
              bootstrapFile = config.age.secrets.roborock-bootstrap.path;
              cloudFallback = false;
            };
          };
        }
      ];
    };
  };
}
```

## Required secrets (agenix)

| Secret | Purpose |
| --- | --- |
| `gohome-blob-access` | S3 access key for OAuth state |
| `gohome-blob-secret` | S3 secret key for OAuth state |
| `tado-token` | Initial Tado OAuth refresh token |
| `roborock-bootstrap` | Roborock bootstrap JSON (email login + local keys) |

## Plugin philosophy

- **Plugins own everything**: proto, metrics, dashboards, AGENTS.md
- **Code-first**: behavior in Go, Nix only wires config/secrets
- **No runtime mutability**: state is mutable, config is not
- **Observability built-in**: metrics and dashboards are part of the contract

## Creating a plugin (WIP)

This is still being refined, but the workflow is basically: let your coding agent steal from Home Assistant.

### Give this to your agent

```text
I want to create a GoHome plugin for [DEVICE/SERVICE NAME].

Repos to clone:
- github:joshp123/gohome (this repo - reference existing plugins in plugins/)
- github:home-assistant/core (steal their integration logic from homeassistant/components/[name]/)

What I need you to do:
1. Find the Home Assistant integration for [DEVICE/SERVICE]
2. Understand how it authenticates (OAuth, API key, local polling, etc)
3. Create a new plugin in plugins/[name]/ following the existing pattern:
   - Proto definitions for the gRPC API
   - OAuth wiring if needed (use the oauth provider in pkg/oauth)
   - Prometheus metrics (what state should we expose?)
   - Grafana dashboard JSON
   - AGENTS.md explaining how an AI agent should use this plugin
4. Wire it into the Nix module (nix/module.nix)
5. Test it works: metrics show up, gRPC calls succeed

Reference plugins/tado/ for the structure. The AGENTS.md is important - it tells 
Clawdbot (or any AI agent) how to talk to your plugin.
```

### What a plugin contains

```
plugins/yourdevice/
├── proto/              # gRPC service definitions
├── client.go           # Device/API client logic (steal from HA)
├── plugin.go           # Plugin registration, metrics, dashboards
├── AGENTS.md           # How an AI agent should use this plugin
└── dashboards/         # Grafana JSON
```

I've had good results with Codex + GPT-5.2 (high reasoning) for this. It can read the HA Python code and translate to Go surprisingly well.

## CLI examples

```bash
# List zones
grpcurl -plaintext localhost:9000 gohome.plugins.tado.v1.TadoService/ListZones

# Set temperature
grpcurl -plaintext -d '{"zone_id":"1","temperature_celsius":21}' \
  localhost:9000 gohome.plugins.tado.v1.TadoService/SetTemperature

# Check metrics
curl -s localhost:8080/metrics | grep gohome_tado
```

## Development

```bash
nix develop
# or: devenv shell

# Generate protobufs
./tools/generate.sh

# Run server (enable desired plugins via build tags)
go run -tags gohome_plugin_tado,gohome_plugin_roborock ./cmd/gohome

# List plugins
go run ./cmd/gohome-cli plugins list

# Friendly CLI (agent-friendly)
go run ./cmd/gohome-cli roborock status
go run ./cmd/gohome-cli roborock clean kitchen
go run ./cmd/gohome-cli roborock mop kitchen
go run ./cmd/gohome-cli roborock vacuum kitchen
go run ./cmd/gohome-cli tado zones
go run ./cmd/gohome-cli tado set living-room 20
```

## Status

Pre-alpha. Built the MVP in 2 days over NYE (yes, including the party and phone calls). Currently running my own heating.

## Docs

- [PHILOSOPHY.md](PHILOSOPHY.md) - why Nix, why Go, why not HA
- [docs/rfc/INDEX.md](docs/rfc/INDEX.md) - technical decisions

## Thanks

This project stands on the shoulders of people who figured out agent-first development before it was cool:

- **Steve Yegge** ([@Steve_Yegge](https://x.com/Steve_Yegge)) - [Zero-Framework Cognition](https://steve-yegge.medium.com/zero-framework-cognition-a-way-to-build-resilient-ai-applications-56b090ed3e69) changed how I think about AI interfaces. Stop building frameworks, start exposing dumb APIs.
- **Peter Steinberger** ([@steipete](https://twitter.com/steipete)) - [Clawdbot](https://docs.clawd.bot/) is the agent gateway that makes this all work. Also read [Shipping at Inference Speed](https://steipete.me/posts/2025/shipping-at-inference-speed).
- **Mario Zechner** ([@badlogicgames](https://x.com/badlogicgames)) - [Pi](https://shittycodingagent.ai) is the "shitty coding agent" that powers Clawdbot. Turns out shitty is pretty good.
- The Home Assistant integration authors whose protocol research, API discovery, and integration logic we have reimplemented in GoHome plugins.

## License

AGPL-3.0
