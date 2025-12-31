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
      extraOptions = [
        "-http.pathPrefix=/vm"
      ];
    };
  };
}
