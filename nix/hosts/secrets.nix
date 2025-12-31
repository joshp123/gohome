{ config, lib, secrets, ... }:
{
  age.secrets =
    {
      tailscale-authkey = {
        file = "${secrets}/tailscale-authkey.age";
        owner = "gohome";
        group = "gohome";
        mode = "0400";
      };
    }
    // lib.optionalAttrs config.services.gohome.plugins.tado.enable {
      tado-token = {
        file = "${secrets}/homeassistant-tado-refresh.age";
        owner = "gohome";
        group = "gohome";
        mode = "0400";
      };
    };
}
