# NixOS module for opencode-status
#
# Usage in configuration.nix:
#   imports = [ inputs.opencode-status.nixosModules.default ];
#   services.opencode-status.enable = true;
#   services.opencode-status.webAddr = ":8080";
#   services.opencode-status.openDataDir = true; # persists to /var/lib/opencode-status
#
{ config, lib, pkgs, ... }:

let
  cfg = config.services.opencode-status;
  defaultPackage = pkgs.callPackage ../default.nix { };
in
{
  options.services.opencode-status = {
    enable = lib.mkEnableOption "opencode free-models status monitor";

    package = lib.mkOption {
      type = lib.types.package;
      default = defaultPackage;
      description = "The opencode-status package to use.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "opencode-status";
      description = "User account under which opencode-status runs.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "opencode-status";
      description = "Group under which opencode-status runs.";
    };

    dataDir = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/opencode-status";
      description = "Directory for the SQLite history database.";
    };

    pollInterval = lib.mkOption {
      type = lib.types.str;
      default = "5m";
      description = "How often to poll OpenRouter /api/v1/models.";
    };

    retentionDays = lib.mkOption {
      type = lib.types.int;
      default = 30;
      description = "Days of history to retain in SQLite.";
    };

    webAddr = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "HTTP listen address (e.g. :8080). Empty disables HTTP.";
    };

    enableTUI = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Run the TUI in the foreground. Default is daemon mode (HTTP only).";
    };

    openRouterKeyFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Path to a file containing the OpenRouter API key (optional).";
    };

    showPaid = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Include paid models in the tracked list.";
    };

    openFirewall = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Open the firewall for the HTTP port.";
    };
  };

  config = lib.mkIf cfg.enable {
    users.users = lib.mkIf (cfg.user == "opencode-status") {
      opencode-status = {
        isSystemUser = true;
        group = cfg.group;
        home = cfg.dataDir;
        description = "opencode-status service user";
      };
    };

    users.groups = lib.mkIf (cfg.group == "opencode-status") {
      opencode-status = { };
    };

    systemd.tmpfiles.rules = [
      "d ${cfg.dataDir} 0750 ${cfg.user} ${cfg.group} - -"
    ];

    systemd.services.opencode-status = {
      description = "opencode free-models status monitor";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;
        WorkingDirectory = cfg.dataDir;
        Restart = "on-failure";
        RestartSec = "30s";

        ExecStart = lib.concatStringsSep " " ([
          "${cfg.package}/bin/opencode-status"
          "--no-tui"
          "--db ${cfg.dataDir}/history.db"
          "--interval ${cfg.pollInterval}"
          "--retention-days ${toString cfg.retentionDays}"
        ] ++ lib.optional (cfg.webAddr != "") "--web ${cfg.webAddr}"
          ++ lib.optional (!cfg.enableTUI) ""
          ++ lib.optional cfg.showPaid "--show-paid"
          ++ lib.optional (cfg.openRouterKeyFile != null) "--openrouter-key $(cat ${toString cfg.openRouterKeyFile})");

        # Hardening
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        ReadWritePaths = [ cfg.dataDir ];
        CapabilityBoundingSet = [ "" ];
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        RestrictNamespaces = true;
        SystemCallArchitectures = "native";
        LockPersonality = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
      };
    };

    networking.firewall = lib.mkIf cfg.openFirewall {
      allowedTCPPorts = lib.optional (cfg.webAddr != "") (lib.toInt cfg.webAddr);
    };
  };
}
