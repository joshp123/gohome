{ lib, buildGoModule, version, buildTags ? [ ], buildCommit ? "unknown" }:

buildGoModule {
  pname = "gohome";
  inherit version;
  src = ../.;
  vendorHash = null;
  subPackages = [
    "cmd/gohome"
    "cmd/gohome-cli"
  ];

  tags = buildTags;
  ldflags = [
    "-X main.buildVersion=${version}"
    "-X main.buildCommit=${buildCommit}"
  ];

  meta = with lib; {
    description = "Nix-native home automation";
    homepage = "https://github.com/joshp123/gohome";
    license = licenses.agpl3Plus;
    maintainers = [ ];
  };
}
