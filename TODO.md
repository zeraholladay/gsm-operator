## TODO

Last updated: 2025-12-20

- [x] Refactor into class structure: code quality
- [x] wifAudience should be configurable on the operator as an envvar.
- [ ] Add comprehensive testing for the controller and helper packages
- [x] Make `gsm-reader` ServiceAccount name configurable (remove hardcoding)
- [ ] Implement Service Account impersonation support in `GCPCredsFromK8sToken`
- [ ] Implement status updates for `ObservedGeneration` and `Conditions` (`Ready`, `Progressing`, `Degraded`)
- [ ] Improve error handling and requeue semantics (distinguish transient vs permanent errors)
- [x] Define and implement configuration/defaulting behavior for `spec.wifAudience`
- [ ] Add metrics for reconcile duration, error counts, and STS/token operations
- [x] Add manifests and documentation for `gsm-reader` ServiceAccount, RBAC, and IAM bindings

- [x] Validate that `keyedSecretPayload` keys used in `buildOpaqueSecret` are valid environment variable names

