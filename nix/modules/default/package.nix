{ buildGo126Module }:
buildGo126Module {
  name = "nixos-setup-secrets";
  # version = "0.7.0";
  meta.mainProgram = "nixos-setup-secrets";
  src = ./src;
  proxyVendor = true;
  vendorHash = "sha256-OkAoJ3ysAK9tW69S7HeYPnhrkQL9JgHKHOQoYwQL6yc=";
}
