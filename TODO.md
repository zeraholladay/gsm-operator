## TODO

Last updated: 2025-12-23

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
- [x] Add predicates to ignore status-only updates to avoid immediate self-triggered reconciles (status update currently causes an extra reconcile independent of the 5m RequeueAfter)
- [x] Debounce/ignore reconcile triggers from owned Secret updates when no data changes (e.g., compare before Update or add predicate) to avoid double runs on rollout
- [x] Update Helm example to reflect latest operator config/flags
- [x] Add architecture flow diagram
- [x] Make resync interval configurable (currently hardcoded to 5 minutes)
- [x] Configurable logging levels.
- [ ] Trusted subsystem mode: use the identity of the operator.
- [ ] Support Secrets in JSON format
