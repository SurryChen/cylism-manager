## 1. VictoriaMetrics PVC Storage

- [x] 1.1 Add failing Kubernetes client tests for PVC-backed VictoriaMetrics creation, node selection, validation, legacy hostPath recognition and same-node migration resources.
- [x] 1.2 Implement VictoriaMetrics storage modes, system-owned PVC reconciliation and a stop/copy/verify/cutover/recovery migration workflow for legacy hostPath installations.
- [x] 1.3 Update monitoring install/settings and migration APIs with tests for capacity, StorageClass, legacy status, stage diagnostics and rollback behavior.

## 2. Infrastructure PVC Inventory

- [x] 2.1 Add failing PVC inventory and handler tests for Alertmanager/VictoriaMetrics ownership classification and protected generic operations.
- [x] 2.2 Implement derived infrastructure ownership metadata, Alertmanager label reconciliation and server-side rejection for every generic PVC mutation.
- [x] 2.3 Update storage inventory API responses and filter data to distinguish application, infrastructure and external claims.

## 3. Monitoring and Storage UI

- [x] 3.1 Add Monitoring view tests for PVC capacity/StorageClass configuration, legacy hostPath migration confirmation/progress, success and recovery states.
- [x] 3.2 Implement VictoriaMetrics PVC installation/settings controls, migration workflow and actionable status messaging.
- [x] 3.3 Add PersistentVolumes view tests and implement infrastructure ownership display, filtering and owner-specific navigation without generic mutation actions.

## 4. Verification

- [x] 4.1 Run focused Go and Vue tests for each completed task, followed by `go test ./...`, `go build ./...`, frontend tests and production build.
- [ ] 4.2 Run `openspec validate infrastructure-pvc-management --strict`, `git diff --check`, and the required security scan for every modified business-code file.
