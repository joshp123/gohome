{ lib, buildGoModule, protobuf, protoc-gen-go, protoc-gen-go-grpc, version, buildTags ? [ ], buildCommit ? "unknown" }:

buildGoModule {
  pname = "gohome";
  inherit version;
  src = ../.;
  vendorHash = "sha256-7pDswPryddKO4spA9PtBJWRZuWuZoWVlD+0o+B1SKhk=";
  nativeBuildInputs = [
    protobuf
    protoc-gen-go
    protoc-gen-go-grpc
  ];

  preBuild = ''
    bash ./tools/generate.sh
  '';

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
