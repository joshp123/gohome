{ config, lib, ... }:

{
  imports = [
    ./secrets.nix
  ];

  networking.hostName = "gohome";

  services.gohome = {
    enable = true;
    plugins.tado.enable = true;
    plugins.tado.tokenFile = config.age.secrets.tado-token.path;
    tailscale.authKeyFile = config.age.secrets.tailscale-authkey.path;
  };
}
