{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.gohome;

  enabledPlugins = builtins.attrNames (filterAttrs (_: v: v != null) cfg.plugins);
  pluginTags = map (name: "gohome_plugin_${name}") enabledPlugins;

  gohomePkg = pkgs.callPackage ./package.nix {
    version = cfg.packageVersion;
    buildTags = pluginTags;
    buildCommit =
      if config.system.configurationRevision == null
      then "unknown"
      else config.system.configurationRevision;
  };

  escapeTextproto = value:
    replaceStrings [ "\\" "\"" "\n" "\r" "\t" ] [ "\\\\" "\\\"" "\\n" "\\r" "\\t" ] (toString value);

  textprotoString = value: "\"${escapeTextproto value}\"";
  textprotoMapString = field: attrs:
    lib.concatStringsSep "\n" (lib.mapAttrsToList (k: v: ''
      ${field} {
        key: ${textprotoString k}
        value: ${textprotoString v}
      }
    '') attrs);

  textprotoMapUIntString = field: attrs:
    lib.concatStringsSep "\n" (lib.mapAttrsToList (k: v: ''
      ${field} {
        key: ${k}
        value: ${textprotoString v}
      }
    '') attrs);

  configText = ''
    schema_version: 1
    core {
      grpc_addr: ${textprotoString "${cfg.listenAddress}:${toString cfg.grpcPort}"}
      http_addr: ${textprotoString "${cfg.listenAddress}:${toString cfg.httpPort}"}
      dashboard_dir: ${textprotoString "/var/lib/gohome/dashboards"}
    }
    oauth {
      blob_endpoint: ${textprotoString cfg.oauth.blobEndpoint}
      blob_bucket: ${textprotoString cfg.oauth.blobBucket}
      blob_prefix: ${textprotoString cfg.oauth.blobPrefix}
      blob_access_key_file: ${textprotoString cfg.oauth.blobAccessKeyFile}
      blob_secret_key_file: ${textprotoString cfg.oauth.blobSecretKeyFile}
  '' + optionalString (cfg.oauth.blobRegion != null) ''
      blob_region: ${textprotoString cfg.oauth.blobRegion}
  '' + ''
    }
  '' + optionalString (cfg.plugins.tado != null) ''
    tado {
      bootstrap_file: ${textprotoString cfg.plugins.tado.bootstrapFile}
  '' + optionalString (cfg.plugins.tado.homeId != null) ''
      home_id: ${toString cfg.plugins.tado.homeId}
  '' + ''
    }
  '' + optionalString (cfg.plugins.daikin != null) ''
    daikin {
      bootstrap_file: ${textprotoString cfg.plugins.daikin.bootstrapFile}
    }
  '' + optionalString (cfg.plugins.growatt != null) ''
    growatt {
      token_file: ${textprotoString cfg.plugins.growatt.tokenFile}
      region: ${textprotoString (if cfg.plugins.growatt.region == null then "other_regions" else cfg.plugins.growatt.region)}
  '' + optionalString (cfg.plugins.growatt.plantId != null) ''
      plant_id: ${toString cfg.plugins.growatt.plantId}
  '' + ''
    }
  '' + optionalString (cfg.plugins.roborock != null) ''
    roborock {
      bootstrap_file: ${textprotoString cfg.plugins.roborock.bootstrapFile}
      cloud_fallback: ${if cfg.plugins.roborock.cloudFallback then "true" else "false"}
  '' + optionalString (cfg.plugins.roborock.deviceIpOverrides != {}) ''
${textprotoMapString "device_ip_overrides" cfg.plugins.roborock.deviceIpOverrides}
  '' + optionalString (cfg.plugins.roborock.segmentNames != {}) ''
${textprotoMapUIntString "segment_names" cfg.plugins.roborock.segmentNames}
  '' + ''
    }
  '';

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

    plugins.tado = mkOption {
      type = types.nullOr (types.submodule {
        options = {
          bootstrapFile = mkOption {
            type = types.path;
            description = "Path to bootstrap Tado OAuth credentials (read-only secret)";
          };

          homeId = mkOption {
            type = types.nullOr types.int;
            default = null;
            description = "Optional homeId override (if /me contains multiple homes)";
          };
        };
      });
      default = null;
      description = "Tado plugin config (presence enables the plugin)";
    };

    plugins.daikin = mkOption {
      type = types.nullOr (types.submodule {
        options = {
          bootstrapFile = mkOption {
            type = types.path;
            description = "Path to bootstrap Daikin Onecta OAuth credentials (read-only secret)";
          };
        };
      });
      default = null;
      description = "Daikin Onecta plugin config (presence enables the plugin)";
    };

    plugins.growatt = mkOption {
      type = types.nullOr (types.submodule {
        options = {
          tokenFile = mkOption {
            type = types.path;
            description = "Path to Growatt API token (read-only secret)";
          };

          region = mkOption {
            type = types.nullOr types.str;
            default = null;
            description = "Growatt API region (other_regions, north_america, australia_new_zealand, china)";
          };

          plantId = mkOption {
            type = types.nullOr types.int;
            default = null;
            description = "Optional Growatt plant_id (required if multiple plants)";
          };
        };
      });
      default = null;
      description = "Growatt plugin config (presence enables the plugin)";
    };

    plugins.roborock = mkOption {
      type = types.nullOr (types.submodule {
        options = {
          bootstrapFile = mkOption {
            type = types.path;
            description = "Path to Roborock bootstrap JSON (read-only secret)";
          };

          cloudFallback = mkOption {
            type = types.bool;
            default = false;
            description = "Allow cloud fallback when local device is unreachable";
          };

          deviceIpOverrides = mkOption {
            type = types.attrsOf types.str;
            default = { };
            description = "Optional map of device_id -> LAN IP for local TCP without UDP broadcast";
          };

          segmentNames = mkOption {
            type = types.attrsOf types.str;
            default = { };
            description = "Optional map of segment_id -> human room name (numeric keys, e.g., \"18\" = \"kitchen\")";
          };
        };
      });
      default = null;
      description = "Roborock plugin config (presence enables the plugin)";
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
        assertion = cfg.plugins.tado == null || cfg.plugins.tado.bootstrapFile != null;
        message = "services.gohome.plugins.tado.bootstrapFile is required when tado is enabled";
      }
      {
        assertion = cfg.plugins.daikin == null || cfg.plugins.daikin.bootstrapFile != null;
        message = "services.gohome.plugins.daikin.bootstrapFile is required when daikin is enabled";
      }
      {
        assertion = cfg.plugins.growatt == null || cfg.plugins.growatt.tokenFile != null;
        message = "services.gohome.plugins.growatt.tokenFile is required when growatt is enabled";
      }
      {
        assertion = cfg.plugins.roborock == null || cfg.plugins.roborock.bootstrapFile != null;
        message = "services.gohome.plugins.roborock.bootstrapFile is required when roborock is enabled";
      }
    ];

    users.users.gohome = {
      isSystemUser = true;
      group = "gohome";
    };

    users.groups.gohome = { };

    systemd.services.gohome = let
      secretChecks =
        [
          "${pkgs.coreutils}/bin/test -r ${cfg.oauth.blobAccessKeyFile}"
          "${pkgs.coreutils}/bin/test -r ${cfg.oauth.blobSecretKeyFile}"
        ]
        ++ lib.optional (cfg.plugins.tado != null) "${pkgs.coreutils}/bin/test -r ${cfg.plugins.tado.bootstrapFile}"
        ++ lib.optional (cfg.plugins.daikin != null) "${pkgs.coreutils}/bin/test -r ${cfg.plugins.daikin.bootstrapFile}"
        ++ lib.optional (cfg.plugins.growatt != null) "${pkgs.coreutils}/bin/test -r ${cfg.plugins.growatt.tokenFile}"
        ++ lib.optional (cfg.plugins.roborock != null) "${pkgs.coreutils}/bin/test -r ${cfg.plugins.roborock.bootstrapFile}";
    in {
      description = "GoHome";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      serviceConfig = {
        User = "gohome";
        Group = "gohome";
        ExecStart = "${gohomePkg}/bin/gohome";
        Restart = "on-failure";
        ExecStartPre = secretChecks;
        RequiresMountsFor = [ "/run/agenix" ];
      };
    };

    environment.etc."gohome/config.pbtxt".text = configText;

    systemd.tmpfiles.rules = [
      "d /var/lib/gohome 0755 gohome gohome - -"
      "d /var/lib/gohome/dashboards 0755 gohome gohome - -"
    ];
  };
}
