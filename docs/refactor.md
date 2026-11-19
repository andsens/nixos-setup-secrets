# Refactor: units, cleanup, no-op runs, a table TUI

## Motivation

setup-secrets runs `--auto` as an activation script on every switch and once more as a oneshot service at boot. On
a machine running k3s, a switch that restarts k3s runs the activation script while the API server is down, so every
source and destination that talks to the cluster fails, and nothing repairs that until the next boot. At boot the
service starts after k3s but before the workloads it reads from, so sources that `kubectl exec` into a pod fail, and
the fixed 5 s timeout kills `kubectl apply` calls on a busy node.

setup-secrets also keeps no record of what it wrote. A destination that is disabled or removed leaves its file or
Secret behind, and nothing finds it again. Every run fetches and stores everything, even when nothing changed.

## Goal

setup-secrets runs as systemd units: a host group on every activation and at boot ahead of the services that read
host secrets, and a cluster group that k3s pulls in and that waits for what it reads. It remembers what it wrote
(names and tokens, never values), so it can list and delete outputs no destination declares any more, and skip
secrets whose source and destinations haven't changed. A table TUI replaces the form. The Go code sits where a Go
project keeps it.

## Decisions

- Order: units, then cleanup, then the TUI. A web frontend is in the backlog; how it would log in is undecided.
- Units: a `group` on destinations; a source runs in every group that has a destination needing it. The host group
  is wanted by `sysinit.target` and `sysinit-reactivation.target` and ordered before both, so it runs at boot before
  the host secret consumers and again on every switch. The cluster group runs from a unit that k3s pulls in (`WantedBy`, `PartOf`, `After`,
  `RemainAfterExit`), so every start of k3s re-runs it. Sources and destinations take a timeout in place of the
  fixed 5 s and a number of retries with backoff. Waiting for a workload to roll out before exec-ing into it is part
  of the source command, which the consumer helpers write.
- Cleanup: destinations declare their outputs (a file path, a kube `namespace/name`, a credstore name). Kube Secrets
  written through the helpers are labeled `app.kubernetes.io/managed-by: setup-secrets`. An output is old when it
  exists in a managed place and no enabled destination declares it. `cleanup` lists them and `cleanup --delete`
  removes them. It never runs on boot or switch: a module enabled again would get a newly generated value, a
  database password for instance, that no longer matches what the service has.
- No-op runs, opt-in:
  - a source may have a `same` command that prints a cheap token without fetching the value (a file mtime, the
    `resourceVersion` of a Secret; its uid stays the same across `kubectl apply`);
  - the key stored per source is its `cmd` string plus that token. `cmd` is usually a store path, so it changes
    whenever the script does, and needs no hash;
  - each destination stores its `cmd` and the tokens of what it requires, and runs again when either changed or a
    declared output is gone;
  - tokens are taken after storing, since a destination that writes what its source reads changes the token;
  - a source without `same` always runs; a run skips unchanged secrets only with `--changed-only`, which the units
    pass;
  - the state file holds names, command strings and tokens, never values or hashes of values. Cleanup uses the same
    file.
- TUI, after k9s: a split pane with secrets on the left and destinations on the right. Selecting a row highlights
  the rows linked to it on the other side and dims the rest; tab switches panes. Describe, per-row retry, a dry run
  view, logs and an orphans view.
- Editing in the TUI:
  - `e` edits a secret; destinations aren't edited;
  - only round-trip and manual sources are editable. A round-trip source reads back an output that one of its own
    destinations writes, named by a new source field `storedIn`, so an edit sticks. A manual source has no `cmd`. An
    upstream source reads from somewhere setup-secrets never writes; it is read-only, as an edit would be fetched
    over on the next run;
  - edits are live: saving fetches the other values its destinations need again and runs those destinations. A
    destination that needs two edited values holds a mismatched pair between the two saves; there is no staging.
- Layout: the Go module at the repository root, the binary under `cmd/`, packages under `internal/`.

## Steps

1. Layout: the Go module to the repository root, the package source and Dependabot pointed at it; fix the
   assertion message, which read the nonexistent `logEntry`.
2. Groups and units, timeouts and retries, with a VM test: a switch re-runs the host unit, the cluster
   unit follows a stand-in for k3s.
3. Consumers move to the groups (their helpers set `group` and `storedIn`).
4. The state file under `/var/lib/setup-secrets`, declared outputs, `cleanup` and `--delete`.
5. `same` on sources and `--changed-only`.
6. The TUI.
7. Deploy and check: a switch that restarts k3s stores the cluster secrets after it, a boot stores everything, a
   switch with no changes skips everything.
