# The units: the host group runs at boot before sysinit.target and again on every switch, the cluster group follows
# a stand-in for the service it talks to. With --changed-only an unchanged destination is skipped, one whose output is
# gone runs again; cleanup finds what a removed destination left behind
{ self, pkgs }:
pkgs.testers.runNixOSTest {
  name = "setup-secrets-units";
  nodes.machine =
    {
      config,
      lib,
      pkgs,
      ...
    }:
    let
      # Every fetch prints a new value, so a destination's file shows whether the group ran again
      fresh = "${lib.getExe' pkgs.coreutils "date"} +%s%N";
      write = file: var: ''printf '%s' "''$${var}" >${file}'';
      hostFile = {
        logPrefix = "Host file";
        requires = [ "HOST_SECRET" ];
        cmd = write "/run/host-secret" "HOST_SECRET";
      };
      clusterFile = {
        logPrefix = "Cluster file";
        group = "cluster";
        requires = [ "CLUSTER_SECRET" ];
        cmd = write "/run/cluster-secret" "CLUSTER_SECRET";
      };
    in
    {
      imports = [ self.nixosModules.default ];
      environment.systemPackages = [ config.setup-secrets.script ];
      setup-secrets = {
        enable = true;
        autoSetup = true;
        sources.HOST_SECRET.cmd = fresh;
        # Read back from where its destination writes it, with a token that changes with the file
        sources.STABLE_SECRET = {
          cmd = "cat /var/lib/stable 2>/dev/null || echo first";
          same = "stat -c %Y.%s /var/lib/stable 2>/dev/null || echo none";
          storedIn = "/var/lib/stable";
        };
        sources.CLUSTER_SECRET = {
          # Fails while the stand-in is down
          cmd = "${lib.getExe' pkgs.systemd "systemctl"} is-active --quiet stand-in.service && ${fresh}";
          retries = 2;
        };
        destinations = [
          hostFile
          {
            logPrefix = "Stable file";
            requires = [ "STABLE_SECRET" ];
            cmd = write "/var/lib/stable" "STABLE_SECRET";
            outputs = [ "file:/var/lib/stable" ];
          }
          clusterFile
        ];
      };
      systemd.services.stand-in = {
        wantedBy = [ "multi-user.target" ];
        serviceConfig.ExecStart = "${lib.getExe' pkgs.coreutils "sleep"} infinity";
      };
      # What a consumer like the k3s module does with its group's unit
      systemd.services."setup-secrets@cluster" = {
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
      # Without the destination of the stable file, which leaves its output behind
      specialisation.without-stable.configuration.setup-secrets.destinations = lib.mkForce [
        hostFile
        clusterFile
      ];
    };
  testScript = ''
    def ts(unit, prop):
        return int(machine.succeed(f"systemctl show -P {prop} {unit}").strip())

    def journal(unit):
        return machine.succeed(f"journalctl -b -o cat -u {unit}")

    machine.wait_for_unit("multi-user.target")

    with subtest("boot: the host group runs before sysinit.target, the cluster group after the stand-in"):
        assert ts("setup-secrets@host", "ExecMainExitTimestampMonotonic") < ts("sysinit.target", "ActiveEnterTimestampMonotonic")
        machine.succeed("test -s /run/host-secret")
        runs = journal("setup-secrets@host").count("Host file: Storing\n")
        assert runs == 1, f"the host group ran {runs} times at boot"
        machine.wait_for_file("/run/cluster-secret", timeout=60)
        assert ts("setup-secrets@cluster", "ExecMainStartTimestampMonotonic") > ts("stand-in", "ActiveEnterTimestampMonotonic")
        machine.succeed("systemctl is-active setup-secrets@cluster")
        machine.succeed("grep -q '\"file:/var/lib/stable\"' /var/lib/setup-secrets/state.json")

    with subtest("switch: the host group runs again, skipping what didn't change; the unchanged cluster group doesn't"):
        host, cluster = machine.succeed("cat /run/host-secret"), machine.succeed("cat /run/cluster-secret")
        machine.succeed("/run/booted-system/specialisation/unrelated/bin/switch-to-configuration test")
        assert machine.succeed("cat /run/host-secret") != host, "the host group didn't run on the switch"
        assert machine.succeed("cat /run/cluster-secret") == cluster, "the cluster group ran though nothing changed"
        assert journal("setup-secrets@host").count("Stable file: Unchanged\n") == 1, journal("setup-secrets@host")

    with subtest("a destination whose output is gone runs again"):
        machine.succeed("rm /var/lib/stable")
        machine.succeed("systemctl restart setup-secrets@host")
        machine.succeed("test -s /var/lib/stable")
        assert "Stable file: Runs, file:/var/lib/stable is gone" in journal("setup-secrets@host"), journal("setup-secrets@host")

    with subtest("switch: a changed cluster group runs again"):
        machine.succeed("/run/booted-system/specialisation/cluster-changed/bin/switch-to-configuration test")
        machine.succeed("test -s /run/cluster-secret-2")
        assert machine.succeed("cat /run/cluster-secret") != cluster

    with subtest("restarting the stand-in runs the cluster group again, stopping it stops the group"):
        cluster = machine.succeed("cat /run/cluster-secret")
        machine.succeed("systemctl restart stand-in")
        machine.wait_until_succeeds(f"test \"$(cat /run/cluster-secret)\" != '{cluster}'", timeout=30)
        machine.succeed("systemctl stop stand-in")
        machine.fail("systemctl is-active setup-secrets@cluster")

    with subtest("cleanup lists what a removed destination wrote, and deletes it with --delete"):
        machine.succeed("/run/booted-system/specialisation/without-stable/bin/switch-to-configuration test")
        listed = machine.succeed("setup-secrets cleanup")
        assert "file:/var/lib/stable" in listed and "written by Stable file" in listed, listed
        machine.succeed("test -e /var/lib/stable")
        machine.succeed("setup-secrets cleanup --delete")
        machine.fail("test -e /var/lib/stable")
        assert "still declared" in machine.succeed("setup-secrets cleanup")
  '';
}
