# Test values: with `testing`, a source with `test` fetches it in place of `cmd`, one without `cmd` too, one that turns
# its own `testing` off keeps its `cmd`, and one without `test` fetches for real. Without `testing` nothing changes
{ self, pkgs }:
pkgs.testers.runNixOSTest {
  name = "setup-secrets-testing";
  nodes.machine =
    { config, lib, ... }:
    let
      write = name: {
        logPrefix = name;
        requires = [ name ];
        cmd = ''printf '%s' "''$${name}" >/run/${name}'';
      };
    in
    {
      imports = [ self.nixosModules.default ];
      environment.systemPackages = [ config.setup-secrets.script ];
      setup-secrets = {
        enable = true;
        testing = true;
        sources = {
          # A service the test doesn't have
          API_KEY = {
            cmd = "exit 1";
            test = "echo test-api-key";
          };
          # Typed in on a real machine
          PASSWORD.test = "echo test-password";
          # The test wants the real command
          KEPT = {
            cmd = "echo real-kept";
            test = "echo test-kept";
            testing = false;
          };
          GENERATED.cmd = "echo real-generated";
        };
        destinations = map write [
          "API_KEY"
          "PASSWORD"
          "KEPT"
          "GENERATED"
        ];
      };
      specialisation.production.configuration.setup-secrets.testing = lib.mkForce false;
    };
  testScript = ''
    names = ["API_KEY", "PASSWORD", "KEPT", "GENERATED"]

    machine.wait_for_unit("multi-user.target")

    with subtest("testing: test values in place of cmd, also without one, but not where a source keeps its cmd"):
        machine.succeed("setup-secrets --auto")
        got = {name: machine.succeed(f"cat /run/{name}") for name in names}
        want = {"API_KEY": "test-api-key", "PASSWORD": "test-password", "KEPT": "real-kept", "GENERATED": "real-generated"}
        assert got == want, got

    with subtest("without testing: the real commands, and nothing for the source without cmd"):
        machine.succeed("${pkgs.coreutils}/bin/unlink /run/API_KEY")
        machine.succeed("${pkgs.coreutils}/bin/unlink /run/PASSWORD")
        machine.succeed("/run/booted-system/specialisation/production/bin/switch-to-configuration test")
        status, out = machine.execute("/run/current-system/sw/bin/setup-secrets --auto 2>&1")
        assert status == 1, out
        machine.fail("test -e /run/API_KEY")
        machine.fail("test -e /run/PASSWORD")
  '';
}
