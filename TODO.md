## TODO

Last updated: 2025-12-20

- [ ] Refactor into class structure: code quality
- [ ] wifAudience should be configurable on the operator as an envvar.
- [ ] Add comprehensive testing for the controller and helper packages
- [ ] Make `gsm-reader` ServiceAccount name configurable (remove hardcoding)
- [ ] Implement Service Account impersonation support in `GCPCredsFromK8sToken`
- [ ] Implement status updates for `ObservedGeneration` and `Conditions` (`Ready`, `Progressing`, `Degraded`)
- [ ] Improve error handling and requeue semantics (distinguish transient vs permanent errors)
- [ ] Define and implement configuration/defaulting behavior for `spec.wifAudience`
- [ ] Add metrics for reconcile duration, error counts, and STS/token operations
- [ ] Add manifests and documentation for `gsm-reader` ServiceAccount, RBAC, and IAM bindings

- [ ] Validate that `keyedSecretPayload` keys used in `buildOpaqueSecret` are valid environment variable names

