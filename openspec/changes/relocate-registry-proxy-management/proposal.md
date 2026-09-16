## Why

Registry Proxy is a deployable image distribution service with an upstream, endpoint, cache and operational diagnostics. Keeping it on the node registry mirrors page conflates service lifecycle with the independent task of generating and distributing K3s node configuration, making both workflows difficult to scan and operate.

## What Changes

- Move Registry Proxy lifecycle management from the cluster's node registry mirrors view to the delivery center's artifact registry workspace.
- Organize the delivery workspace into distinct self-hosted OCI Registry and Registry Proxy sections without changing their independent APIs or stored configuration.
- Reduce the node registry mirrors view to K3s `registries.yaml` rules, endpoint verification and selected-node application results.
- Retain the explicit relationship: a ready Registry Proxy endpoint may be entered into a same-registry node mirror rule, but proxy changes never automatically modify node configuration.

## Capabilities

### New Capabilities

- `node-registry-mirror-management`: Manage desired K3s registry mirror rules separately from Registry Proxy workload lifecycle.

### Modified Capabilities

- `registry-proxy`: Move Registry Proxy discovery and lifecycle operations to the delivery artifact registry workspace.

## Impact

- Updates `ManagedOCIRegistries.vue`, `NodeRegistryMirrors.vue`, their focused view tests, navigation copy, and the Registry Proxy API module boundary.
- Reuses the existing Registry Proxy REST endpoints, Kubernetes resources, persistence, diagnostics and audit events; no data migration or new dependency is required.
