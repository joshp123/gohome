{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.plugins.growatt;

in
{
  options.services.gohome.plugins.growatt = {
    enable = mkEnableOption "Growatt plugin";

    tokenFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Growatt API token (read-only secret)";
    };

    region = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Growatt API region (other_regions, north_america, australia_new_zealand, china)";
    };

    plantId = mkOption {
      type = types.nullOr types.int;
      default = null;
      description = "Optional Growatt plant_id (required if multiple plants)";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.tokenFile != null;
        message = "services.gohome.plugins.growatt.tokenFile is required";
      }
    ];
  };
}
