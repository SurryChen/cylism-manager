## Context

The application already manages workloads, storage, certificates, registries, and Kubernetes resources through standard `client-go` APIs. Its deployment image nevertheless installs the Tailscale CLI, its Helm and static manifests always mount `/run/tailscale`, and `/api/tailscale/*` can operate on the host Tailscale daemon. The K3s node-join workflow also installs and registers Tailscale before running the normal `k3s-agent` installer.

Those behaviors are independent of generic Kubernetes management. Separately, the server network diagnostic already notices active K3s `vpn-auth` settings, but labels all positive detections as embedded Tailscale and probes every server with Tailscale commands. The platform does not currently expose its Kubernetes distribution to the UI, so K3s-specific actions cannot be reliably gated.

## Goals / Non-Goals

**Goals:**

- Establish Kubernetes API compatibility as the default product boundary.
- Identify K3s from safe Kubernetes discovery metadata and expose it as an API contract for the UI.
- Retain only read-only, K3s-scoped VPN compatibility visibility when active K3s configuration declares VPN integration.
- Delete platform-level Tailscale administration, host socket access, Tailscale node bootstrap, and stored reusable Auth Keys.
- Keep K3s worker bootstrap available through a user-provided reachable control-plane address, without assuming a particular overlay network.

**Non-Goals:**

- Configure, install, authenticate, or upgrade Tailscale on any host.
- Change K3s `vpn-auth`, `vpn-auth-file`, routing, firewall, CNI, or node networking.
- Provide worker bootstrap for arbitrary Kubernetes distributions.
- Implement a new VPN provider abstraction or support arbitrary provider-specific health checks.
- Automatically preserve or export obsolete Tailscale Auth Keys.

## Decisions

### 1. Use Kubernetes discovery as the source of platform identity

The backend will query the Kubernetes server version through a narrow discovery adapter. A successful server version containing the K3s distribution marker such as `+k3s` is reported as `k3s`; another successful Kubernetes server version is reported as `kubernetes`; an unavailable or unparsable discovery response is reported as `unknown` with a sanitized availability reason.

The response exposes only distribution, normalized Git version, and capability flags. It does not expose kubeconfig data, control-plane addresses, or discovery errors containing credentials. The frontend consumes this response rather than inferring platform type from node labels, a hostname, a Tailscale address, or an SSH command.

Alternative considered: infer K3s from a reachable host's `k3s` systemd unit. This fails for remote or partially managed clusters and makes the generic cluster identity depend on SSH access. Alternative considered: let users choose the type manually. That creates a stale, misleading configuration and does not gate unsafe actions reliably.

### 2. K3s node join uses direct reachability and is capability-gated

The worker join operation retains its fixed SSH preflight, then runs the normal K3s agent installer with the configured control-plane address and encrypted K3s join token. It no longer invokes `tailscale`, downloads installation scripts, or reads `tailscale_auth_key`.

The API and UI require the discovered platform to be K3s before exposing or starting the workflow. The control-plane address is the existing operator-managed address; it may happen to be a Tailnet address, a private address, or a DNS name, but the product does not classify or configure it.

Alternative considered: retain an optional Tailscale checkbox in node join. That would reintroduce an account credential, host mutation, and an implicit network dependency into the generic workflow. It is intentionally excluded.

### 3. Diagnose only active K3s VPN configuration

The server diagnostic first finds an active `k3s` or `k3s-agent` unit. Only then does it test for the presence of supported K3s VPN markers in that unit or standard K3s configuration locations. It returns a boolean integration state, active unit, and a provider classification limited to `tailscale`, `other`, or `unknown`; raw option values, configuration contents, file paths that reveal private topology, and credentials are never returned.

When no active K3s VPN marker exists, the response contains no Tailscale status, Tailnet IP, NAT, DERP, or peer-path data and the frontend shows no Tailscale-specific table fields. No `tailscale` command is invoked in this path. The diagnostic remains read-only and only K3s-aware; it is not a VPN health-control API.

Alternative considered: keep collecting Tailnet status as an optional generic diagnostic. That preserves the host CLI dependency and would continue to make a network product part of Cylism's baseline. Alternative considered: identify provider by returning `vpn-auth` values. Those values can contain join credentials and must not cross the SSH/API boundary.

### 4. Delete rather than deprecate host-level Tailscale APIs

The three `/api/tailscale/*` routes, handler, runtime service, bootstrap wiring, route assertions, and Tailscale-specific audit classification will be removed in the same release. The Docker image will no longer install the CLI and manifests will not mount `/run/tailscale`.

This is intentionally a breaking removal. Leaving a compatibility route that controls the local host would retain an unnecessary privilege boundary and make it difficult to guarantee generic deployment. A startup migration deletes the exact `tailscale_auth_key` system-config entry without reading or logging its decrypted value. The migration is idempotent and is recorded without secret material.

### 5. Make generic Kubernetes deployment the default

The Helm Chart uses a PVC-based SQLite data configuration by default and does not declare a hostPath for Tailscale or a control-plane-specific node selector. A clearly named K3s/local-host example may remain only where a behavior genuinely requires it, but it is not a default deployment path. The stale static manifest is brought into line with the chart or removed if it duplicates unsupported defaults.

Alternative considered: make socket mounting a disabled-by-default Helm option. Since no remaining product capability needs the socket, retaining the option expands the deployment privilege surface without value.

## Risks / Trade-offs

- **Existing automation calls removed APIs** -> Document the breaking removal in README, release notes, installation, and troubleshooting pages; route golden tests ensure the endpoints are absent.
- **A custom K3s build does not expose the usual version marker** -> Report `unknown` instead of assuming Kubernetes or enabling K3s mutation; operators retain generic read-only and resource management capabilities.
- **VPN marker formats differ across K3s versions** -> Match only known option and configuration-key presence, return `unknown` provider when safe classification is unavailable, and add fixture tests for accepted formats.
- **Deleting a saved Auth Key is irreversible** -> Delete only the exact obsolete system-config key, never log its value, declare the migration in release documentation, and make the operation idempotent.
- **PVC support is not installed in an operator's cluster** -> Helm install surfaces the normal PVC scheduling failure; documentation states that a default StorageClass or explicitly configured storage is required.

## Migration Plan

1. Deploy the version with the database cleanup migration and removed Tailscale runtime API.
2. The startup migration deletes only `tailscale_auth_key`; no other system configuration or server credential is changed.
3. Existing server records remain reachable through their saved `host` management address. Any historical Tailscale metadata becomes unused and is excluded from API responses.
4. Operators with Tailscale-backed reachability can continue using that address, but must maintain it outside Cylism.
5. Upgrade Helm values by removing `tailscale.hostPath`; move local SQLite data to the documented PVC path before changing an existing installation. Rollback to the previous application version is possible only before the obsolete Auth Key deletion; the old key must be supplied again externally after a rollback.

## Open Questions

- None. K3s VPN configuration is deliberately detection-only in this change; a future mutating K3s VPN setup workflow requires a separate approved design.
