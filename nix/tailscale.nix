{ config, lib, ... }:

with lib;

let
  cfg = config.services.gohome.tailscale;

in
{
  options.services.gohome.tailscale = {
    enable = mkOption {
      type = types.bool;
      default = true;
      description = "Enable Tailscale for GoHome host";
    };

    authKeyFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Path to Tailscale auth key";
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.authKeyFile != null;
        message = "services.gohome.tailscale.authKeyFile is required when tailscale is enabled";
      }
    ];

    services.tailscale = {
      enable = true;
      authKeyFile = cfg.authKeyFile;
      extraUpFlags = [ "--ssh" "--accept-routes" ];
    };
  };
}
