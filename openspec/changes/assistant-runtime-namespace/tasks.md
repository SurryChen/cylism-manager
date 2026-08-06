## 1. Namespace Reconciliation and State

- [x] 1.1 Add failing Kubernetes tests for creating the dedicated assistant namespace and reconciling all Runtime resources there.
- [x] 1.2 Replace the Runtime's `default` namespace constant with `cylism-assistant` and ensure the namespace before resource reconciliation.
- [x] 1.3 Update Runtime status, active uninstall behavior, PVC infrastructure discovery tests, and Manager assistant API tests to use the new namespace.
- [x] 1.4 Add a Runtime-specific migration model, persistence methods, and database migration for durable migration stage, source/target identity, node names, bytes copied, diagnostics, and timestamps.

## 2. Audit Data Migration and Cutover

- [x] 2.1 Add failing tests around extracted local-volume transfer primitives for infrastructure callers, including namespace-independent path preflight and target-PVC binding.
- [x] 2.2 Refactor the existing Storage PVC migration's controlled SSH `tar` transfer, byte accounting, and local-path validation into an internal reusable component without changing its application migration behavior.
- [ ] 2.3 Add failing Runtime migration tests for preflight, source shutdown, data-copy progression, verification failure, restart/resume state, and rollback before cutover.
- [x] 2.4 Implement the Runtime-specific cross-namespace workflow using the shared transfer component and audit SQLite checksum verification.
- [x] 2.5 Add failing tests for target readiness followed by idempotent deletion of every known legacy resource, including the source PVC and temporary binding Pod.
- [x] 2.6 Implement post-readiness cleanup and source Runtime restoration for pre-cutover failure paths.

## 3. Service Discovery

- [x] 3.1 Add handler tests for default target service discovery, legacy routing during migration, and preserving an explicit URL override.
- [x] 3.2 Change the Manager's default in-cluster Runtime URL to the dedicated namespace after a verified cutover.
- [x] 3.3 Expose persisted Runtime migration progress through the assistant status API and existing deployment UI.

## 4. Subsequent Node Migration

- [x] 4.1 Add failing tests for rejecting an ordinary deploy/update node change while an active local audit PVC is bound elsewhere.
- [x] 4.2 Make the Runtime reconcile, status, provider update, and uninstall paths derive and preserve the active audit PVC name from the Deployment.
- [x] 4.3 Add a Runtime node-migration API and UI action with target-node validation and duplicate-migration protection.
- [x] 4.4 Implement same-namespace target-PVC creation with a generated name, data transfer, Deployment claim/node cutover, readiness validation, rollback, and source-PVC cleanup.
- [x] 4.5 Add success, copy/verification failure, source restoration, and persistence-resume tests for subsequent node migrations.

## 5. Verification

- [x] 5.1 Run focused `internal/k8s` and `internal/api` tests.
- [x] 5.2 Run `gofmt`, `go test ./...`, and `go build ./...` for the Manager.
- [x] 5.3 Run the required security scan submission for each modified business code file and report any unavailable scanner transparently. (Scanner binary was unavailable on the host.)
