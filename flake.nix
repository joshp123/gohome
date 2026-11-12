{
  description = "GoHome - Nix-native home automation";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.callPackage ./nix/package.nix {
          version = "0.1.0";
          buildTags = [ "gohome_plugin_tado" ];
          buildCommit = self.rev or "dirty";
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
      openclawPlugin = system: {
        name = "gohome";
        skills = [ ./skills/gohome ];
        packages = [ self.packages.${system}.default ];
        needs = {
          stateDirs = [];
          requiredEnv = [];
        };
      };

      nixosModules.default = import ./nix/module.nix;
    };
}
