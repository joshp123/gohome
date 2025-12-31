{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

in
{
  options.services.gohome.webRoot = mkOption {
    type = types.path;
    default = ./../web;
    description = "Path to GoHome web assets";
  };

  config = mkIf cfg.enable {
    services.nginx = {
      enable = true;
      recommendedProxySettings = true;

      virtualHosts."gohome" = {
        listen = [ { addr = "0.0.0.0"; port = 80; } ];
        default = true;
        locations."/" = {
          root = cfg.webRoot;
          tryFiles = "$uri /index.html";
        };
        locations."/grafana/".proxyPass = "http://127.0.0.1:3000/";
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
