## ADDED Requirements

### Requirement: Deploy a non-persistent Docker Hub proxy

The platform SHALL deploy one managed `registry:2` pull-through proxy for Docker Hub on a selected cluster node without a PVC or hostPath cache volume.

#### Scenario: Create a proxy on an eligible node

- **WHEN** a user selects a connected cluster node and a private reachable address
- **THEN** the platform SHALL create a Node-selected Deployment, temporary `emptyDir` cache volume, and NodePort Service
- **AND THEN** it SHALL wait for the proxy Pod to become Ready before applying any node mirror mapping

#### Scenario: Reject unsafe endpoint addresses

- **WHEN** a user provides an empty, loopback, or public endpoint address
- **THEN** the platform SHALL reject the configuration and explain that the proxy endpoint must be reachable only through private network or Tailscale

### Requirement: Apply the proxy as a Docker Hub mirror

The platform SHALL apply a ready managed proxy endpoint only to user-selected cluster nodes as the `docker.io` registry mirror.

#### Scenario: Apply to selected nodes

- **WHEN** the proxy is Ready and the user applies it to selected nodes
- **THEN** the platform SHALL render the proxy NodePort endpoint under `mirrors.docker.io` in each selected node's `registries.yaml`
- **AND THEN** it SHALL report node-by-node apply results

### Requirement: Safely clean temporary proxy cache

The platform SHALL clear temporary proxy cache by replacing the proxy Pod, never by deleting cache files from a running Registry process.

#### Scenario: Cache threshold reached

- **WHEN** a cache check observes usage at or above the configured cleanup threshold
- **THEN** the platform SHALL record the observed size and delete the proxy Pod
- **AND THEN** the Deployment replacement Pod SHALL start with an empty cache volume

#### Scenario: Scheduled cleanup is due

- **WHEN** the configured cleanup interval has elapsed since the last successful cleanup
- **THEN** the platform SHALL replace the proxy Pod even when current cache usage is below the threshold

#### Scenario: Cache inspection fails

- **WHEN** the platform cannot inspect temporary cache usage
- **THEN** it SHALL record the inspection failure
- **AND THEN** it SHALL not delete the proxy Pod solely because the cache size is unknown

### Requirement: Proxy status remains observable

The platform SHALL expose deployment readiness, cache usage, last inspection, last cleanup and the latest error without exposing upstream proxy credentials.

#### Scenario: View proxy status

- **WHEN** a user opens the registry proxy management page
- **THEN** the platform SHALL show the configured node, endpoint, cache limit, cleanup schedule, current readiness and maintenance status
