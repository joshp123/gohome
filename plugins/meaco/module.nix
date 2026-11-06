{ lib, ... }:
with lib;
{
  options.services.gohome.plugins.meaco = mkOption {
      type = types.nullOr (types.submodule {
        options = {
          bootstrapFile = mkOption {
            type = types.str;
            description = "Agenix Meaco local bootstrap JSON path";
          };
          deviceIp = mkOption {
            type = types.str;
            default = "";
            description = "Device IP override; empty uses the bootstrap address";
          };
        };
      });
      default = null;
      description = "Meaco Arete Two plugin (presence enables it)";
    };

}
