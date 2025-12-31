{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gohome;

  enabledPlugins = builtins.attrNames (filterAttrs (_: v: v.enable or false) cfg.plugins);
  pluginTags = map (name: "gohome_plugin_${name}") enabledPlugins;

  gohomePkg = pkgs.callPackage ./package.nix {
    version = cfg.packageVersion;
    buildTags = pluginTags;
    buildCommit =
      if config.system.configurationRevision == null
      then "unknown"
      else config.system.configurationRevision;
  };

  tadoEnv = optionalString (cfg.plugins.tado.enable or false) ''
    GOHOME_TADO_TOKEN_FILE=${cfg.plugins.tado.tokenFile}
  '' + optionalString (cfg.plugins.tado.enable or false && cfg.plugins.tado.homeId != null) ''
    GOHOME_TADO_HOME_ID=${toString cfg.plugins.tado.homeId}
  '';

  envFile = pkgs.writeText "gohome-env" ''
    GOHOME_GRPC_ADDR=${cfg.listenAddress}:${toString cfg.grpcPort}
    GOHOME_HTTP_ADDR=${cfg.listenAddress}:${toString cfg.httpPort}
    GOHOME_ENABLED_PLUGINS_FILE=/etc/gohome/enabled-plugins
    GOHOME_DASHBOARD_DIR=/var/lib/gohome/dashboards
  '' + tadoEnv;

in
{
  imports = [
    ./nginx.nix
    ./grafana.nix
    ./victoriametrics.nix
    ./tailscale.nix
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

    plugins.tado = {
      enable = mkEnableOption "Tado plugin";

      tokenFile = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to Tado OAuth refresh token JSON";
      };

      homeId = mkOption {
        type = types.nullOr types.int;
        default = null;
        description = "Optional homeId override (if /me contains multiple homes)";
      };
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = !(cfg.plugins.tado.enable or false) || cfg.plugins.tado.tokenFile != null;
        message = "services.gohome.plugins.tado.tokenFile is required when tado is enabled";
      }
    ];

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

    systemd.tmpfiles.rules = [
      "d /var/lib/gohome/dashboards 0755 gohome gohome - -"
    ];
  };
}
