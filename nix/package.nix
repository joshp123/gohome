{ lib, buildGoModule, version }:

buildGoModule {
  pname = "gohome";
  inherit version;
  src = ../.;
  vendorHash = null;

  meta = with lib; {
    description = "Nix-native home automation";
    homepage = "https://github.com/elliot-alderson/gohome";
    license = licenses.agpl3Plus;
    maintainers = [ ];
  };
}
