## 1. Contract and persistence

- [x] 1.1 Extend `ManagedOCIRegistry` model, store methods and migration coverage with PVC name, StorageClass, requested capacity and CPU/memory request/limit fields while retaining ownership links to ImageRegistry and NodeRegistryMirror.
- [x] 1.2 Replace hostPath request validation with data-node, `local-path` StorageClass/`WaitForFirstConsumer`, PVC capacity and Kubernetes resource-quantity validation.
- [x] 1.3 Add store and handler tests proving credentials are encrypted/redacted, resource configuration is persisted, and conflicting non-managed records are not overwritten.

## 2. Kubernetes lifecycle

- [x] 2.1 Replace hostPath renderer coverage with a labeled `local-path` PVC, PVC-backed Deployment, submitted container resources, Recreate strategy, ClusterIP Service, Basic Auth Secret and host-based Ingress.
- [x] 2.2 Implement PVC creation/reuse checks, storage-class preflight, reconcile and status reporting with bounded, redacted diagnostics.
- [x] 2.3 Update explicit deletion checks and behavior to preserve the PVC while blocking active Release/Project references.

## 3. Node distribution integration

- [x] 3.1 Add managed ownership fields/helpers for ImageRegistry and NodeRegistryMirror integration without changing unrelated external Registry records.
- [x] 3.2 Implement selected-node access application through the existing node registry configuration flow, including HTTPS/HTTP transport rendering and per-node status persistence.
- [x] 3.3 Add API tests for node selection, partial failure, HTTP confirmation, credential redaction and collision handling.

## 4. REST API and delivery UI

- [x] 4.1 Extend authenticated managed Registry REST endpoints and audit entries with storage-class preflight, PVC state and resource configuration.
- [x] 4.2 Update the self-hosted Registry view with editable CPU/memory configuration, selected data node, read-only `local-path` storage class, PVC capacity/state, transport risk, readiness, node application progress, create/update and deletion confirmation states.
- [x] 4.3 Extend Vue component and routing tests for storage preflight, defaults, resource validation, PVC state, normal, loading, insecure, degraded and API error states.
- [x] 4.4 Replace manual TLS Secret entry with matching platform Certificate selection and validate Certificate readiness, namespace and hostname coverage.
- [x] 4.5 Fix Registry namespace to `cylism-system`, select an existing eligible `local-path` PVC, and stop creating or modifying PVC resources during Registry reconciliation.
- [x] 4.6 Add Registry-to-certificate/PVC management entry points that open prefilled creation forms for the fixed namespace and endpoint/storage context.

## 5. Verification

- [x] 5.1 Run focused backend and frontend tests for every completed task, then run `go test ./...`, `go build ./...`, frontend tests and production frontend build.
- [x] 5.2 Validate the OpenSpec change strictly, run `git diff --check`, and submit required sec-code scans for every modified business code file.
