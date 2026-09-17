## Context

The current node registry mirrors page contains two unrelated operational surfaces: desired K3s `/etc/rancher/k3s/registries.yaml` rules and managed `registry:2` pull-through Proxy workloads. A proxy has its own lifecycle, endpoint, cache, outbound connectivity and diagnostics. A mirror rule selects endpoints and is later applied to selected nodes. The existing backend deliberately does not automatically connect the two.

The delivery center already owns the self-hosted OCI Registry page at `/delivery/registry`, including its configuration and image catalog. It is the appropriate workspace for image distribution services.

## Goals / Non-Goals

**Goals:**

- Make all platform-managed image distribution services discoverable from the delivery center.
- Keep node registry mirrors focused on desired configuration, verification and node application.
- Preserve all Registry Proxy actions and existing API contracts.
- Make it clear that applying a mirror changes the complete enabled K3s registry configuration on selected nodes.

**Non-Goals:**

- Do not implement observed `registries.yaml` collection, drift detection, rollout history, canary rollout, or a change to the node-side apply protocol.
- Do not automatically create, update, or apply a node mirror when a Proxy is created or edited.
- Do not merge the self-hosted OCI Registry and Registry Proxy data models; they have different storage, deployment and security contracts.
- Do not alter Proxy Kubernetes resources, NodePort allocation, cache semantics, or diagnostic behavior.

## Decisions

### 1. Delivery owns image distribution workloads

`/delivery/registry` will retain its `制品库` title and gain top-level workspace tabs for `自托管制品库` and `Registry Proxy`. The self-hosted workspace combines operational status and image browsing into one continuous view: the compact status strip is always visible, while the repository and Tag workspace follows it directly. Registry configuration remains a modal workflow rather than an expanded page section. Registry Proxy gets a first-class list with deploy, configure, diagnose, DNS, cache-cleanup and legacy-resource-migration controls.

This keeps each workflow shallow: delivery has one workspace-tab layer only. The active Proxy workspace is URL-addressable with `?tab=registry-proxy` so a direct link returns to the correct screen.

### 2. Nodes own only desired runtime configuration

The cluster page retains `节点镜像源`. It lists the desired Registry host, endpoint set, verification state and the last selected-node application result. Proxy cards, proxy forms and Proxy-specific errors are removed. The page describes the actual unit of application: all enabled mirror rules are rendered into one K3s configuration and distributed only to selected nodes.

When creating or editing a mirror rule, its endpoint remains free-form so external and cloud Registry endpoints continue to work. The endpoint form provides an available managed Proxy reference only as contextual information, rather than coupling records or silently copying values.

### 3. Preserve APIs and separate frontend boundaries

Existing `/registry-proxies` endpoints and payloads remain unchanged. Proxy API functions move out of the node-mirror client module into a dedicated `registry-proxies` client module owned by the delivery view. Node mirror API functions remain in `node-registry-mirrors`.

This is a frontend boundary migration only. Existing users, stored proxy records and deep links to `/cluster?tab=registry-mirrors` stay valid. The cluster tab no longer loads Proxy data.

## Alternatives Considered

### Keep Proxy as a subsection on the node configuration page

This preserves the old route but continues to put a deployable delivery service ahead of the node configuration it merely supplies. It is not chosen.

### Treat every Proxy as an automatically managed mirror rule

Automatic rule creation would make endpoint changes unexpectedly trigger K3s restarts and cannot represent external endpoints or per-node rollout choice. It is not chosen.

### Create a separate delivery route for Proxy

The delivery center currently has one artifact registry workspace. Adding another route and sidebar item would fragment closely related image distribution operations before their scale justifies it. It is not chosen.

## Risks / Trade-offs

- [The self-hosted Registry page becomes too dense] -> keep only health, address and storage in the persistent status strip; place detailed configuration in its existing modal and make image browsing the primary page body.
- [Existing users look for Proxy in cluster] -> node mirrors includes a concise contextual link to the delivery Proxy workspace; existing cluster deep links remain valid for mirror configuration.
- [API imports break during movement] -> migrate and test named API imports in one change; endpoint URLs and request payloads remain unchanged.
- [A proxy is mistaken for automatic node configuration] -> preserve explicit endpoint use and explain that Proxy deployment does not write or restart K3s nodes.

## Migration Plan

1. Add the delivery Proxy workspace and migrate existing Proxy controls without changing server endpoints or data.
2. Remove Proxy state and controls from node mirrors; retain all mirror interactions and add the cross-workspace link.
3. Run focused UI/API tests, full frontend tests and build, then verify Go tests/build remain green because no backend behavior changes.
4. Rollback restores the original frontend composition only; no stored data must be reverted.
