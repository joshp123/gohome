{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

  # A camera picture route, for tailnet clients only.
  cameraPicture = picture: {
    proxyPass = "http://${cfg.remotePlugins.hikvision or "127.0.0.1:${toString cfg.httpPort}"}/hikvision/${picture}";
    extraConfig = ''
      proxy_buffering off;
      proxy_cache off;
      allow 100.64.0.0/10;
      allow fd7a:115c:a1e0::/48;
      deny all;
    '';
  };

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
          # nginx sees that connection as plain HTTP, so its redirects keep only the path: the browser stays on HTTPS.
          extraConfig = ''
            set_real_ip_from 127.0.0.1;
            real_ip_header X-Forwarded-For;
            absolute_redirect off;
          '';
          # The house is read in Grafana; its home dashboard is the House overview.
          locations."= /".return = "302 /grafana/";
          locations."/grafana/".proxyPass = "http://127.0.0.1:3000";
          locations."/vm/".proxyPass = "http://127.0.0.1:8428";
          locations."/gohome/".proxyPass = "http://127.0.0.1:${toString cfg.httpPort}/";
          # Camera pictures: the latest, one from the recording at ?at= (the
          # query string is passed on), and the latest alarm's.
          locations."= /gohome/hikvision/snapshot.jpg" = cameraPicture "snapshot.jpg";
          locations."= /gohome/hikvision/frame.jpg" = cameraPicture "frame.jpg";
          locations."= /gohome/hikvision/alarm.jpg" = cameraPicture "alarm.jpg";
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
          # Tailscale keeps the setting across restarts, so stopping this unit (a rollback, or removing it) must turn it off.
          ExecStop = "${config.services.tailscale.package}/bin/tailscale serve --https=443 off";
        };
      };
    })
  ]);
}
