{
  description = "GoHome - Nix-native home automation";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "gohome";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # Update after go mod tidy

          nativeBuildInputs = [
            pkgs.protobuf
            pkgs.protoc-gen-go
            pkgs.protoc-gen-go-grpc
          ];

          meta = with pkgs.lib; {
            description = "Nix-native home automation";
            homepage = "https://github.com/joshp123/gohome";
            license = licenses.agpl3Plus;
            maintainers = [ ];
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            delve
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            grpcurl
            opentofu
          ];
        };
      }
    ) // {
      nixosModules.default = import ./nix/module.nix;
    };
}
