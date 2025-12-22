## TODO

Last updated: 2025-12-21

- [x] Validate key format: Each key must consist of alphanumeric characters, '-', '_' or '.'.
- [x] Add info to Secret if problems.
- [x] Refactor into class structure: code quality
- [x] wifAudience should be configurable on the operator as an envvar.
- [x] Add comprehensive testing for the controller and helper packages
- [x] Make `default` ServiceAccount name configurable (remove hardcoding)
- [x] Implement Service Account impersonation support
- [x] Implement status updates for `ObservedGeneration` and `Conditions` (`Ready`, `Progressing`, `Degraded`)
- [ ] Improve error handling and requeue semantics (distinguish transient vs permanent errors)
- [x] Define and implement configuration/defaulting behavior for `spec.wifAudience`
- [ ] Add metrics for reconcile duration, error counts, and STS/token operations
- [x] Add manifests and documentation for `default` ServiceAccount, RBAC, and IAM bindings
- [x] Validate that `keyedSecretPayload` keys used in `buildOpaqueSecret` are valid environment variable names

