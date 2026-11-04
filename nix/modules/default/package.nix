{ buildGo126Module }:
buildGo126Module {
  name = "nixos-setup-secrets";
  # version = "0.7.0";
  meta.mainProgram = "nixos-setup-secrets";
  src = ./src;
  proxyVendor = true;
  vendorHash = "sha256-aSArZqy0Sgav/mdCAPs+8fb5XRGsPFG/2ciwJUPYetw=";
}
