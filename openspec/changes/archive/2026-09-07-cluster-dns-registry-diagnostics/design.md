# Design: Cluster DNS and Registry Proxy Diagnostics

## Context

The incident showed that the two CoreDNS Pods returned different answers for `registry-1.docker.io`. A Registry Proxy Pod on the Qiniu node used the cluster DNS Service, received an unreachable upstream address, and timed out. Packet capture proved that Pod traffic was correctly SNATed; the upstream did not return SYN/ACK. The registry distribution proxy logged the timeout as `manifest unknown`, so the Kubelet surfaced an incorrect `not found` reason.

Increasing `metrics-server` or `local-path-provisioner` replicas created the additional Pods but did not cause image pull failure. System-component replica count and placement must remain independent from DNS forwarding policy.

The existing system-components page applies `replicas: 2` to every `static_deployment`. Controller mode only identifies the controlling mechanism; it does not prove the component supports high availability. This blanket baseline therefore needs to be replaced independently of the image-pull root cause.

## Architecture

```text
Cluster DNS page
  -> CoreDNS DNS-policy API
  -> versioned policy record + CoreDNS ConfigMap overlay
  -> rolling restart only after validation / explicit apply
  -> CoreDNS Pods validate each configured upstream

Node Registry Mirrors page / Agent CLI
  -> Registry Proxy diagnostic API
  -> selected managed Registry Proxy Pod
  -> fixed resolver and HTTPS Registry probe
  -> structured, bounded diagnostic result
```

### Cluster DNS policy

The Manager owns one logical `ClusterDNSPolicy` record containing ordered primary/secondary upstream resolvers, optional timeout and cache TTL settings, status, last accepted ConfigMap revision, and immutable history entries. Resolver addresses must be literal IPv4/IPv6 addresses or explicitly allowed internal resolver Service addresses; they are not user-supplied hostnames. The policy contains no raw Corefile.

When no policy is active, the page reports `forward . /etc/resolv.conf` as inherited K3s behavior. Applying a policy creates a Manager-owned CoreDNS custom override ConfigMap mounted through the K3s-supported CoreDNS custom import path. The override supplies one authoritative `forward . <resolver...>` block and preserves Kubernetes, hosts, cache, health, metrics, and other K3s-owned Corefile sections. The implementation must use the active K3s CoreDNS customization mechanism confirmed from the current ConfigMap/mount configuration; it must never replace the full Corefile wholesale.

Before saving, Manager validates each resolver from every Ready CoreDNS Pod using a fixed DNS query set: `registry-1.docker.io`, `registry.k8s.io`, and a platform-owned test domain. It records answer addresses and timeout/error evidence with a strict per-probe deadline. A policy can be saved only when all configured upstreams respond successfully from every Ready CoreDNS Pod. Applying an already validated policy updates the custom override and performs a controlled CoreDNS Deployment rollout. The API waits only for configuration acceptance, while rollout health is observed asynchronously.

Rollback selects a prior immutable policy revision, validates it again against current CoreDNS Pods, and applies it as a new revision. Deleting the active platform policy removes only the Manager-owned override and restores K3s inherited forwarding; it does not modify CoreDNS component replicas or placement.

### Registry Proxy diagnostic

`POST /registry-proxies/:id/diagnose` accepts no URL, command, image, or Pod parameters. Manager resolves the Proxy's resource label and selects one Ready Pod in `kube-system`. It executes a fixed diagnostic helper available in the Registry image or a Manager-owned, non-privileged ephemeral diagnostic container. The helper may only:

1. resolve the Proxy's configured upstream registry hostname via the Pod's configured resolver;
2. probe at most the returned addresses over TCP 443 and HTTPS `/v2/` using the original host/SNI;
3. issue an unauthenticated `GET /v2/` expectation test (`401` for Docker Hub is healthy; `200` or registry-auth challenge are healthy for compliant registries).

The manager converts the helper result into a schema with resolver identity, resolved IPs, selected address results, HTTP class, elapsed time, and a classified outcome. Raw stdout/stderr is capped, redacted, and retained only as an internal debug reference; the browser and Agent response contains no command strings, credentials, headers, tokens, or arbitrary environment values.

Error classification is deterministic:

| Evidence | Classification |
|---|---|
| Resolver returns incompatible answers between Ready CoreDNS Pods | `dns_resolution_inconsistent` |
| DNS query times out/fails | `dns_resolution_failed` |
| TCP connect deadline expires | `upstream_connect_timeout` |
| TLS handshake fails after TCP succeeds | `upstream_tls_failed` |
| Registry returns 401/200/challenge | `healthy` |
| Registry returns 404 after authenticated upstream manifest request | `manifest_not_found` |
| Registry proxy does not have a Ready Pod | `proxy_not_ready` |

The diagnostic action runs only for a platform-managed Proxy and its configured registry. It cannot be used as a generic egress scanner.

### Optional Registry Proxy outbound proxy

`RegistryProxy` gains encrypted fields for `HTTPProxy`, `HTTPSProxy`, and `NoProxy`. UI responses return boolean `outbound_proxy_configured` and non-sensitive `no_proxy` only; all proxy URLs, usernames, passwords, and tokens are omitted. The Deployment receives proxy environment variables only when explicitly configured. Updating these fields performs a controlled Deployment rollout and does not change node `registries.yaml`.

Before accepting an outbound proxy URL, Manager requires `http` or `https`, no fragments or query, a hostname, bounded length, and stores it using the existing encrypted field pattern. The readiness probe remains local and does not claim the upstream is usable; the diagnostic result is the egress health source of truth.

### UI placement

`/cluster?tab=dns` renders `ClusterDNS.vue` with:

- Current forwarding mode, CoreDNS Pod locations/readiness, current policy revision and last rollout status.
- Resolver list editor with a concise validation summary before the explicit apply action.
- Probe table grouped by CoreDNS Pod and resolver, showing DNS answer differences and error category.
- Policy history and rollback controls.

`/cluster?tab=registry-mirrors` retains Proxy lifecycle management. Each Proxy gains an egress health badge, latest diagnostic time/category, a `诊断` button, and an `出网代理` configuration section in its existing modal. It does not duplicate global DNS editing.

Node pages display only read-only DNS and Registry availability summaries in a future independent change; no global DNS writes are added there.

### Component-specific safety baselines

The system-components service owns a static, versioned availability profile for every whitelisted component. A profile declares the default replica count, supported replica range, whether high availability is supported, controller modes, placement capability, and prerequisites. Components with no explicit profile do not expose replica-changing safety actions.

The initial profiles are conservative:

| Component | Default | HA support | Baseline behavior |
|---|---:|---|---|
| CoreDNS | 1 | supported | Offer two replicas only after at least two Ready, schedulable nodes pass preflight. |
| metrics-server | K3s/current manifest default | unsupported | Keep its profile default; offer rollout safeguards only. |
| local-path-provisioner | K3s/current manifest default | unsupported | Keep one active provisioner; offer rollout safeguards only. |
| Other components | profile-defined | profile-defined | No replica action without an explicit profile. |

The safe-baseline API derives the desired fields from the component profile, not a generic static Deployment default. Before increasing replicas it checks Ready/schedulable node count, resource availability, component health, and managed image-pull diagnostics. A failed preflight returns a structured blocker and makes no Deployment change. The UI shows either “高可用已支持” with prerequisites and exact effect, or “副本由 K3s 管理” with no replica action.

Existing surplus replicas are not automatically removed. For an unsupported component whose replica count differs from its profile default, the UI identifies drift and offers an explicit restore-default confirmation. This action is independent from DNS remediation.

### Agent interface

Agent capability grants add cluster-scoped read-only operations:

```text
cylism-cli dns status --output json
cylism-cli dns resolve --name registry-1.docker.io --output json
cylism-cli registry proxy-diagnose --registry docker.io --output json
```

`dns resolve` accepts a small Manager-maintained domain allowlist. `proxy-diagnose` resolves a configured Registry to a managed Proxy, executes the same bounded service as the browser, and never accepts an arbitrary target. Neither operation modifies configuration or requires approval. CoreDNS policy changes, outbound proxy changes, and Proxy restarts only use browser-admin APIs with user JWT and existing audit controls.

## Risks And Mitigations

| Risk | Mitigation |
|---|---|
| A bad DNS policy breaks cluster resolution | Validate from every Ready CoreDNS Pod before applying; use versioned rollback; only modify the Manager-owned override. |
| K3s overwrites CoreDNS assets | Use K3s custom import convention; reconcile only the Manager-owned override; never overwrite the generated base Corefile. |
| Pod exec becomes generic remote command execution | Fixed argv/helper, no user target or command fields, selected managed Pod only, bounded results, and dedicated RBAC resource name/labels. |
| Diagnostics leak credentials or proxy URLs | Redacted schemas, encrypted persistence, no headers/environment/raw command output in responses. |
| A ready Proxy is mistaken for egress health | UI keeps Kubernetes readiness and upstream diagnostic status as separate fields. |
| Changes disrupt image pulls | Configuration changes require explicit apply and controlled deployment rollout; no automatic remediation. |
| A generic baseline creates unsupported replicas | Component-specific profiles, preflight validation, unsupported-action suppression, and explicit restore-default confirmation. |

## Alternatives Considered

### Set `hostNetwork: true` for Registry Proxy

Not selected. The verified timeout also occurred from the host to the bad CDN address. Host networking reduces isolation and does not repair resolver selection or upstream routing.

### Change every node's `/etc/resolv.conf`

Not selected. It is host-specific, affects unrelated processes, and produces configuration drift. Cluster workloads should receive a consistent policy from CoreDNS.

### Let Nanobot run `curl`, `dig`, or `kubectl exec`

Not selected. It creates unrestricted network or Kubernetes execution capability. The diagnostic APIs instead expose fixed, audited probes from the correct workload context.

### Put DNS controls in the system-components modal

Not selected. That modal manages component lifecycle (replicas, rollout, placement); global external forwarding policy has separate validation, rollback, and ownership semantics.

### Keep a static-Deployment-wide two-replica baseline

Not selected. `static_deployment` is a control-source classification, not a high-availability contract. It would keep exposing unsupported replica changes for singleton components and hide the prerequisites needed by components that do support HA.
