{ self, ... }:
{
  config,
  pkgs,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.setup-secrets;
  users = lib.filterAttrs (
    user: spec: spec.enable && spec ? hashedPasswordFile && spec.hashedPasswordFile != null
  ) config.users.users;
in
{
  options.setup-secrets.users = {
    enable = lib.mkEnableOption "management of user hashed password files through setup-secrets";
  };
  config = lib.mkIf (cfg.enable && cfg.users.enable) {
    setup-secrets = {
      sources = lib.mapAttrs' (
        user: spec:
        lib.nameValuePair "USER_PW_${user}" {
          description = "Password for ${user}";
        }
      ) users;
      destinations = lib.mapAttrsToList (user: spec: {
        logPrefix = "Password for ${user}";
        requires = [ "USER_PW_${user}" ];
        cmd = lib.getExe (
          pkgs.writeShellScriptBin "set-pwhash.sh" ''
            umask 077
            printf "%s" "$USER_PW_${user}" | mkpasswd --stdin > "${spec.hashedPasswordFile}"
          ''
        );
      }) (users);
    };
  };
}
