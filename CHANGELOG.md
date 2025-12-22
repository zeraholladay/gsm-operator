## Changelog

### 2025-12-21

- Refactored GSMSecret spec to move KSA/GSA/WIF audience into annotations and aligned controller logic.
- Tightened schema validation (patterns for targetSecret.name, key, projectId, secretId, version) and updated CRD.
- Expanded test coverage for schema constraints, annotation precedence, and controller behaviors.
- Updated docs and sample manifests to use the new annotations.
- General bug fixes and refactor cleanups.

### 2025-12-20

- Initial working "hack" verion:
  - The code just functions at the most basic level.
  - The code was tested with the most trival use-case.
- Initial logging added to:
  - `internal/controller/gsm_secret.go`
  - `internal/controller/gsmsecret_controller.go`
  - `internal/controller/k8s_secret.go`
  - `internal/controller/service_accounts.go`
- Added `TODO.md` with high-level roadmap items (testing, `default` configurability, SA impersonation, status/conditions, WIF audience behavior, metrics, and docs).
