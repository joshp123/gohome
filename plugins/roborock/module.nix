{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.plugins.roborock;

in
{
  options.services.gohome.plugins.roborock = {
    enable = mkEnableOption "Roborock plugin";

    bootstrapFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Roborock bootstrap JSON";
    };

    cloudFallback = mkOption {
      type = types.bool;
      default = false;
      description = "Allow cloud fallback when local device is unreachable";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.bootstrapFile != null;
        message = "services.gohome.plugins.roborock.bootstrapFile is required";
      }
    ];
  };
}
