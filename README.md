<div align="center">

# Cylism Manager

**Kubernetes-compatible infrastructure control plane for operators who manage servers, clusters, delivery, and certificates together.**

[![Build](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml/badge.svg?branch=main)](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml)
[![Docs](https://img.shields.io/badge/docs-GitHub%20Pages-222?logo=github)](https://surrychen.github.io/cylism-manager/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[Documentation](https://surrychen.github.io/cylism-manager/) · [Get started](docs/getting-started.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

</div>

<p align="center">
  <a href="docs/assets/screenshots/platform-overview.png">
    <img src="docs/assets/screenshots/platform-overview.png" alt="Cylism Manager platform health overview" width="1200">
  </a>
</p>

Cylism Manager brings host operations, Kubernetes resources, artifact delivery, certificates, and audit history into one operator-focused Web UI. Kubernetes-compatible APIs are the baseline; K3s receives targeted enhancements when it is detected.

> [!WARNING]
> This is an active control plane, not a read-only dashboard. It holds Kubernetes permissions and protected SSH credentials. Deploy it only in a trusted cluster and follow the [security guidance](SECURITY.md).

## Start here

| You want to... | Go to |
| --- | --- |
| Check whether the platform fits the environment | [Requirements and security boundary](docs/installation/prerequisites.md) |
| Install on a control-plane or single-node environment | [Deployment script](docs/installation/install-script.md) |
| Install through an existing release process | [Helm Chart](docs/installation/install-helm.md) |
| Log in and manage the first server | [First-use guide](docs/getting-started.md) |
| Configure retention, credentials, and public access | [Configuration reference](docs/operations/configuration.md) |
| Diagnose an installation or daily operation | [Operations and troubleshooting](docs/operations/operations.md) |
| Read the full documentation site | [surrychen.github.io/cylism-manager](https://surrychen.github.io/cylism-manager/) |

## Why Cylism Manager

- **Operate hosts and cluster resources together.** Run SSH preflight checks and maintenance operations alongside workloads, Services, storage, certificates, and cluster components.
- **Keep delivery close to operations.** Manage image registries, self-hosted artifact registries, Registry Proxy instances, and platform releases from the same interface.
- **Make changes traceable.** The Web UI and REST API record audit history for operator actions; Agent communication is controlled rather than exposed as an unauthenticated host shell.
- **Use Kubernetes as the common contract.** It works against Kubernetes-compatible APIs instead of requiring a specific distribution.

## Platform compatibility

| Area | Behavior |
| --- | --- |
| Kubernetes | The baseline platform. Manager uses the Kubernetes API and stores its SQLite state in a PersistentVolumeClaim (PVC). |
| K3s | Optional enhancements. K3s agent-node joining and `vpn-auth` compatibility diagnostics are enabled only after K3s is detected. |
| Tailscale and other VPNs | Externally managed. Manager does not install, authenticate, register, upgrade, or mount any external VPN service. |
| Server access | Operators provide a reachable SSH address, user, and protected credential. The address may be private networking, DNS, or another externally managed network path. |

## Install

Choose one supported production path. Both require a Kubernetes-compatible cluster, a suitable StorageClass for the Manager PVC, and an SSH private key for hosts you intend to manage. Read the [prerequisites](docs/installation/prerequisites.md) before applying either command.

### Deployment script

Suitable for a single-node control plane or an interactive initial setup. The script creates or reuses required Secrets and waits for the rollout.

```bash
git clone https://github.com/SurryChen/cylism-manager.git
cd cylism-manager

bash scripts/deploy-platform.sh \
  --image ghcr.io/surrychen/cylism-manager:latest \
  --namespace cylism-system \
  --ssh-key ~/.ssh/id_ed25519 \
  --verify-image-pull
```

Continue with the [script installation guide](docs/installation/install-script.md) for private images, existing Secrets, backups, upgrades, and rollback checks.

### Helm Chart

Suitable when Kubernetes applications are managed through Helm values and a controlled release process.

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --create-namespace \
  --set image.repository=ghcr.io/surrychen/cylism-manager \
  --set image.tag=latest \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

Create the required Secrets before this command. See the [Helm installation guide](docs/installation/install-helm.md) for a complete, production-safe example and PVC configuration.

## What it manages

| Workspace | Examples |
| --- | --- |
| Servers and cluster | SSH preflight, server lifecycle operations, nodes, workloads, Services, ConfigMaps, Secrets, PVCs, and system components |
| Delivery | Image registries, self-hosted OCI registries, Registry Proxy, application releases, and platform image updates |
| Network and security | Managed domains, Ingress endpoints, certificates, certificate issuers, and DNS challenge integrations |
| Operations | Audit history, platform release history, health information, configuration, and diagnostics |

## Scope and operating model

- Manager intentionally receives broad Kubernetes permissions so it can reconcile the resources above. It is not designed as a namespace-only, read-only, or strict multi-tenant control plane.
- SQLite is a single-writer database. The Manager Deployment uses the `Recreate` strategy so upgrades briefly stop the old Pod before starting the new one against the PVC.
- Back up the Manager PVC before upgrades or destructive operations. Do not delete the PVC as part of a normal upgrade.
- K3s support is additive. A standard Kubernetes environment can use the common resource and operations features without K3s or Tailscale.

## Development

Prerequisites: Go 1.25+, Node.js 24+, and npm. Docker, Helm, Python 3.10+, and a Kubernetes test cluster are optional depending on the change.

```bash
go test ./...
go run ./cmd/platform

npm --prefix web ci
npm --prefix web run dev
```

Build the project with:

```bash
make build
```

Read [CONTRIBUTING.md](CONTRIBUTING.md) for the OpenSpec change process, review expectations, and the full verification set.

## Project layout

```text
cmd/        Go program entry points
internal/   API, services, data store, and Kubernetes integration
web/        Vue 3 management interface
k8s/        Static Kubernetes manifest
charts/     Helm Chart
scripts/    Installation and deployment helpers
docs/       Public documentation and historical design material
openspec/   Change specifications and archived decisions
```

## License

Cylism Manager is licensed under the [Apache License 2.0](LICENSE).
