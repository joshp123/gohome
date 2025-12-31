{
  description = "GoHome - Nix-native home automation";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    agenix.url = "github:ryantm/agenix";
    secrets = {
      url = "git+ssh://git@github.com/joshp123/nix-secrets";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, flake-utils, agenix, ... }@inputs:
    let
      secrets = inputs.secrets or null;
    in
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
      nixosModules.default = import ./nix/module.nix;

      nixosConfigurations =
        if secrets == null then
          { }
        else
          {
            gohome = nixpkgs.lib.nixosSystem {
              system = "aarch64-linux";
              modules = [
                agenix.nixosModules.default
                ./nix/hosts/gohome.nix
                ./nix/module.nix
              ];
              specialArgs = {
                inherit secrets;
              };
            };
          };
    };
}
