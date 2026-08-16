# nix-setup-secrets

A NixOS module and CLI for setting up secrets on a machine without ever
storing them -- encrypted or not -- in the Nix store or your git repo.

Instead of encrypting secrets into your flake the way
[sops-nix](https://github.com/Mic92/sops-nix) or
[agenix](https://github.com/ryantm/agenix) do, you declare *sources* (where
a secret's value comes from) and *destinations* (what to do with it once
you have it). Running `setup-secrets` fetches whatever sources have a fetch
command, lets you review and fill in the rest in a terminal form, and then
runs your destination commands with the values as environment variables --
so a secret only ever exists on the target machine, for as long as it takes
to hand it off to wherever it's actually needed.

## Setup

Add `nix-setup-secrets` to your `flake.nix` and import the module:

```nix
{
  inputs = {
    ...
    setup-secrets = {
      url = "github:andsens/nix-setup-secrets";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    ...
  };
  ...
}
```

```nix
{ inputs, ... }:
{
  imports = [ inputs.setup-secrets.nixosModules.default ];
  config.setup-secrets.enable = true;
}
```

## Declaring secrets

```nix
{
  setup-secrets = {
    sources.DB_PASSWORD.description = "Database password";
    destinations = [
      {
        logPrefix = "App database password";
        requires = [ "DB_PASSWORD" ];
        cmd = ''printf '%s' "$DB_PASSWORD" >/run/secrets/db-password'';
      }
    ];
  };
}
```

A source with no `cmd` has to be filled in by hand in the form. Give it one
and it's fetched automatically instead -- from a password manager CLI, a
KMS, wherever:

```nix
setup-secrets.sources.DB_PASSWORD.cmd = "op read op://vault/db/password";
```

A destination's `cmd` runs with every secret listed in `requires` available
as an environment variable named after the source; if any required secret
has no value, the destination is skipped. `wants` secrets are injected the
same way when available, but don't block the destination if they're
missing.

## Running it

```
$ setup-secrets
```

Opens a terminal form: sources with a `cmd` are fetched and pre-filled,
everything else is left for you to type in. Hit "Save" to run the
destinations.

```
$ setup-secrets --auto
```

Skips the form entirely -- fetches and stores in one pass. Turn on
`setup-secrets.autoSetup` to also run this automatically, both as a
`nixos-rebuild switch` activation script and as a `setup-secrets.service`
unit at boot.

## User passwords

`setup-secrets.users.enable` wires up a source and destination for every
user with a `hashedPasswordFile` set: it prompts for their password in the
form and writes the hashed result to that file, so the plaintext password
never touches disk.

```nix
{
  setup-secrets.users.enable = true;
  users.users.anders.hashedPasswordFile = "/persist/passwd/anders";
}
```

For the full list of module options, see [docs/options.md](docs/options.md).
