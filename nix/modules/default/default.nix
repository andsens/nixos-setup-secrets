{ self, inputs, ... }:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.setup-secrets;
  nixos-setup-secrets = pkgs.callPackage ./package.nix { };
  nixos-setup-secrets-wrapper = pkgs.writeShellScriptBin "setup-secrets" ''
    PATH=${pkgs.lib.makeBinPath [ pkgs.bash ]}
    umask 077
    export NIXOS_SETUP_SECRETS_STATE=${lib.escapeShellArg cfg.stateFile}
    export NIXOS_SETUP_SECRETS_CONFIG=$(${lib.getExe' pkgs.coreutils "cat"} <<'EOF'
    ${builtins.toJSON {
      sources = lib.mapAttrs (
        _: source:
        removeAttrs (fetched source) [
          "test"
          "testing"
        ]
      ) (lib.filterAttrs (name: { enable, ... }: enable) cfg.sources);
      destinations = enabledDestinations;
      inherit (cfg) outputKinds;
    }}
    EOF
    )
    exec ${lib.getExe nixos-setup-secrets} "$@"
  '';
  attemptsOptions = what: {
    timeout = lib.mkOption {
      description = "How long the command to ${what} may run, as a Go duration (`30s`, `2m`). A timeout counts as a failed try.";
      type = lib.types.str;
      default = "30s";
    };
    retries = lib.mkOption {
      description = "How often a failed command to ${what} is tried again, after 1s, 2s, 4s and so on up to 30s between tries.";
      type = lib.types.ints.unsigned;
      default = 0;
    };
  };
  enabledDestinations = lib.filter ({ enable, ... }: enable) cfg.destinations;
  # A source in a test fetches its test value, at once
  fetched =
    source:
    if source.testing && source.test != null then
      source
      // {
        cmd = source.test;
        retries = 0;
      }
    else
      source;
  # Every group a destination is in, and every group declared in `groups`
  groupNames = lib.unique (
    map ({ group, ... }: group) enabledDestinations ++ lib.attrNames cfg.groups
  );
  groupUnit = group: "setup-secrets@${group}";
in
{
  key = "${toString __curPos.file}#modules.nixos.setup-secrets";
  options.setup-secrets = {
    enable = lib.mkEnableOption "the module: its assertions, and with `autoSetup` its service and activation script. It doesn't install the CLI, which is `setup-secrets.script`";
    autoSetup = lib.mkEnableOption ''
      automatic fetching & storing of secrets: one instance of `setup-secrets@.service` per group, `setup-secrets@<group>.service`. A group with
      `onActivation` runs at boot before `sysinit.target` and again on every switch; the other groups are left to be
      pulled in by what they depend on'';
    groups = lib.mkOption {
      description = ''
        Settings of the groups the destinations are in. A group exists as soon as a destination is in it, so it needs
        an entry here only to change its settings.
      '';
      type = lib.types.attrsOf (
        lib.types.submodule (
          { name, ... }:
          {
            options.onActivation = lib.mkOption {
              description = ''
                Whether the group runs at boot, ordered before `sysinit.target`, and on every switch, before the switch
                starts or restarts any other unit (through `sysinit-reactivation.target`). Its unit has no default
                dependencies, so its commands can't count on the network or other services. Without it the group's
                unit runs only when something pulls it in, e.g. `wantedBy` and `partOf` the service its commands talk
                to, and again whenever its configuration changes while it is active.
              '';
              type = lib.types.bool;
              default = name == "host";
              defaultText = lib.literalExpression ''name == "host"'';
            };
          }
        )
      );
      default = { };
    };
    testing = lib.mkEnableOption ''
      the sources' test values: each source with a `test` fetches it in place of `cmd`, unless its own `testing` is
      off. For NixOS tests and test clusters, where the services a source reaches don't exist. It is built into the
      script, so a machine without it never writes a test value'';
    sources = lib.mkOption {
      description = "Map of secrets. <name> becomes the secret name.";
      type = lib.types.attrsOf (
        lib.types.submodule (
          { name, ... }:
          {
            options = {
              test = lib.mkOption {
                type = lib.types.nullOr lib.types.str;
                description = ''
                  Command that prints a stand-in for the secret, fetched in place of `cmd` where `testing` is on: for a
                  secret from a service or account a test doesn't have. Also for a source without `cmd`, which then
                  needs no one to type it in. A source that a test can fetch for real (a generated value, a Secret in
                  the test's own cluster) needs none.
                '';
                default = null;
                example = "echo test-api-key";
              };
              testing = lib.mkOption {
                type = lib.types.bool;
                description = "Whether this source fetches its `test` value in place of `cmd`. Set it to keep the real `cmd` in a test.";
                default = cfg.testing;
                defaultText = lib.literalExpression "config.setup-secrets.testing";
              };
              enable = lib.mkOption {
                description = "Whether to enable this secret source";
                type = lib.types.bool;
                default = true;
              };
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
              storedIn = lib.mkOption {
                type = lib.types.nullOr lib.types.str;
                description = ''
                  Where a destination of this secret stores the value that `cmd` reads back, as shown in the form
                  (`kube cert-manager/inwx-credentials`, `/etc/secrets.d/restic.key`). Set, the value can be edited in
                  the form and the edit sticks. Without it, a source with `cmd` reads its value from somewhere
                  setup-secrets never writes, so the form shows it read-only. A source without `cmd` is always
                  editable.
                '';
                default = null;
              };
              same = lib.mkOption {
                type = lib.types.nullOr lib.types.str;
                description = ''
                  Command that prints a cheap token that changes whenever the value does, without fetching the value: a
                  file's mtime, a Kubernetes Secret's resourceVersion. With `--changed-only`, which the units pass, a
                  destination runs only when the command or token of one of its secrets, or its own command, changed
                  since it last stored, or when one of its `outputs` is gone. A source without it always counts as
                  changed.

                  Exit 0 with the token on stdout; an empty token is a token like any other. Exit 75 (`EX_TEMPFAIL`
                  of sysexits.h) when it can't tell yet, a workload still starting: the source counts as changed, and
                  the command isn't retried. Any other exit is a failure, retried like `cmd`, after which the source
                  counts as changed too.
                '';
                default = null;
              };
            }
            // attemptsOptions "retrieve the secret";
          }
        )
      );
    };
    destinations = lib.mkOption {
      description = "List of commands to run when the form is submitted.";
      type = lib.types.listOf (
        lib.types.submodule {
          options = {
            enable = lib.mkOption {
              description = "Whether to enable this secret destination";
              type = lib.types.bool;
              default = true;
            };
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
            outputs = lib.mkOption {
              description = ''
                What the command writes, as `<kind>:<id>` with a kind from `outputKinds`: `file:/etc/secrets.d/key`,
                `kube:homepage/sonarr-api-key`. setup-secrets records the outputs it writes; `setup-secrets cleanup`
                lists those no destination declares any more and `--delete` deletes them. With `--changed-only` a
                destination whose output is gone runs again.
              '';
              type = lib.types.listOf (lib.types.strMatching "[^:]+:.+");
              default = [ ];
            };
            group = lib.mkOption {
              description = "The group the destination is in, which decides the unit that runs it (see `groups`).";
              type = lib.types.str;
              default = "host";
            };
            cmd = lib.mkOption {
              type = lib.types.str;
              description = "Command to create or update the secret";
            };
          }
          // attemptsOptions "create or update the secret";
        }
      );
    };
    outputKinds = lib.mkOption {
      description = ''
        How to check and delete each kind of output, given its id as `$OUTPUT`. `exists` exits 0 when the output is
        there and 1 when it isn't; anything else is an error. `file` is built in.
      '';
      type = lib.types.attrsOf (
        lib.types.submodule {
          options = {
            exists = lib.mkOption {
              description = "Command that checks whether `$OUTPUT` exists.";
              type = lib.types.str;
            };
            delete = lib.mkOption {
              description = "Command that deletes `$OUTPUT`.";
              type = lib.types.str;
            };
          };
        }
      );
      default = { };
    };
    stateFile = lib.mkOption {
      description = "Where setup-secrets keeps what it wrote: names, command strings and tokens, never values.";
      type = lib.types.str;
      default = "/var/lib/setup-secrets/state.json";
    };
    script = lib.mkOption {
      description = "The generated setup-secrets script";
      type = lib.types.package;
      readOnly = true;
      default = nixos-setup-secrets-wrapper;
      defaultText = "";
    };
  };
  imports = [ (inputs.flake-parts.lib.importApply ./users.nix { inherit self inputs; }) ];
  config = lib.mkIf cfg.enable {
    setup-secrets.outputKinds.file = {
      exists = lib.mkDefault ''test -e "$OUTPUT"'';
      delete = lib.mkDefault ''${lib.getExe' pkgs.coreutils "rm"} -f -- "$OUTPUT"'';
    };
    # For the TUI and manual runs as much as for the units, which have StateDirectory
    systemd.tmpfiles.rules = [ "d ${dirOf cfg.stateFile} 0700 root root -" ];
    assertions =
      map (output: {
        # Without the context of a store path in the id, which an attribute name can't have
        assertion =
          cfg.outputKinds ? ${lib.head (lib.splitString ":" (builtins.unsafeDiscardStringContext output))};
        message = "setup-secrets: the output ${output} has a kind that setup-secrets.outputKinds doesn't define";
      }) (lib.unique (lib.concatMap ({ outputs, ... }: outputs) enabledDestinations))
      ++ lib.mapAttrsToList (logPrefix: dests: {
        # The state keeps the destinations by it
        assertion = lib.length dests == 1;
        message = "setup-secrets: ${toString (lib.length dests)} destinations have the logPrefix \"${logPrefix}\"; it has to be unique";
      }) (lib.groupBy ({ logPrefix, ... }: logPrefix) enabledDestinations)
      ++ lib.flatten (
        map (
          dest:
          map (src: {
            assertion = builtins.elem src (builtins.attrNames cfg.sources);
            message = ''
              setup-secrets.destinations with logPrefix "${dest.logPrefix}" requires a secret source that is not defined: ${src}
              A secret source need not have a command set, but it must be defined.
            '';
          }) dest.requires
        ) enabledDestinations
      )
      ++ map (group: {
        # The group is the instance of the template, and the argument of --group through %i
        assertion = builtins.match "[A-Za-z0-9_.-]+" group != null;
        message = "setup-secrets: the group name \"${group}\" may contain only letters, digits, _, . and -";
      }) groupNames
      ++ [
        {
          # Settings on the old unit, like `after`, would make a unit without ExecStart and silently drop the ordering
          assertion = !(config.systemd.services ? setup-secrets);
          message = ''
            setup-secrets.service is gone: each group of destinations has a unit of its own, setup-secrets@<group>.service
            (${
              lib.concatMapStringsSep ", " (group: "${groupUnit group}.service") groupNames
            }). Move the settings of
            systemd.services.setup-secrets to the unit of the group they are meant for.
          '';
        }
      ];
    # A template run with the group as its instance, and a drop-in per group with the settings of that group, which
    # what pulls the group in adds to (systemd.services."setup-secrets@<group>")
    systemd.services = lib.mkIf cfg.autoSetup (
      {
        "setup-secrets@" = {
          description = "Fetch & store the NixOS secrets of the group %i";
          serviceConfig = {
            Type = "oneshot";
            # Remains active: an onActivation group isn't pulled in again by a boot transaction queued while
            # sysinit.target is still pending, the others stay up with what pulled them in
            RemainAfterExit = true;
            ExecStart = "${lib.getExe cfg.script} --auto --changed-only --group %i";
            StateDirectory = "setup-secrets";
            StateDirectoryMode = "0700";
          };
        };
      }
      // lib.listToAttrs (
        map (
          group:
          lib.nameValuePair (groupUnit group) (
            lib.mkMerge [
              # Without it NixOS writes a whole unit, which systemd would take in place of the template
              { overrideStrategy = "asDropin"; }
              (
                if cfg.groups.${group}.onActivation or (group == "host") then
                  {
                    # A switch restarts sysinit-reactivation.target before it starts any other unit, and PartOf carries
                    # that restart over. Wanted, not required, so a failure holds up neither the boot nor the switch
                    partOf = [ "sysinit-reactivation.target" ];
                    wantedBy = [
                      "sysinit.target"
                      "sysinit-reactivation.target"
                    ];
                    # After the TPM, without pulling it in (systemd.special): commands may use systemd-creds with TPM2
                    after = [
                      "local-fs.target"
                      "tpm2.target"
                    ];
                    before = [
                      "sysinit.target"
                      "sysinit-reactivation.target"
                      "shutdown.target"
                    ];
                    conflicts = [ "shutdown.target" ];
                    unitConfig.DefaultDependencies = false;
                  }
                else
                  {
                    # Restarted by a switch that changes the configuration, while whatever pulled it in keeps it active
                    restartTriggers = [ cfg.script ];
                  }
              )
            ]
          )
        ) groupNames
      )
    );
  };
}
