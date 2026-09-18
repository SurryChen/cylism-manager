## Why

Cylism Manager currently treats Tailscale as a platform prerequisite even though its Kubernetes resource management uses standard Kubernetes APIs. This prevents deployment on ordinary Kubernetes clusters and makes K3s-specific node operations, K3s VPN configuration, and host-level Tailscale administration indistinguishable.

## What Changes

- Add an authenticated cluster-identity capability that reports the connected platform as Kubernetes, K3s, or unknown from Kubernetes discovery information.
- Make K3s node bootstrap an explicit K3s-only workflow using the configured reachable control-plane address; remove all Tailscale installation, registration, and Auth Key use from node join.
- Replace Tailscale network diagnostics with a read-only K3s VPN compatibility diagnostic. It performs K3s-specific presentation only when an active K3s configuration contains a VPN marker and never reads or returns credentials.
- **BREAKING** Remove the platform-level `/api/tailscale/init`, `/api/tailscale/status`, and `/api/tailscale/install-script` APIs, their runtime service, and persisted `tailscale_auth_key` support.
- Remove the Tailscale CLI and `/run/tailscale` host socket from the container image, Helm Chart, and static deployment manifest. Make the published deployment path Kubernetes-compatible by default.
- Remove Tailnet discovery/import as a server-management workflow; registered hosts use an operator-provided reachable management address.
- Update product, installation, security, and troubleshooting documentation so Tailscale is optional K3s VPN compatibility rather than a product requirement.

## Capabilities

### New Capabilities

- `cluster-platform-identification`: Safely identify the connected Kubernetes distribution and provide feature availability to the UI.

### Modified Capabilities

- `k3s-node`: Restrict node bootstrap to K3s and remove Tailscale-dependent joining and node mapping.
- `k3s-platform`: Treat Kubernetes as the deployment baseline and K3s as an optional compatibility profile.
- `server-management`: Remove Tailscale metadata and Tailnet import from managed-server behavior.
- `tailnet-discovery`: Remove host-socket Tailnet discovery and reusable Tailscale Auth Key behavior.
- `tailscale-network-diagnostics`: Replace Tailnet probing with read-only K3s VPN compatibility detection.
- `public-documentation-site`: Remove Tailscale socket and tailnet prerequisites from supported public deployment documentation.

## Impact

- Backend: Kubernetes discovery adapter, cluster-status API, K3s join workflow, network diagnostic collector, router/bootstrap composition, system configuration cleanup, and related Go tests.
- Frontend: cluster identity display, K3s-only action gating, and simplified server network diagnostics.
- Deployment: Dockerfile, Helm values/templates, static Kubernetes manifest, and documentation no longer require a Tailscale binary or host socket.
- Existing clients of `/api/tailscale/*` must stop calling those removed endpoints. Existing encrypted `tailscale_auth_key` records are deleted by the application migration and cannot be recovered through the application afterward.
