## Changelog

### 2025-12-20

- Initial working "hack" verion:
  - The code just functions at the most basic level.
  - The code was tested with the most trival use-case.
- Initial logging added to:
  - `internal/controller/gsm_secret.go`
  - `internal/controller/gsmsecret_controller.go`
  - `internal/controller/k8s_secret.go`
  - `internal/controller/service_accounts.go`
- Added `TODO.md` with high-level roadmap items (testing, `gsm-reader` configurability, SA impersonation, status/conditions, WIF audience behavior, metrics, and docs).
