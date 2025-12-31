{ secrets, ... }:
{
  age.secrets = {
    tado-token = {
      file = "${secrets}/homeassistant-tado-refresh.age";
      owner = "gohome";
      group = "gohome";
      mode = "0400";
    };
    tailscale-authkey = {
      file = "${secrets}/tailscale-authkey.age";
      owner = "gohome";
      group = "gohome";
      mode = "0400";
    };
  };
}
