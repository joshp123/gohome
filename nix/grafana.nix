{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

in
{
  config = mkIf cfg.enable {
    services.grafana = {
      enable = true;
      settings = {
        server = {
          http_addr = "127.0.0.1";
          http_port = 3000;
          domain = config.networking.hostName;
          root_url = "%(protocol)s://%(domain)s/grafana/";
          serve_from_sub_path = true;
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
          }
        ];
      };
    };
  };
}
