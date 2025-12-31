{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gohome;

  gohomePkg = pkgs.callPackage ./package.nix {
    version = cfg.packageVersion;
  };

  enabledPlugins = builtins.attrNames (filterAttrs (_: v: v.enable or false) cfg.plugins);

  tadoEnv = optionalString (cfg.plugins.tado.enable or false) ''
    GOHOME_TADO_TOKEN_FILE=${cfg.plugins.tado.tokenFile}
  '' + optionalString (cfg.plugins.tado.enable or false && cfg.plugins.tado.homeId != null) ''
    GOHOME_TADO_HOME_ID=${toString cfg.plugins.tado.homeId}
  '';

  envFile = pkgs.writeText "gohome-env" ''
    GOHOME_GRPC_ADDR=${cfg.listenAddress}:${toString cfg.grpcPort}
    GOHOME_HTTP_ADDR=${cfg.listenAddress}:${toString cfg.httpPort}
    GOHOME_ENABLED_PLUGINS_FILE=/etc/gohome/enabled-plugins
  '' + tadoEnv;

in
{
  imports = [
    ./nginx.nix
    ./grafana.nix
    ./victoriametrics.nix
    ../plugins/tado/module.nix
  ];

  options.services.gohome = {
    enable = mkEnableOption "GoHome";

    packageVersion = mkOption {
      type = types.str;
      default = "0.1.0";
      description = "GoHome package version";
    };

    listenAddress = mkOption {
      type = types.str;
      default = "0.0.0.0";
      description = "Listen address for gRPC and HTTP";
    };

    grpcPort = mkOption {
      type = types.port;
      default = 9000;
      description = "gRPC port";
    };

    httpPort = mkOption {
      type = types.port;
      default = 8080;
      description = "HTTP port (health/metrics/dashboards)";
    };

    plugins = mkOption {
      type = types.attrsOf (types.submodule ({ name, ... }: {
        freeformType = types.attrs;
        options = {
          enable = mkEnableOption "GoHome plugin ${name}";
        };
      }));
      default = { };
      description = "Plugin enablement set";
    };
  };

  config = mkIf cfg.enable {
    users.users.gohome = {
      isSystemUser = true;
      group = "gohome";
    };

    users.groups.gohome = { };

    systemd.services.gohome = {
      description = "GoHome";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      serviceConfig = {
        User = "gohome";
        Group = "gohome";
        EnvironmentFile = envFile;
        ExecStart = "${gohomePkg}/bin/gohome";
        Restart = "on-failure";
      };
    };

    environment.etc."gohome/enabled-plugins".text = builtins.concatStringsSep "\n" enabledPlugins;
  };
}
