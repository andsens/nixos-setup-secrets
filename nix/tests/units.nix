# The units: the host group runs at boot before sysinit.target and again on every switch, the cluster group follows
# a stand-in for the service it talks to
{ self, pkgs }:
pkgs.testers.runNixOSTest {
  name = "setup-secrets-units";
  nodes.machine =
    { lib, pkgs, ... }:
    let
      # Every fetch prints a new value, so a destination's file shows whether the group ran again
      fresh = "${lib.getExe' pkgs.coreutils "date"} +%s%N";
      write = file: var: ''printf '%s' "''$${var}" >${file}'';
    in
    {
      imports = [ self.nixosModules.default ];
      setup-secrets = {
        enable = true;
        autoSetup = true;
        sources.HOST_SECRET.cmd = fresh;
        sources.CLUSTER_SECRET = {
          # Fails while the stand-in is down
          cmd = "${lib.getExe' pkgs.systemd "systemctl"} is-active --quiet stand-in.service && ${fresh}";
          retries = 2;
        };
        destinations = [
          {
            logPrefix = "Host file";
            requires = [ "HOST_SECRET" ];
            cmd = write "/run/host-secret" "HOST_SECRET";
          }
          {
            logPrefix = "Cluster file";
            group = "cluster";
            requires = [ "CLUSTER_SECRET" ];
            cmd = write "/run/cluster-secret" "CLUSTER_SECRET";
          }
        ];
      };
      systemd.services.stand-in = {
        wantedBy = [ "multi-user.target" ];
        serviceConfig.ExecStart = "${lib.getExe' pkgs.coreutils "sleep"} infinity";
      };
      # What a consumer like the k3s module does with its group's unit
      systemd.services.setup-secrets-cluster = {
        wantedBy = [ "stand-in.service" ];
        partOf = [ "stand-in.service" ];
        after = [ "stand-in.service" ];
      };
      # A switch that changes nothing setup-secrets sees
      specialisation.unrelated.configuration.environment.etc."unrelated".text = "x";
      # A switch that changes the cluster group
      specialisation.cluster-changed.configuration.setup-secrets.destinations = [
        {
          logPrefix = "Second cluster file";
          group = "cluster";
          requires = [ "CLUSTER_SECRET" ];
          cmd = write "/run/cluster-secret-2" "CLUSTER_SECRET";
        }
      ];
    };
  testScript = ''
    def ts(unit, prop):
        return int(machine.succeed(f"systemctl show -P {prop} {unit}").strip())

    machine.wait_for_unit("multi-user.target")

    with subtest("boot: the host group runs before sysinit.target, the cluster group after the stand-in"):
        assert ts("setup-secrets-host", "ExecMainExitTimestampMonotonic") < ts("sysinit.target", "ActiveEnterTimestampMonotonic")
        machine.succeed("test -s /run/host-secret")
        runs = machine.succeed("journalctl -b -o cat -u setup-secrets-host | grep -c '^Host file: Storing$'").strip()
        assert runs == "1", f"the host group ran {runs} times at boot"
        machine.wait_for_file("/run/cluster-secret", timeout=60)
        assert ts("setup-secrets-cluster", "ExecMainStartTimestampMonotonic") > ts("stand-in", "ActiveEnterTimestampMonotonic")
        machine.succeed("systemctl is-active setup-secrets-cluster")

    with subtest("switch: the host group runs again, the unchanged cluster group doesn't"):
        host, cluster = machine.succeed("cat /run/host-secret"), machine.succeed("cat /run/cluster-secret")
        machine.succeed("/run/booted-system/specialisation/unrelated/bin/switch-to-configuration test")
        assert machine.succeed("cat /run/host-secret") != host, "the host group didn't run on the switch"
        assert machine.succeed("cat /run/cluster-secret") == cluster, "the cluster group ran though nothing changed"

    with subtest("switch: a changed cluster group runs again"):
        machine.succeed("/run/booted-system/specialisation/cluster-changed/bin/switch-to-configuration test")
        machine.succeed("test -s /run/cluster-secret-2")
        assert machine.succeed("cat /run/cluster-secret") != cluster

    with subtest("restarting the stand-in runs the cluster group again, stopping it stops the group"):
        cluster = machine.succeed("cat /run/cluster-secret")
        machine.succeed("systemctl restart stand-in")
        machine.wait_until_succeeds(f"test \"$(cat /run/cluster-secret)\" != '{cluster}'", timeout=30)
        machine.succeed("systemctl stop stand-in")
        machine.fail("systemctl is-active setup-secrets-cluster")
  '';
}
