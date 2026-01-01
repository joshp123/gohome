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
        scrape_configs = [
          {
            job_name = "gohome";
            scrape_interval = "15s";
            metrics_path = "/metrics";
            static_configs = [
              {
                targets = [ "127.0.0.1:8080" ];
                labels = {
                  instance = "gohome";
                };
              }
            ];
          }
        ];
      };
      extraOptions = [
        "-http.pathPrefix=/vm"
      ];
    };
  };
}
