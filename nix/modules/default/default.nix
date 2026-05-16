{ self, ... }:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.setup-secrets;
  nixos-setup-secrets = pkgs.buildGo126Module {
    name = "nixos-setup-secrets";
    # version = "0.7.0";
    meta.mainProgram = "nixos-setup-secrets";
    src = ./src;
    proxyVendor = true;
    vendorHash = "sha256-OkAoJ3ysAK9tW69S7HeYPnhrkQL9JgHKHOQoYwQL6yc=";
  };
  nixos-setup-secrets-wrapper = pkgs.writeShellScriptBin "setup-secrets" ''
    PATH=${pkgs.lib.makeBinPath pkgs.bash}
    export NIXOS_SETUP_SECRETS_CONFIG=$(cat <<'EOF'
    ${builtins.toJSON {
      sources = cfg.sources;
      destinations = cfg.destinations;
    }}
    EOF
    )
    exec ${lib.getExe nixos-setup-secrets} "$@"
  '';
in
{
  options.setup-secrets = {
    enable = lib.mkEnableOption "the cli utility for setting up secrets";
    autoSetup = lib.mkEnableOption "automatic fetching & storing of secrets";
    sources = lib.mkOption {
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
                type = lib.types.nullOr lib.types.str;
                description = "Command to retrieve the secret";
                default = null;
              };
            };
          }
        )
      );
    };
    destinations = lib.mkOption {
      description = "List of commands to run when the form is submitted.";
      type = lib.types.listOf (
        lib.types.submodule {
          options = {
            enable = lib.mkEnableOption "this secret destination";
            logPrefix = lib.mkOption {
              description = "A short description of the destination.";
              type = lib.types.str;
              default = "";
            };
            requires = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              description = "List of secrets this command requires in order to execute";
              default = [ ];
            };
            wants = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              description = "List of secrets this command wants (but not requires)";
              default = [ ];
            };
            cmd = lib.mkOption {
              type = lib.types.str;
              description = "Command to create or update the secret";
            };
          };
        }
      );
    };
    script = lib.mkOption {
      description = "The generated setup-secrets script";
      type = lib.types.package;
      readOnly = true;
      default = nixos-setup-secrets-wrapper;
    };
  };
  config = {
    assertions = lib.flatten (
      map (
        dest:
        map (src: {
          assertion = builtins.elem src (builtins.attrNames cfg.sources);
          message = ''
            setup-secrets.destinations with logEntry \"${dest.logEntry}\" requires a secret source that is not defined: ${src}
            A secret source need not have a command set, but it must be defined.
          '';
        }) dest.requires
      ) (lib.filter ({ enable, ... }: enable) cfg.destinations)
    );
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
