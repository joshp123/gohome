{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gohome;
  grafanaEnvPath = "/run/gohome/grafana.env";
  grafanaEnvScript = pkgs.writeShellScript "gohome-grafana-env" ''
    set -euo pipefail

    # Grafana is reached only over the tailnet, at its MagicDNS name, where Tailscale serves HTTPS (nginx.nix).
    # After boot tailscaled takes a moment to sign in; wait for it rather than give Grafana a wrong address.
    # Grafana is unreachable without Tailscale anyway, so it waits too, however long, and starts once Tailscale is up.
    waited=0
    while true; do
      suffix="$(${config.services.tailscale.package}/bin/tailscale status --json 2>/dev/null \
        | ${pkgs.jq}/bin/jq -r 'select(.BackendState == "Running") | .MagicDNSSuffix // empty' || true)"
      [ -n "$suffix" ] && break
      if [ $((waited % 60)) -eq 59 ]; then
        echo "Waiting for Tailscale to run before starting Grafana ($((waited + 1)) s so far)" >&2
      fi
      sleep 1
      waited=$((waited + 1))
    done

    ${pkgs.coreutils}/bin/mkdir -p /run/gohome
    cat > ${grafanaEnvPath} <<EOF
GF_SERVER_DOMAIN=${config.networking.hostName}.$suffix
GF_SERVER_ROOT_URL=https://${config.networking.hostName}.$suffix/grafana/
EOF
  '';

  # The dashboards of the plugins another gohome runs, from this build's copy of each plugin.
  # A remote plugin with no plugins/<name>/dashboard.json in this build gets no dashboard.
  remoteDashboardFile = plugin: ../plugins + "/${plugin}/dashboard.json";
  remoteDashboards = pkgs.runCommand "gohome-remote-dashboards" { } (''
    mkdir -p $out/Devices
  '' + concatMapStrings (plugin: ''
    cp ${remoteDashboardFile plugin} $out/Devices/${plugin}-overview.json
  '') (filter (plugin: builtins.pathExists (remoteDashboardFile plugin)) (attrNames cfg.remotePlugins)));

in
{
  config = mkIf (cfg.enable && cfg.monitoring.enable) {
    systemd.services.gohome-grafana-env = mkIf (cfg.grafanaEnvFile == null && cfg.tailscale.enable) {
      description = "Generate Grafana env from Tailscale MagicDNS";
      after = [ "tailscaled.service" ];
      wants = [ "tailscaled.service" ];
      before = [ "grafana.service" ];
      requiredBy = [ "grafana.service" ];
      serviceConfig = {
        Type = "oneshot";
        # Stays active, so a deploy that changes the script runs it again.
        RemainAfterExit = true;
        ExecStart = grafanaEnvScript;
      };
    };
    # ...and restarts Grafana to read the new address.
    systemd.services.grafana.restartTriggers = mkIf (cfg.grafanaEnvFile == null && cfg.tailscale.enable) [ grafanaEnvScript ];

    services.grafana = {
      enable = true;
      declarativePlugins = [ pkgs.grafana-image-renderer ];
      settings = {
        rendering = {
          server_url = "http://127.0.0.1:8081/render";
          callback_url = "http://127.0.0.1/grafana/";
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
        dashboards.default_home_dashboard_path = "/var/lib/gohome/dashboards/House/home-home.json";
        date_formats = {
          full_date = "D MMM YYYY HH:mm:ss";
          interval_second = "HH:mm:ss";
          interval_minute = "HH:mm";
          interval_hour = "D MMM HH:mm";
          interval_day = "D MMM";
          interval_month = "MMM YYYY";
          interval_year = "YYYY";
          default_timezone = "Europe/Amsterdam";
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
            options.foldersFromFilesStructure = true;
            updateIntervalSeconds = 10;
          }
        ] ++ optional (cfg.remotePlugins != { }) {
          name = "GoHome remote plugins";
          options.path = remoteDashboards;
          options.foldersFromFilesStructure = true;
        };
      };
    };

    services.grafana-image-renderer = {
      enable = true;
    };

    # gohome rewrites the dashboard files when it starts; provisioning them mid-rewrite leaves Grafana with none.
    systemd.services.grafana.after = [ "gohome.service" ];

    systemd.services.grafana.serviceConfig.EnvironmentFile = mkMerge [
      (mkIf (cfg.grafanaEnvFile != null) [ cfg.grafanaEnvFile ])
      (mkIf (cfg.grafanaEnvFile == null && cfg.tailscale.enable) [ grafanaEnvPath ])
    ];

    systemd.tmpfiles.rules = mkAfter [
      "r /var/lib/grafana/data/grafana.db - - - -"
    ];
  };
}
