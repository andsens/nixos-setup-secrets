# setup-secrets

- `proxyVendor = true` (`nix/modules/default/default.nix`) and no CI. If
  Dependabot is added, its `vendorHash` needs the hash of a `go mod download`
  cache, unlike k8sss, which hashes `vendor/`.
