{ secrets, ... }:
{
  age.secrets = {
    tado-token.file = "${secrets}/homeassistant-tado-refresh.age";
    tailscale-authkey.file = "${secrets}/tailscale-authkey.age";
  };
}
