{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

in
{
  config = mkIf cfg.enable {
    services.victoriametrics = {
      enable = true;
      retentionPeriod = "100y";
      listenAddress = "127.0.0.1:8428";
      prometheusConfig = {
        scrape_configs = map (source:
          {
            job_name = "gohome-${source}";
            scrape_interval = "15s";
            metrics_path = "/metrics/${source}";
            scrape_timeout = "12s";
            static_configs = [
              {
                targets = [ "127.0.0.1:8080" ];
                labels = {
                  instance = "gohome";
                  job = "gohome";
                  source = source;
                };
              }
            ];
          }
        ) ([ "core" ] ++ builtins.attrNames (lib.filterAttrs (_: plugin: plugin != null) cfg.plugins));
      };
      extraOptions = [
        "-http.pathPrefix=/vm"
      ];
    };
  };
}
