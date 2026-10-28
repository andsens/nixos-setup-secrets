# setup-secrets

- `proxyVendor = true` (`nix/modules/default/package.nix`), so `vendorHash` is
  the hash of a `go mod download` cache, unlike k8sss, which hashes `vendor/`.
  `.github/workflows/pins.yaml` recomputes it on Dependabot's gomod branches.
