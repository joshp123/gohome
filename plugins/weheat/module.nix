{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.plugins.weheat;

in
{
  options.services.gohome.plugins.weheat = {
    enable = mkEnableOption "Weheat plugin";

    bootstrapFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Weheat OAuth credentials bootstrap JSON";
    };

    baseUrl = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Optional Weheat API base URL override";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.bootstrapFile != null;
        message = "services.gohome.plugins.weheat.bootstrapFile is required";
      }
    ];
  };
}
