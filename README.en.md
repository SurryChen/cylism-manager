<div align="center">

# Cylism Manager

**A management dashboard for personal servers, Kubernetes clusters, and self-hosted services.**

[![Build](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml/badge.svg?branch=main)](https://github.com/SurryChen/cylism-manager/actions/workflows/docker-image.yml)
[![Docs](https://github.com/SurryChen/cylism-manager/actions/workflows/docs-pages.yml/badge.svg?branch=main)](https://surrychen.github.io/cylism-manager/)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[简体中文](README.md) · [Documentation](https://surrychen.github.io/cylism-manager/) · [Get started](docs/getting-started.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

</div>

Cylism Manager is a dashboard for managing Linux servers, Kubernetes clusters, and self-hosted services.

<p align="center">
  <a href="docs/assets/screenshots/platform-overview.png">
    <img src="docs/assets/screenshots/platform-overview.png" alt="Cylism Manager platform health overview" width="1200">
  </a>
</p>

## What it does today

- Shows the state of managed hosts, nodes, services, and recent operations
- Runs host preflight checks and basic maintenance through SSH
- Lets you inspect and manage common Kubernetes resources
- Tracks images, deployments, and application releases
- Configures service domains and certificates

## Install

You can install it with the deployment script or Helm Chart. Installation requires an accessible Kubernetes or K3s cluster. Platform data is stored in a PVC created during installation by default; with Helm, you can specify a StorageClass or use an existing PVC.

- [Deployment script](docs/installation/install-script.md): interactive initial setup.
- [Helm Chart](docs/installation/install-helm.md): environments already managed with Helm values.
- [Getting started guide](docs/getting-started.md): first login and server onboarding.
- [Documentation site](https://surrychen.github.io/cylism-manager/): complete reference.

### Deployment script

For a single-node control plane or an interactive initial setup:

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

Create the required Secrets first as described in the [Helm installation guide](docs/installation/install-helm.md):

```bash
helm upgrade --install cylism-manager charts/cylism-manager \
  --namespace cylism-system \
  --create-namespace \
  --set image.repository=ghcr.io/surrychen/cylism-manager \
  --set image.tag=latest \
  --set secrets.existingSecret=cylism-secret \
  --set ssh.existingSecret=cylism-ssh-key
```

## Operating notes

- Manager stores its SQLite state in a PVC. Back up the PVC before upgrades, and do not delete it during a normal upgrade.
- SQLite has a single writer. The Deployment uses `Recreate`, so upgrades have a short period of unavailability.

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
