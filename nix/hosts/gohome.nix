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
    plugins.tado.enable = false;
    tailscale.authKeyFile = config.age.secrets.tailscale-authkey.path;
  };
}
