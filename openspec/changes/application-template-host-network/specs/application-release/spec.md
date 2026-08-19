## MODIFIED Requirements

### Requirement: Application templates render Kubernetes Services

The platform SHALL preserve an optional Pod network mode in application deployment templates and release snapshots. Templates that omit the mode SHALL continue using the Kubernetes Pod network.

#### Scenario: Render a host-network template

- **WHEN** an application deployment template sets `host_network=true`
- **THEN** the rendered Pod SHALL set `hostNetwork: true`
- **AND THEN** the rendered Pod SHALL set `dnsPolicy: ClusterFirstWithHostNet`
- **AND THEN** the rendered Service and workload update strategy SHALL otherwise retain their existing behavior

#### Scenario: Preserve a default Pod-network template

- **WHEN** an application deployment template omits `host_network` or sets it to `false`
- **THEN** the rendered Pod SHALL not set `hostNetwork`
- **AND THEN** the rendered Pod SHALL retain the default Kubernetes DNS policy

#### Scenario: Edit a host-network template

- **WHEN** an operator enables host networking in the deployment template editor and saves the template
- **THEN** subsequent reads and releases of that template SHALL preserve `host_network=true`
