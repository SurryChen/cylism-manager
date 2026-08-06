## Why

The managed assistant Runtime is currently installed into Kubernetes `default`, which mixes a platform-owned service with workloads that do not share its lifecycle or access boundary. The Runtime needs a dedicated namespace before later phases add more Agents, Runtimes, and a controlled tool gateway.

Kubernetes PVCs cannot cross a namespace boundary. The migration must preserve any audit data that exists in the legacy `default/cylism-ops-agent-audit` PVC before removing it, even though the currently installed volume is empty.

## What Changes

- Install and observe the managed assistant Runtime in the dedicated `cylism-assistant` namespace.
- Use the Runtime Service DNS name in `cylism-assistant` as the Manager's default Runtime endpoint.
- On a legacy installation, run a managed audit-PVC migration from `default` to `cylism-assistant`: stop the old Runtime, bind a target PVC, copy and verify the audit data, start and verify the target Runtime, then remove the legacy Deployment, Service, ConfigMap, Secret, and source PVC.
- Support subsequent controlled Runtime moves between nodes after the namespace migration, reusing the same local-volume transfer engine and preserving the active audit database.
- Keep a normal Runtime uninstall unchanged: it removes workload configuration but retains the current audit PVC.

## Capabilities

### New Capabilities

- `assistant-runtime-namespace`: Reconcile the managed assistant Runtime in its own namespace and safely clean up the one-time legacy installation.

### Modified Capabilities

- None.

## Impact

The Manager Kubernetes reconciler, existing local-PVC migration transfer primitives, migration state persistence, Runtime status API, default in-cluster service discovery, focused Kubernetes tests, and assistant API tests are affected. The existing Runtime deployment UI remains the migration trigger and gains an explicit node-migration action with progress. No Runtime application code or provider data changes are required.
