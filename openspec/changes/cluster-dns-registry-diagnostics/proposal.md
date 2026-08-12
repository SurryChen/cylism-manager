# Cluster DNS and Registry Proxy Diagnostics

## Why

CoreDNS forwards external lookups to each node's `/etc/resolv.conf`. In a multi-node cluster those resolvers can return different CDN addresses for the same registry. Some addresses may be unreachable from the node hosting a managed Registry Proxy. The proxy then reports an upstream timeout as `manifest unknown`, which makes a reachable image appear to be missing.

The current console shows only Registry Proxy readiness and NodePort availability. It cannot show the DNS result and HTTPS reachability from the workload that actually proxies image requests, nor can it manage a cluster-wide external DNS policy. CoreDNS placement and replicas are already managed as system-component concerns, but external resolution policy is a separate cluster networking concern.

## What Changes

- Add a `集群 DNS` tab under the Cluster hub for platform-managed CoreDNS external upstream configuration, health, validation history, and rollback.
- Preserve the existing system-components CoreDNS controls for replicas, rollout policy, and node placement; the new DNS page owns only external forwarding policy.
- Add Registry Proxy diagnostics executed in the selected Proxy Pod context, returning bounded DNS, TCP/TLS, and Registry `/v2/` evidence with explicit error categories.
- Add optional, encrypted outbound proxy settings (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) to each managed Registry Proxy. Values remain redacted in API responses and UI.
- Extend the Node Registry Mirrors page with Proxy diagnostics and concise egress status, without treating NodePort readiness as upstream availability.
- Replace the blanket static-system-component safety baseline with component-specific availability profiles that state replica support, placement constraints, and preconditions before a replica increase.
- Extend the controlled Agent CLI and Nanobot operation catalog with read-only `dns status`, allowlisted `dns resolve`, and `registry proxy-diagnose` operations. CoreDNS changes and Proxy redeploys remain browser-admin actions.

## Capabilities

### New Capabilities

- `cluster-dns-management`: Platform-managed CoreDNS external upstream policy, validation, history, and rollback.
- `registry-proxy-diagnostics`: Workload-context diagnosis and optional outbound proxy configuration for managed Registry Proxy instances.

### Modified Capabilities

- `registry-proxy`: Managed Registry Proxies expose workload-context DNS and upstream egress health separately from Kubernetes readiness.
- `agent-platform-capabilities`: Runtimes can request bounded, read-only DNS and Registry Proxy diagnostic evidence when explicitly granted.
- `system-components`: Safety baselines are selected by explicit component availability profiles rather than controller mode alone.
- `ui-navigation`: Cluster Hub exposes a dedicated Cluster DNS tab.

## Non-Goals

- Do not provide arbitrary CoreDNS Corefile editing, arbitrary resolver domains, shell access, `kubectl exec`, or arbitrary Pod network probing.
- Do not change CoreDNS replicas, rollout strategy, or scheduling from the DNS page.
- Do not expose Registry credentials, outbound proxy credentials, node SSH credentials, resolver configuration from arbitrary nodes, or raw unbounded command output.
- Do not automatically change DNS, restart CoreDNS, change proxy configuration, or rewrite workload image references after a diagnostic result.
- Do not make `hostNetwork` the default for Registry Proxies.
- Do not infer high-availability support from `static_deployment`, or silently increase replicas for unsupported components.

## Impact

- Adds CoreDNS ConfigMap service/API, persistent versioned configuration and audit history, plus controlled CoreDNS rollout after an accepted policy change.
- Extends Registry Proxy model, deployment rendering, API, Agent services, CLI schemas, Nanobot catalog, and Registry Mirrors UI.
- Extends system-component metadata, safety-baseline validation, API responses, and UI to make replica support and prerequisites explicit.
- Requires Manager Kubernetes permissions to read/update the `kube-system/coredns` ConfigMap, list CoreDNS/Registry Proxy Pods, retrieve bounded Pod logs, and use the Kubernetes Pod exec subresource only for the fixed diagnostic command.
