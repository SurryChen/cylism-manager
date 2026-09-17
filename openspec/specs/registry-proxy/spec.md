# registry-proxy Specification

## Purpose
TBD - created by archiving change managed-registry-proxy. Update Purpose after archive.
## Requirements
### Requirement: Manage independent Registry Proxy instances

The platform SHALL manage multiple non-persistent `registry:2` pull-through proxy instances. Each instance SHALL bind one Registry domain to one HTTPS upstream and SHALL have independent Kubernetes resources and temporary cache storage.

#### Scenario: Create a Kubernetes Registry proxy

- **WHEN** a user creates an instance for `registry.k8s.io` with a connected node, reachable private or Tailscale address, and unused NodePort
- **THEN** the platform SHALL create a node-selected Deployment and NodePort Service unique to that instance
- **AND THEN** it SHALL set `REGISTRY_PROXY_REMOTEURL` to `https://registry.k8s.io`

#### Scenario: Preserve Docker Hub defaults

- **WHEN** a user creates a Docker Hub instance without an explicit upstream URL
- **THEN** the platform SHALL configure `https://registry-1.docker.io` as its upstream

#### Scenario: Reject an incompatible upstream

- **WHEN** a user configures a non-Docker Hub Registry with a different upstream domain
- **THEN** the platform SHALL reject the configuration before creating Kubernetes resources

#### Scenario: Reject duplicate NodePort usage

- **WHEN** a user selects a NodePort already assigned to another managed proxy instance
- **THEN** the platform SHALL reject the request and identify the conflicting proxy

#### Scenario: Migrate a legacy Docker Hub resource name

- **WHEN** a user explicitly migrates the legacy Docker Hub proxy using `cylism-registry-proxy`
- **THEN** the platform SHALL delete its legacy Service and Deployment
- **AND THEN** it SHALL recreate the same proxy configuration as `cylism-registry-proxy-<id>` using the existing NodePort
- **AND THEN** it SHALL report that a short proxy interruption occurs during migration

### Requirement: Retain non-persistent cache lifecycle

The platform SHALL use a size-limited `emptyDir` cache for every managed proxy and SHALL clear it by replacing only that proxy's Pod.

#### Scenario: Manually clear one proxy cache

- **WHEN** a user requests cache cleanup for an instance
- **THEN** the platform SHALL delete only Pods selected by that instance's labels
- **AND THEN** the replacement Pod SHALL receive an empty cache volume

### Requirement: Observe and configure proxies independently

The platform SHALL list each Registry Proxy's Registry, upstream, endpoint, node, cache configuration, readiness and latest error in the delivery artifact registry workspace. It SHALL allow configuring, diagnosing, migrating legacy resources, and clearing an individual Proxy cache from that workspace.

#### Scenario: View multiple proxies in the delivery center

- **WHEN** a user opens the Registry Proxy workspace in the delivery center
- **THEN** the platform SHALL show Docker Hub and Kubernetes Registry Proxy instances as separate entries
- **AND THEN** it SHALL not imply that one endpoint can proxy both Registries

#### Scenario: Maintain a Proxy without changing node configuration

- **WHEN** a user creates, edits, diagnoses, or clears a Registry Proxy cache
- **THEN** the platform SHALL perform only the requested Proxy lifecycle action
- **AND THEN** it SHALL not create, edit, apply, or restart a node registry mirror configuration

