# services.corsarr: the release binary as a hardened systemd service, the same
# sandbox as deploy/systemd/corsarr.service.
self:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.corsarr;
  yaml = pkgs.formats.yaml { };
  configFile =
    if cfg.configFile != null then cfg.configFile
    else yaml.generate "corsarr.yml" cfg.settings;
  inherit (lib) mkOption mkEnableOption mkIf mkDefault types;
in
{
  options.services.corsarr = {
    enable = mkEnableOption "corsarr, the media automation server";

    package = mkOption {
      type = types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.corsarr;
      defaultText = lib.literalExpression "corsarr.packages.\${system}.corsarr";
      description = "The corsarr package.";
    };

    port = mkOption {
      type = types.port;
      default = 8090;
      description = "Port of the web UI and API (sets server.address).";
    };

    openFirewall = mkOption {
      type = types.bool;
      default = false;
      description = "Open the port in the firewall.";
    };

    settings = mkOption {
      type = yaml.type;
      default = { };
      example = lib.literalExpression ''
        {
          log.level = "debug";
          mediaKinds.book.mode = "active";
          # secrets as references, never values
          metadata.tmdb = {
            type = "tmdb";
            apiKey = "\''${file:/run/credentials/corsarr.service/tmdb_api_key}";
          };
        }
      '';
      description = ''
        corsarr.yml as Nix, written to /etc/corsarr/corsarr.yml. Every option is listed
        by `corsarr config defaults --format=yaml`. Keep data.dir at /var/lib/corsarr
        (the state directory).
      '';
    };

    configFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "A corsarr.yml to use instead of `settings`.";
    };

    user = mkOption {
      type = types.str;
      default = "corsarr";
      description = "User corsarr runs as (created when left at the default).";
    };

    group = mkOption {
      type = types.str;
      default = "corsarr";
      description = "Group corsarr runs as; a shared media group lets it write the library.";
    };

    mediaPaths = mkOption {
      type = types.listOf types.str;
      default = [ ];
      example = [ "/srv/media" ];
      description = "Paths corsarr may write (downloads and library, on one filesystem).";
    };

    readOnlyMediaPaths = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = "Paths corsarr may only read (shadow mode).";
    };

    credentials = mkOption {
      type = types.attrsOf types.path;
      default = { };
      example = { tmdb_api_key = "/run/secrets/tmdb_api_key"; };
      description = ''
        Secret files handed to the service with LoadCredential=; reference them in
        settings as ''${file:/run/credentials/corsarr.service/<name>}.
      '';
    };

    environmentFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = "Environment file for values referenced as \${env:NAME}.";
    };
  };

  config = mkIf cfg.enable {
    services.corsarr.settings = {
      version = mkDefault 1;
      server.address = mkDefault ":${toString cfg.port}";
      data.dir = mkDefault "/var/lib/corsarr";
      log.level = mkDefault "info";
      auth.mode = mkDefault "password";
    };

    users.users = mkIf (cfg.user == "corsarr") {
      corsarr = {
        isSystemUser = true;
        inherit (cfg) group;
        home = "/var/lib/corsarr";
      };
    };
    users.groups = mkIf (cfg.group == "corsarr") { corsarr = { }; };

    environment.etc."corsarr/corsarr.yml".source = configFile;
    environment.systemPackages = [ cfg.package ];
    networking.firewall.allowedTCPPorts = mkIf cfg.openFirewall [ cfg.port ];

    systemd.services.corsarr = {
      description = "corsarr media automation server";
      documentation = [ "https://github.com/jrand0m/corsarr" ];
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      restartTriggers = [ configFile ];
      environment.GOMEMLIMIT = "96MiB";
      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;
        ExecStart = "${lib.getExe cfg.package} --configFile=/etc/corsarr/corsarr.yml";
        Restart = "on-failure";
        RestartSec = "5s";
        TimeoutStopSec = "30s";
        StateDirectory = "corsarr";
        StateDirectoryMode = "0750";
        UMask = "0002";
        MemoryMax = "256M";
        TasksMax = 256;
        ReadWritePaths = map (p: "-${p}") cfg.mediaPaths;
        ReadOnlyPaths = map (p: "-${p}") cfg.readOnlyMediaPaths;
        LoadCredential = lib.mapAttrsToList (name: path: "${name}:${path}") cfg.credentials;
        EnvironmentFile = lib.optional (cfg.environmentFile != null) cfg.environmentFile;

        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        AmbientCapabilities = "";
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        DevicePolicy = "closed";
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;
        ProtectClock = true;
        ProtectHostname = true;
        ProtectProc = "invisible";
        ProcSubset = "pid";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        RemoveIPC = true;
        KeyringMode = "private";
        SystemCallArchitectures = "native";
        SystemCallFilter = [ "@system-service" "~@privileged @resources" ];
        SystemCallErrorNumber = "EPERM";
      };
    };
  };
}
