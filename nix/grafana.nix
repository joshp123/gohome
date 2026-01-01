{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gohome;
  grafanaEnvPath = "/run/gohome/grafana.env";
  grafanaEnvScript = pkgs.writeShellScript "gohome-grafana-env" ''
    set -euo pipefail

    suffix="$(${pkgs.tailscale}/bin/tailscale status --json | ${pkgs.jq}/bin/jq -r '.MagicDNSSuffix // empty')"
    if [ -n "$suffix" ]; then
      host="${config.networking.hostName}.$suffix"
    else
      host="${config.networking.hostName}"
    fi

    ${pkgs.coreutils}/bin/mkdir -p /run/gohome
    cat > ${grafanaEnvPath} <<EOF
GF_SERVER_DOMAIN=$host
GF_SERVER_ROOT_URL=http://$host/grafana/
EOF
  '';

in
{
  config = mkIf cfg.enable {
    systemd.services.gohome-grafana-env = mkIf (cfg.grafanaEnvFile == null && cfg.tailscale.enable) {
      description = "Generate Grafana env from Tailscale MagicDNS";
      before = [ "grafana.service" ];
      requiredBy = [ "grafana.service" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = grafanaEnvScript;
      };
    };

    services.grafana = {
      enable = true;
      declarativePlugins = [ pkgs.grafana-image-renderer ];
      settings = {
        rendering = {
          server_url = "http://127.0.0.1:8081/render";
          callback_url = "http://127.0.0.1:3000/grafana/";
        };
        server = {
          http_addr = "127.0.0.1";
          http_port = 3000;
          domain = config.networking.hostName;
          root_url = "%(protocol)s://%(domain)s/grafana/";
          serve_from_sub_path = true;
        };
        auth = {
          disable_login_form = true;
        };
        "auth.anonymous" = {
          enabled = true;
          org_name = "Main Org.";
          org_role = "Admin";
        };
      };

      provision = {
        datasources.settings.datasources = [
          {
            name = "VictoriaMetrics";
            type = "prometheus";
            url = "http://127.0.0.1:8428/vm";
            isDefault = true;
          }
        ];

        dashboards.settings.providers = [
          {
            name = "GoHome";
            options.path = "/var/lib/gohome/dashboards";
            updateIntervalSeconds = 10;
          }
        ];
      };
    };

    services.grafana-image-renderer = {
      enable = true;
    };

    systemd.services.grafana.serviceConfig.EnvironmentFile = mkMerge [
      (mkIf (cfg.grafanaEnvFile != null) [ cfg.grafanaEnvFile ])
      (mkIf (cfg.grafanaEnvFile == null && cfg.tailscale.enable) [ grafanaEnvPath ])
    ];

    systemd.tmpfiles.rules = mkAfter [
      "r /var/lib/grafana/data/grafana.db - - - -"
    ];
  };
}
