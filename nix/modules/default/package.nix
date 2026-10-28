{ buildGo126Module }:
buildGo126Module {
  name = "nixos-setup-secrets";
  # version = "0.7.0";
  meta.mainProgram = "nixos-setup-secrets";
  src = ./src;
  proxyVendor = true;
  vendorHash = "sha256-5QrZ2zInsQs5mFL7QVV57PtaEutc+Ut5eIG6CmGkDgU=";
}
