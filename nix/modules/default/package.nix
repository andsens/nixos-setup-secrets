{ buildGo126Module, lib }:
buildGo126Module {
  name = "nixos-setup-secrets";
  # version = "0.7.0";
  meta.mainProgram = "nixos-setup-secrets";
  # Only the Go sources, so a change to the Nix files or docs doesn't rebuild the binary
  src = lib.fileset.toSource {
    root = ../../..;
    fileset = lib.fileset.unions [
      ../../../go.mod
      ../../../go.sum
      ../../../cmd
      ../../../internal
    ];
  };
  proxyVendor = true;
  vendorHash = "sha256-HQckOp36RIbbmcGOQiosRdEU/BSxwq93yII9/2lB9bQ=";
}
