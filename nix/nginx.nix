{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

in
{
  config = mkIf (cfg.enable && cfg.monitoring.enable) (mkMerge [
    {
      services.nginx = {
        enable = true;
        recommendedProxySettings = true;

        virtualHosts."gohome" = {
          listen = [ { addr = "0.0.0.0"; port = 80; } ];
          default = true;
          # Tailscale's HTTPS proxy connects from 127.0.0.1 and passes the client's tailnet address on.
          extraConfig = ''
            set_real_ip_from 127.0.0.1;
            real_ip_header X-Forwarded-For;
          '';
          # The house is read in Grafana; its home dashboard is the House overview.
          locations."= /".return = "302 /grafana/";
          locations."/grafana/".proxyPass = "http://127.0.0.1:3000";
          locations."/vm/".proxyPass = "http://127.0.0.1:8428";
          locations."/gohome/".proxyPass = "http://127.0.0.1:${toString cfg.httpPort}/";
          locations."= /gohome/hikvision/snapshot.jpg" = {
            proxyPass = "http://${cfg.remotePlugins.hikvision or "127.0.0.1:${toString cfg.httpPort}"}/hikvision/snapshot.jpg";
            extraConfig = ''
              proxy_buffering off;
              proxy_cache off;
              allow 100.64.0.0/10;
              allow fd7a:115c:a1e0::/48;
              deny all;
            '';
          };
        };
      };

      networking.firewall = {
        enable = true;
        trustedInterfaces = [ "tailscale0" ];
        allowedTCPPorts = [ ];
      };
    }

    # HTTPS on the MagicDNS name, with a certificate Tailscale issues and renews.
    # Phone apps such as Grafium refuse plain HTTP.
    (mkIf cfg.tailscale.enable {
      systemd.services.gohome-tailscale-https = {
        description = "Serve nginx over HTTPS on the tailnet";
        after = [ "tailscaled.service" "nginx.service" ];
        wants = [ "tailscaled.service" ];
        wantedBy = [ "multi-user.target" ];
        serviceConfig = {
          Type = "oneshot";
          RemainAfterExit = true;
          # tailscaled takes a few seconds after boot to sign in.
          Restart = "on-failure";
          RestartSec = 10;
          ExecStart = "${config.services.tailscale.package}/bin/tailscale serve --bg --https=443 http://127.0.0.1:80";
        };
      };
    })
  ]);
}
