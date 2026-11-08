{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

  localSources = [ "core" ] ++ builtins.attrNames (lib.filterAttrs (_: plugin: plugin != null) cfg.plugins);
  targets = genAttrs localSources (_: "127.0.0.1:${toString cfg.httpPort}") // cfg.remotePlugins;

in
{
  config = mkIf (cfg.enable && cfg.monitoring.enable) {
    services.victoriametrics = {
      enable = true;
      retentionPeriod = "100y";
      listenAddress = "127.0.0.1:8428";
      prometheusConfig = {
        scrape_configs = mapAttrsToList (source: target:
          {
            job_name = "gohome-${source}";
            scrape_interval = "15s";
            metrics_path = "/metrics/${source}";
            scrape_timeout = "12s";
            static_configs = [
              {
                targets = [ target ];
                # The same labels wherever the plugin runs, so its dashboards and history carry over.
                labels = {
                  instance = "gohome";
                  job = "gohome";
                  source = source;
                };
              }
            ];
          }
        ) targets;
      };
      extraOptions = [
        "-http.pathPrefix=/vm"
      ];
    };
  };
}
