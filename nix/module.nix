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

  oauthEnv = ''
    GOHOME_OAUTH_BLOB_ENDPOINT=${cfg.oauth.blobEndpoint}
    GOHOME_OAUTH_BLOB_BUCKET=${cfg.oauth.blobBucket}
    GOHOME_OAUTH_BLOB_PREFIX=${cfg.oauth.blobPrefix}
    GOHOME_OAUTH_BLOB_ACCESS_KEY_FILE=${cfg.oauth.blobAccessKeyFile}
    GOHOME_OAUTH_BLOB_SECRET_KEY_FILE=${cfg.oauth.blobSecretKeyFile}
  '' + optionalString (cfg.oauth.blobRegion != null) ''
    GOHOME_OAUTH_BLOB_REGION=${cfg.oauth.blobRegion}
  '';

  tadoEnv = optionalString (cfg.plugins.tado.enable or false) ''
    GOHOME_TADO_BOOTSTRAP_FILE=${cfg.plugins.tado.bootstrapFile}
  '' + optionalString (cfg.plugins.tado.enable or false && cfg.plugins.tado.homeId != null) ''
    GOHOME_TADO_HOME_ID=${toString cfg.plugins.tado.homeId}
  '';

  daikinEnv = optionalString (cfg.plugins.daikin.enable or false) ''
    GOHOME_DAIKIN_BOOTSTRAP_FILE=${cfg.plugins.daikin.bootstrapFile}
  '';

  envFile = pkgs.writeText "gohome-env" (''
    GOHOME_GRPC_ADDR=${cfg.listenAddress}:${toString cfg.grpcPort}
    GOHOME_HTTP_ADDR=${cfg.listenAddress}:${toString cfg.httpPort}
    GOHOME_ENABLED_PLUGINS_FILE=/etc/gohome/enabled-plugins
    GOHOME_DASHBOARD_DIR=/var/lib/gohome/dashboards
  '' + oauthEnv + tadoEnv + daikinEnv);

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

    grafanaEnvFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Optional EnvironmentFile for Grafana overrides (e.g., GF_SERVER_DOMAIN/GF_SERVER_ROOT_URL) to avoid committing tailnet URLs.";
    };

    oauth = {
      blobEndpoint = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "S3-compatible endpoint for OAuth state blob mirror (e.g., Hetzner Object Storage endpoint)";
      };

      blobBucket = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Bucket name for OAuth state blob mirror";
      };

      blobPrefix = mkOption {
        type = types.str;
        default = "gohome/oauth";
        description = "Prefix for OAuth state objects in blob storage";
      };

      blobAccessKeyFile = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to blob access key (read-only secret)";
      };

      blobSecretKeyFile = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to blob secret key (read-only secret)";
      };

      blobRegion = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "Optional blob region";
      };
    };

    plugins.tado = {
      enable = mkEnableOption "Tado plugin";

      bootstrapFile = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to bootstrap Tado OAuth credentials (read-only secret)";
      };

      homeId = mkOption {
        type = types.nullOr types.int;
        default = null;
        description = "Optional homeId override (if /me contains multiple homes)";
      };
    };

    plugins.daikin = {
      enable = mkEnableOption "Daikin Onecta plugin";

      bootstrapFile = mkOption {
        type = types.nullOr types.path;
        default = null;
        description = "Path to bootstrap Daikin Onecta OAuth credentials (read-only secret)";
      };
    };
  };

  config = mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.oauth.blobEndpoint != null;
        message = "services.gohome.oauth.blobEndpoint is required";
      }
      {
        assertion = cfg.oauth.blobBucket != null;
        message = "services.gohome.oauth.blobBucket is required";
      }
      {
        assertion = cfg.oauth.blobAccessKeyFile != null;
        message = "services.gohome.oauth.blobAccessKeyFile is required";
      }
      {
        assertion = cfg.oauth.blobSecretKeyFile != null;
        message = "services.gohome.oauth.blobSecretKeyFile is required";
      }
      {
        assertion = !(cfg.plugins.tado.enable or false) || cfg.plugins.tado.bootstrapFile != null;
        message = "services.gohome.plugins.tado.bootstrapFile is required when tado is enabled";
      }
      {
        assertion = !(cfg.plugins.daikin.enable or false) || cfg.plugins.daikin.bootstrapFile != null;
        message = "services.gohome.plugins.daikin.bootstrapFile is required when daikin is enabled";
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
      "d /var/lib/gohome 0755 gohome gohome - -"
      "d /var/lib/gohome/dashboards 0755 gohome gohome - -"
    ];
  };
}
