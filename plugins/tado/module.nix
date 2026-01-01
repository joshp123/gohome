{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.plugins.tado;

in
{
  options.services.gohome.plugins.tado = {
    enable = mkEnableOption "Tado plugin";

    bootstrapFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Tado OAuth credentials bootstrap JSON";
    };

    homeId = mkOption {
      type = types.nullOr types.int;
      default = null;
      description = "Optional homeId override (if /me contains multiple homes)";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.bootstrapFile != null;
        message = "services.gohome.plugins.tado.bootstrapFile is required";
      }
    ];
  };
}
