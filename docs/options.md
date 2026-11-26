## setup-secrets\.enable



Whether to enable the module: its assertions, and with ` autoSetup ` its service and activation script\. It doesn’t install the CLI, which is ` setup-secrets.script `\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.autoSetup

Whether to enable automatic fetching \& storing of secrets: one instance of ` setup-secrets@.service ` per group, ` setup-secrets@<group>.service `\. A group with
` onActivation ` runs at boot before ` sysinit.target ` and again on every switch; the other groups are left to be
pulled in by what they depend on\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations



List of commands to run when the form is submitted\.



*Type:*
list of (submodule)

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.enable



Whether to enable this secret destination



*Type:*
boolean



*Default:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.cmd



Command to create or update the secret



*Type:*
string

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.group



The group the destination is in, which decides the unit that runs it (see ` groups `)\.



*Type:*
string



*Default:*

```nix
"host"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.logPrefix



A short description of the destination\.



*Type:*
string



*Default:*

```nix
""
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.outputs



What the command writes, as ` <kind>:<id> ` with a kind from ` outputKinds `: ` file:/etc/secrets.d/key `,
` kube:homepage/sonarr-api-key `\. setup-secrets records the outputs it writes; ` setup-secrets cleanup `
lists those no destination declares any more and ` --delete ` deletes them\. With ` --changed-only ` a
destination whose output is gone runs again\.



*Type:*
list of string matching the pattern \[^:]+:\.+



*Default:*

```nix
[ ]
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.requires



List of secrets this command requires in order to execute



*Type:*
list of string



*Default:*

```nix
[ ]
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.retries



How often a failed command to create or update the secret is tried again, after 1s, 2s, 4s and so on up to 30s between tries\.



*Type:*
unsigned integer, meaning >=0



*Default:*

```nix
0
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.timeout



How long the command to create or update the secret may run, as a Go duration (` 30s `, ` 2m `)\. A timeout counts as a failed try\.



*Type:*
string



*Default:*

```nix
"30s"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.destinations\.\*\.wants



List of secrets this command wants (but not requires)



*Type:*
list of string



*Default:*

```nix
[ ]
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.groups



Settings of the groups the destinations are in\. A group exists as soon as a destination is in it, so it needs
an entry here only to change its settings\.



*Type:*
attribute set of (submodule)



*Default:*

```nix
{ }
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.groups\.\<name>\.onActivation



Whether the group runs at boot, ordered before ` sysinit.target `, and on every switch, before the switch
starts or restarts any other unit (through ` sysinit-reactivation.target `)\. Its unit has no default
dependencies, so its commands can’t count on the network or other services\. Without it the group’s
unit runs only when something pulls it in, e\.g\. ` wantedBy ` and ` partOf ` the service its commands talk
to, and again whenever its configuration changes while it is active\.



*Type:*
boolean



*Default:*

```nix
name == "host"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.outputKinds



How to check and delete each kind of output, given its id as ` $OUTPUT `\. ` exists ` exits 0 when the output is
there and 1 when it isn’t; anything else is an error\. ` file ` is built in\.



*Type:*
attribute set of (submodule)



*Default:*

```nix
{ }
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.outputKinds\.\<name>\.delete



Command that deletes ` $OUTPUT `\.



*Type:*
string

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.outputKinds\.\<name>\.exists



Command that checks whether ` $OUTPUT ` exists\.



*Type:*
string

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.script



The generated setup-secrets script



*Type:*
package *(read only)*



*Default:*

```nix
""
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources



Map of secrets\. \<name> becomes the secret name\.



*Type:*
attribute set of (submodule)

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.enable



Whether to enable this secret source



*Type:*
boolean



*Default:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.cmd



Command to retrieve the secret



*Type:*
null or string



*Default:*

```nix
null
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.description



The description of the secret as displayed in the form\. Defaults to \<name>



*Type:*
string



*Default:*

```nix
"‹name›"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.retries



How often a failed command to retrieve the secret is tried again, after 1s, 2s, 4s and so on up to 30s between tries\.



*Type:*
unsigned integer, meaning >=0



*Default:*

```nix
0
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.same



Command that prints a cheap token that changes whenever the value does, without fetching the value: a
file’s mtime, a Kubernetes Secret’s resourceVersion\. With ` --changed-only `, which the units pass, a
destination runs only when the command or token of one of its secrets, or its own command, changed
since it last stored, or when one of its ` outputs ` is gone\. A source without it always counts as
changed\.

Exit 0 with the token on stdout; an empty token is a token like any other\. Exit 75 (` EX_TEMPFAIL `
of sysexits\.h) when it can’t tell yet, a workload still starting: the source counts as changed, and
the command isn’t retried\. Any other exit is a failure, retried like ` cmd `, after which the source
counts as changed too\.



*Type:*
null or string



*Default:*

```nix
null
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.storedIn



Where a destination of this secret stores the value that ` cmd ` reads back, as shown in the form
(` kube cert-manager/inwx-credentials `, ` /etc/secrets.d/restic.key `)\. Set, the value can be edited in
the form and the edit sticks\. Without it, a source with ` cmd ` reads its value from somewhere
setup-secrets never writes, so the form shows it read-only\. A source without ` cmd ` is always
editable\.



*Type:*
null or string



*Default:*

```nix
null
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.test



Command that prints a stand-in for the secret, fetched in place of ` cmd ` where ` testing ` is on: for a
secret from a service or account a test doesn’t have\. Also for a source without ` cmd `, which then
needs no one to type it in\. A source that a test can fetch for real (a generated value, a Secret in
the test’s own cluster) needs none\.



*Type:*
null or string



*Default:*

```nix
null
```



*Example:*

```nix
"echo test-api-key"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.testing



Whether this source fetches its ` test ` value in place of ` cmd `\. Set it to keep the real ` cmd ` in a test\.



*Type:*
boolean



*Default:*

```nix
config.setup-secrets.testing
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.sources\.\<name>\.timeout



How long the command to retrieve the secret may run, as a Go duration (` 30s `, ` 2m `)\. A timeout counts as a failed try\.



*Type:*
string



*Default:*

```nix
"30s"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.stateFile



Where setup-secrets keeps what it wrote: names, command strings and tokens, never values\.



*Type:*
string



*Default:*

```nix
"/var/lib/setup-secrets/state.json"
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.testing



Whether to enable the sources’ test values: each source with a ` test ` fetches it in place of ` cmd `, unless its own ` testing ` is
off\. For NixOS tests and test clusters, where the services a source reaches don’t exist\. It is built into the
script, so a machine without it never writes a test value\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/default\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/default.nix)



## setup-secrets\.users\.enable



Whether to enable management of user hashed password files through setup-secrets\.



*Type:*
boolean



*Default:*

```nix
false
```



*Example:*

```nix
true
```

*Declared by:*
 - [nix/modules/default/users\.nix](https://github.com/andsens/nixos-setup-secrets/blob/main/nix/modules/default/users.nix)


