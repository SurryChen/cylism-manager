## 1. Contract and persistence

- [x] 1.1 Add `ManagedOCIRegistry` model, store methods and migration coverage, including ownership links to ImageRegistry and NodeRegistryMirror.
- [x] 1.2 Add request validation for endpoint authority, transport choice, TLS Secret, data path, data node, resource naming and explicit HTTP confirmation.
- [x] 1.3 Add store and handler tests proving credentials are encrypted/redacted and conflicting non-managed records are not overwritten.

## 2. Kubernetes lifecycle

- [x] 2.1 Add Kubernetes resource renderer tests for the labeled Registry Deployment, hostPath volume, Recreate strategy, ClusterIP Service, Basic Auth Secret and host-based Ingress.
- [x] 2.2 Implement create, reconcile, status and endpoint-health operations with bounded, redacted diagnostics.
- [x] 2.3 Implement explicit deletion checks and deletion behavior that preserves the hostPath data directory and blocks active Release/Project references.

## 3. Node distribution integration

- [x] 3.1 Add managed ownership fields/helpers for ImageRegistry and NodeRegistryMirror integration without changing unrelated external Registry records.
- [x] 3.2 Implement selected-node access application through the existing node registry configuration flow, including HTTPS/HTTP transport rendering and per-node status persistence.
- [x] 3.3 Add API tests for node selection, partial failure, HTTP confirmation, credential redaction and collision handling.

## 4. REST API and delivery UI

- [x] 4.1 Add authenticated managed Registry REST endpoints and audit entries for create, update, apply-node-access, inspect and delete operations.
- [x] 4.2 Add Delivery Center navigation and a Registry view with endpoint, data node, transport risk, readiness, node application progress, create/update and deletion confirmation states.
- [x] 4.3 Add Vue component and routing tests for normal, loading, insecure, degraded and API error states.

## 5. Verification

- [ ] 5.1 Run focused backend and frontend tests for every completed task, then run `go test ./...`, `go build ./...`, frontend tests and production frontend build.
- [ ] 5.2 Validate the OpenSpec change strictly, run `git diff --check`, and submit required sec-code scans for every modified business code file.
