{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome;

  localSources = [ "core" ] ++ builtins.attrNames (lib.filterAttrs (_: plugin: plugin != null) cfg.plugins);
  targets = genAttrs localSources (_: "127.0.0.1:${toString cfg.httpPort}") // cfg.remotePlugins;

  # Sources whose stamped readings are stored at their own time: Daikin's electricity per two hours, day and
  # month at the period's start, Roborock's run records at the run's end, and P1's power-failure event log
  # at each outage's restore time. Live gauges without explicit timestamps still use scrape time.
  # Growatt also stamps its readings, with the inverter's report time, but leaves them as scraped: those
  # readings are 3 to 14 minutes old when scraped, older than the lookback of an instant query, so tiles such
  # as Home's Solar would go blank. Readings from the inverter's own time come from the history backfill.
  honourTimestamps = [ "daikin" "roborock" "p1_homewizard" ];

  # Per-source scrape intervals: keep fast local/live telemetry at 15s, moderate local/bridge sources at 30s,
  # and slow cloud-throttled sources at 60s to avoid pointless disk churn and local RPC bursts.
  scrapeIntervalFor = source:
    if elem source [ "growatt" "daikin" "brother" ] then "60s"
    else if elem source [ "roborock" "tado" "airgradient" "sonos" "hikvision" "applehome" "unifi" ] then "30s"
    else "15s";

  # Drop high-cardinality per-characteristic read timestamps from Apple Home (530+ unused series);
  # accessory reachability and current values already carry freshness.
  relabelConfigsFor = source:
    if source == "applehome" then [
      {
        source_labels = [ "__name__" ];
        regex = "gohome_applehome_characteristic_last_read_timestamp_seconds";
        action = "drop";
      }
    ] else [ ];

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
            scrape_interval = scrapeIntervalFor source;
            metrics_path = "/metrics/${source}";
            scrape_timeout = "12s";
            honor_timestamps = elem source honourTimestamps;
            metric_relabel_configs = relabelConfigsFor source;
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
