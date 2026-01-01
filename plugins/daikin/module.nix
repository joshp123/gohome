{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.plugins.daikin;

in
{
  options.services.gohome.plugins.daikin = {
    enable = mkEnableOption "Daikin Onecta plugin";

    bootstrapFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Daikin Onecta OAuth credentials bootstrap JSON";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.bootstrapFile != null;
        message = "services.gohome.plugins.daikin.bootstrapFile is required";
      }
    ];
  };
}
