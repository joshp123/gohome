{ config, lib, ... }:

{
  imports = [
    ./secrets.nix
    ./gohome-hardware.nix
  ];

  networking.hostName = "gohome";

  boot.loader.systemd-boot.enable = false;
  boot.loader.grub = {
    enable = true;
    efiSupport = true;
    efiInstallAsRemovable = true;
    device = "nodev";
  };
  boot.loader.efi.canTouchEfiVariables = false;
  boot.loader.efi.efiSysMountPoint = "/boot";

  services.openssh.enable = true;
  users.users.root.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOLItFT3SVm5r7gELrfRRJxh6V2sf/BIx7HKXt6oVWpB joshpalmer123@gmail.com"
  ];

  age.identityPaths = [ "/etc/ssh/ssh_host_ed25519_key" ];

  system.stateVersion = "23.05";

  services.gohome = {
    enable = true;

    oauth = {
      blobEndpoint = "https://s3.eu-central-1.amazonaws.com";
      blobBucket = "gohome-oauth-homelab-eu-central-1";
      blobPrefix = "gohome/oauth";
      blobAccessKeyFile = config.age.secrets.gohome-oauth-blob-access-key.path;
      blobSecretKeyFile = config.age.secrets.gohome-oauth-blob-secret-key.path;
      blobRegion = "eu-central-1";
    };

    plugins.tado.enable = true;
    plugins.tado.bootstrapFile = config.age.secrets.tado-token.path;
    plugins.daikin.enable = true;
    plugins.daikin.bootstrapFile = config.age.secrets.daikin-bootstrap.path;
    tailscale.authKeyFile = config.age.secrets.tailscale-authkey.path;
  };
}
