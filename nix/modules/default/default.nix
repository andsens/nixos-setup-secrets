{ self, ... }:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.setup-secrets;
  vars = {
    records_sh = lib.escapeShellArg (
      pkgs.fetchzip {
        name = "records.sh";
        version = "1.0.3";
        url = "https://github.com/orbit-online/records.sh/releases/download/v1.0.3/records.sh.tar.gz";
        hash = "sha256-A3d3OolMGOv08PqdxzUbx65Y3lIpmonns4xzg+kuW9k=";
        stripRoot = false;
      }
    );
    docopt_sh = lib.escapeShellArg (
      pkgs.fetchzip {
        name = "docopt.sh";
        version = "2.0.3";
        url = "https://github.com/andsens/docopt.sh/releases/download/v2.0.3/docopt-lib.sh.tar.gz";
        hash = "sha256-L0J6aFgEcPPJnoJD6oZwtnAzGIB1R5cdZz6R7Ez5zcc=";
        stripRoot = false;
      }
    );
    PATH =
      with pkgs;
      lib.makeBinPath [
        bash
        coreutils
        dialog
      ];
    fetch = lib.join "\n" (
      lib.mapAttrsToList (
        name:
        { description, cmd }:
        lib.join " " (
          map lib.escapeShellArg [
            name
            description
            cmd
          ]
        )
      ) cfg.fetch
    );
    store = lib.join "\n" (
      map (
        { logEntry, cmd }:
        lib.join " " (
          map lib.escapeShellArg [
            logEntry
            cmd
          ]
        )
      ) cfg.store
    );
  };
  setup-secrets = pkgs.writeShellScriptBin "setup-secrets" (
    builtins.readFile (pkgs.replaceVars ./setup-secrets.sh vars)
  );
in
{
  options.setup-secrets = {
    enable = lib.mkEnableOption "the cli utility for setting up secrets";
    autoSetup = lib.mkEnableOption "automatic fetching & storing of secrets";
    fetch = lib.mkOption {
      description = "Map of secrets. <name> becomes the secret name.";
      type = lib.types.attrsOf (
        lib.types.submodule (
          { name, ... }:
          {
            options = {
              description = lib.mkOption {
                description = "The description of the secret as displayed in the form. Defaults to <name>";
                type = lib.types.str;
                default = name;
              };
              cmd = lib.mkOption {
                type = lib.types.str;
                description = "Command to retrieve the secret";
                default = [ ];
              };
            };
          }
        )
      );
    };
    store = lib.mkOption {
      description = "List of commands to run when the form is submitted.";
      type = lib.types.listOf (
        lib.types.submodule {
          options = {
            logEntry = lib.mkOption {
              description = "A description what the store command is doing.";
              type = lib.types.str;
              default = "";
            };
            cmd = lib.mkOption {
              type = lib.types.str;
              description = "Command to store the secrets";
              default = [ ];
            };
          };
        }
      );
    };
    script = lib.mkOption {
      description = "The generated setup-secrets script";
      type = lib.types.package;
      readOnly = true;
      default = setup-secrets;
    };
  };
  config = {
    systemd.services."setup-secrets" = {
      enable = cfg.autoSetup;
      description = "Automatically fetch & store NixOS secrets";
      wantedBy = [ "default.target" ];
      unitConfig = {
        Type = "oneshot";
        ExecStart = "${lib.getExe cfg.script} --auto";
      };
    };
  };
}
