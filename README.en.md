<div align="center">

# Cylism Manager

**Put the scattered, easy-to-forget parts of self-hosting in one place.**

[![Build](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml/badge.svg?branch=main)](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml)
[![Docs](https://img.shields.io/badge/docs-GitHub%20Pages-222?logo=github)](https://surrychen.github.io/cylism-manager/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[简体中文](README.md) · [Documentation](https://surrychen.github.io/cylism-manager/) · [Get started](docs/getting-started.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

</div>

After a small cluster has been running for a while, machines live in SSH, resources live in `kubectl`, and releases, domains, certificates, and logs end up in different places.

Cylism Manager is a self-hosted workspace for personal projects, small services, and homelabs. It brings SSH-reachable Linux servers, Kubernetes clusters, and the services running on them into one interface, so daily maintenance involves less context switching.

<p align="center">
  <a href="docs/assets/screenshots/platform-overview.png">
    <img src="docs/assets/screenshots/platform-overview.png" alt="Cylism Manager platform health overview" width="1200">
  </a>
</p>

## What it helps with

**See what is happening first.**

Check servers, nodes, services, and recent operations so you can find the part that actually needs attention.

**Then take action.**

Run SSH preflight and maintenance operations, and handle common Kubernetes resources from the same interface.

**Finally, ship the service.**

Manage images and release history, deploy applications, and configure domains and certificates so services are reachable.

## What it is not

It is not a multi-tenant cloud platform, and it does not try to replace every Kubernetes tool with another abstraction layer. It is for people with a few servers, a K3s or Kubernetes cluster, and a desire to keep their own services easier to run.

Kubernetes is the baseline. K3s is an optional optimization. Tailscale and other VPNs remain managed outside Manager; it does not install, authenticate, register, or upgrade them.

> [!WARNING]
> Manager is not a read-only dashboard. It needs Kubernetes management permissions and holds protected SSH credentials. Deploy it only in a cluster you trust, and read the [security policy](SECURITY.md).

## Getting started

Read the [installation prerequisites](docs/installation/prerequisites.md) first. The cluster needs PVC storage, and you need an SSH private key for the servers you intend to manage.

- Want to get it running quickly? Use the [deployment script](docs/installation/install-script.md).
- Already use Helm? Start with the [Helm Chart](docs/installation/install-helm.md).
- Want to understand the first login and server workflow? Read the [getting started guide](docs/getting-started.md).
- Want the complete guide? Visit the [documentation site](https://surrychen.github.io/cylism-manager/).

### Deployment script

Suitable for a single-node control plane or an interactive initial setup:

```bash
git clone https://github.com/SurryChen/cylism-manager.git
cd cylism-manager

bash scripts/deploy-platform.sh \
  --image ghcr.io/surrychen/cylism-manager:latest \
  --namespace cylism-system \
  --ssh-key ~/.ssh/id_ed25519 \
  --verify-image-pull
```

### Helm Chart

Suitable when applications are already managed through Helm values. Create the required Secrets first as described in the [Helm installation guide](docs/installation/install-helm.md):

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --create-namespace \
  --set image.repository=ghcr.io/surrychen/cylism-manager \
  --set image.tag=latest \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

## A few operating notes

- Manager stores its SQLite state in a PVC. Back up the PVC before upgrades, and do not delete it during a normal upgrade.
- SQLite has a single writer. The Deployment uses `Recreate`, so upgrades have a short period of unavailability.
- Manager needs broad Kubernetes permissions and is not intended for shared clusters that only allow namespace-scoped read access.
- K3s node joining and `vpn-auth` diagnostics appear only when K3s is detected; standard Kubernetes environments can still use the common features.

## Development

You need Go 1.25+, Node.js 24+, and npm. Docker, Helm, Python 3.10+, and a Kubernetes test cluster are optional depending on the change.

```bash
go test ./...
go run ./cmd/platform

npm --prefix web ci
npm --prefix web run dev
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full verification commands and OpenSpec change process.

## License

Cylism Manager is licensed under the [Apache License 2.0](LICENSE).
