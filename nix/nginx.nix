{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

in
{
  config = mkIf cfg.enable {
    services.nginx = {
      enable = true;
      recommendedProxySettings = true;

      virtualHosts."gohome" = {
        listen = [ { addr = "0.0.0.0"; port = 80; } ];
        default = true;
        # The house is read in Grafana; its home dashboard is the House overview.
        locations."= /".return = "302 /grafana/";
        locations."/grafana/".proxyPass = "http://127.0.0.1:3000";
        locations."/vm/".proxyPass = "http://127.0.0.1:8428";
        locations."/gohome/".proxyPass = "http://127.0.0.1:${toString cfg.httpPort}/";
      };
    };

    networking.firewall = {
      enable = true;
      trustedInterfaces = [ "tailscale0" ];
      allowedTCPPorts = [ ];
    };
  };
}
