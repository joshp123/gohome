{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

  localSources = [ "core" ] ++ builtins.attrNames (lib.filterAttrs (_: plugin: plugin != null) cfg.plugins);
  targets = genAttrs localSources (_: "127.0.0.1:${toString cfg.httpPort}") // cfg.remotePlugins;

  # Sources whose stamped readings are stored at their own time: Daikin's electricity per two hours, day and
  # month at the period's start, and Roborock's run records at the run's end. VictoriaMetrics ignores a
  # sample's own timestamp unless the job says otherwise, and then stores every scrape's copy of the latest
  # bucket or run at the scrape time.
  # Growatt also stamps its readings, with the inverter's report time, but leaves them as scraped: those
  # readings are 3 to 14 minutes old when scraped, older than the lookback of an instant query, so tiles such
  # as Home's Solar would go blank. Readings from the inverter's own time come from the history backfill.
  honourTimestamps = [ "daikin" "roborock" ];

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
            honor_timestamps = elem source honourTimestamps;
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
