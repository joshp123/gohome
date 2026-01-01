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
      gohome-oauth-blob-access-key = {
        file = "${secrets}/gohome-oauth-blob-access-key.age";
        owner = "gohome";
        group = "gohome";
        mode = "0400";
      };
      gohome-oauth-blob-secret-key = {
        file = "${secrets}/gohome-oauth-blob-secret-key.age";
        owner = "gohome";
        group = "gohome";
        mode = "0400";
      };
    }
    // lib.optionalAttrs (config.services.gohome.plugins.tado != null) {
      tado-token = {
        file = "${secrets}/homeassistant-tado-refresh.age";
        owner = "gohome";
        group = "gohome";
        mode = "0400";
      };
    }
    // lib.optionalAttrs (config.services.gohome.plugins.daikin != null) {
      daikin-bootstrap = {
        file = "${secrets}/gohome-daikin-bootstrap.age";
        owner = "gohome";
        group = "gohome";
        mode = "0400";
      };
    };
}
