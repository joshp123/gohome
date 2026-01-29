{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.plugins.weheat;

in
{
  options.services.gohome.plugins.weheat = {
    enable = mkEnableOption "Weheat plugin";

    baseUrl = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Optional Weheat API base URL override";
    };
  };
}
