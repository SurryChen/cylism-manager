## Context

`cylism-ops-agent` has Deployment, Service, ConfigMap, Secret, and audit PVC resources hard-coded into `default`. The Manager invokes it through `cylism-ops-agent.default.svc`. Kubernetes resources cannot change namespaces in place, and the audit PVC cannot be mounted from a different namespace.

The current legacy audit PVC is empty, but the migration must not encode that as a destructive assumption. Future installations and upgrades can have audit records, so it needs the same controlled local-volume data migration principles used elsewhere in the platform.

## Goals / Non-Goals

**Goals:**

- Establish `cylism-assistant` as the ownership and security boundary for the managed assistant Runtime.
- Make installation and status checks use that namespace consistently.
- Copy and verify audit storage before the target Runtime consumes it.
- Clean up the legacy `default` resources and source PVC only after the target Runtime is ready.
- Persist migration progress so a Manager restart does not lose the cutover state.
- Keep the reconcile path idempotent and tolerate an already-removed legacy installation.

**Non-Goals:**

- Moving the Manager, monitoring, or application workloads out of their existing namespaces.
- Moving an arbitrary PVC, copying non-audit application data, or providing a generic cross-namespace PVC migration UI.
- Changing normal Runtime uninstall semantics or deleting the active Runtime audit PVC.
- Introducing a generic namespace selection UI.

## Decisions

### Use a dedicated `cylism-assistant` namespace

The Runtime is a platform-managed assistant service, not a monitoring component. `cylism-assistant` separates its RBAC, future NetworkPolicy, resource quotas, and multi-Runtime evolution from both `default` and `monitoring`.

Alternative: place it in `monitoring`. Rejected because monitoring's lifecycle, access needs, and failure domain are unrelated to an Agent Runtime.

### Migrate audit storage through a controlled, persisted workflow

When the legacy `default/cylism-ops-agent-audit` PVC exists, the deploy/update operation starts or resumes a Runtime-specific migration record rather than treating the move as a simple reconcile. The workflow is:

1. Preflight the legacy PVC as a platform-managed local-path/hostPath volume and resolve its bound node and local path. The source and requested target nodes must be ready, registered platform servers using SSH-key authentication, and able to run the fixed `tar` transfer commands with non-interactive `sudo`.
2. Create the target `cylism-assistant` namespace and target audit PVC, then bind it on the selected Runtime node with a short-lived, platform-owned binding Pod. Resolve and preflight the target local path.
3. Scale the legacy Runtime Deployment to zero and wait until its Pods exit, giving the SQLite audit database a consistent filesystem view.
4. Stream an archive from the source path to the target path over the same controlled SSH transport used by local PVC migration. Record the byte count and verify each existing audit SQLite file (`audit.db`, `audit.db-wal`, and `audit.db-shm`) by checksum after the copy.
5. Create or update the target Secret, ConfigMap, Service, and Deployment, then wait for the target Runtime to be ready.
6. Delete only the known legacy Runtime Deployment, Service, ConfigMap, Secret, temporary binding Pod, and `default/cylism-ops-agent-audit` source PVC.

The migration state includes stage, source and target PVC identity, nodes, copied-byte count, diagnostic detail, and timestamps. The explicit deploy/update action starts the workflow; subsequent deploy/update requests resume an incomplete workflow. Runtime status reports the stage but does not itself create or delete resources.

The source PVC is deleted only after checksum verification and target readiness. `NotFound` is an idempotent terminal state for legacy cleanup resources. If a pre-cutover failure occurs, the target temporary resources are removed and the source Deployment is restored. If a failure occurs after source shutdown, the stored migration state makes the recovery or retry action explicit rather than deleting source data.

Alternative: mount both PVCs in a copy Job. Rejected because Kubernetes PVC references are namespace-scoped, so a Job cannot mount one claim from `default` and one from `cylism-assistant`.

### Reuse the local PVC migration engine below the application layer

The existing Storage PVC migration feature already provides the correct transfer mechanics: local-path and node preflight, target-PVC binding, controlled SSH-key authentication, fixed quoted `tar` transfer commands, byte accounting, and failure recovery conventions. This change SHALL extract or generalize those mechanics into an internal transfer component and invoke it from the Runtime migration workflow.

The existing `PersistentVolumeMigration` HTTP API, persistence record, and application cutover logic are not reused directly. They require an Environment and Application, reject infrastructure PVCs, and rewrite application templates and Deployment PVC references. Supplying synthetic environment or application records would compromise ownership checks and cause the wrong workload mutation. Runtime migration instead owns its cross-namespace resource lifecycle while delegating the host-path transfer to the shared component.

### Keep service routing on the legacy Runtime until cutover

For the default in-cluster endpoint only, the Manager resolves the legacy Runtime Service while a recorded migration has not reached target-ready. After target readiness and source cleanup, it uses `cylism-ops-agent.cylism-assistant.svc`. An explicit `CYLISM_ASSISTANT_RUNTIME_URL` remains authoritative and is never overwritten by this migration.

This limits the unavailable window to the consistency stop-and-copy step instead of routing all requests to a target Runtime before it is ready.

### Delete only the known legacy audit PVC after verified cutover

The migration deletes precisely `default/cylism-ops-agent-audit`, along with the other named legacy Runtime resources. It does not list or delete arbitrary PVCs, and it does not delete `cylism-assistant/cylism-ops-agent-audit` on normal uninstall.

### Support subsequent Runtime moves between nodes

After the one-time namespace migration, an operator may explicitly migrate a ready Runtime to another ready node. This is not performed by changing the node selection in the ordinary deploy/update form: a local-path PVC remains bound to its original node, and simply changing the Deployment node selector would strand the Pod.

The migration workflow reuses the same persisted Runtime migration record, target-PVC binding, source shutdown, controlled SSH `tar` transfer, checksum verification, target readiness check, and rollback semantics. The source and target both reside in `cylism-assistant` for this path.

Because a PVC cannot be renamed, the target uses a bounded, generated name such as `cylism-ops-agent-audit-migrate-<id>`. During cutover, the Runtime Deployment is updated to reference this target claim and the selected node. The active audit PVC name is derived from the Deployment volume rather than assumed to be the original fixed name. Provider reconciliation, Runtime status, normal deploy/update, and uninstall consequently preserve whichever claim is active.

After the target Runtime is ready, the system deletes only the recorded source PVC. It retains the source PVC and restores the source Deployment if pre-cutover transfer or verification fails. A failed migration never causes the ordinary deploy/update operation to silently select a different node.

An explicit `POST /api/assistant/runtime/migrations` action receives the target node. It requires a ready Runtime, a different ready target node, and platform-managed SSH-key server registrations for both nodes. The UI presents it as a separate Runtime storage action, not as an incidental deployment setting.

### Update the Manager's default Runtime service discovery

When `CYLISM_ASSISTANT_RUNTIME_URL` is unset, the Manager uses `http://cylism-ops-agent.cylism-assistant.svc:8080`. An explicit environment value continues to override this default for nonstandard deployments.

The Runtime-to-Manager callback remains pointed at the Manager's existing in-cluster Service name; relocating the Manager is outside this change.

## Migration Plan

1. Deploy the Manager release containing the namespace migration.
2. Select **Deploy or update Runtime**. The Manager detects the legacy audit PVC and starts or resumes the persisted migration.
3. Observe the Runtime migration status until it reports completion. The platform copies and validates audit data, switches to the target Runtime, and removes the old named resources and source PVC.
4. Verify `default/cylism-ops-agent*` resources no longer exist, the target Runtime is ready in `cylism-assistant`, and the audit database exists on its target PVC.

## Risks / Trade-offs

- [The source or target node lacks controlled SSH migration prerequisites] -> The migration fails before source shutdown and retains the legacy Runtime and PVC; the operator corrects server registration or credentials and retries.
- [Copy or checksum verification fails] -> The source PVC is retained and the legacy Deployment is restored before cutover; target temporary resources are cleaned up where safe.
- [The Manager restarts during migration] -> Persistent stage data permits an explicit deploy/update retry to resume or recover rather than repeating an unsafe deletion.
- [A custom Runtime URL is configured] -> The environment override remains authoritative; its owner must update it if it points to the legacy service.

## Open Questions

- None. The legacy audit PVC is confirmed empty and should be removed after the target Runtime is ready.
