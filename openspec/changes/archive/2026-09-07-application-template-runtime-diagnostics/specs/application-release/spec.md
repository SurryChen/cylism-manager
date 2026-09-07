## MODIFIED Requirements

### Requirement: Application deployment templates configure container startup

The platform SHALL allow each deployment template to define optional Kubernetes container command and arguments. The fields SHALL use exact array semantics and SHALL remain empty when the image default entrypoint and command are desired.

#### Scenario: Save a template with command and arguments

- **WHEN** a user saves a template whose command and argument inputs contain non-empty lines
- **THEN** the platform SHALL persist each line as one `command` or `args` array item
- **AND THEN** the next release from that template SHALL render the same arrays to the primary container

#### Scenario: Edit a template without changing startup configuration

- **WHEN** a user edits an existing template and leaves the startup fields unchanged
- **THEN** the platform SHALL preserve the existing command and arguments

#### Scenario: Use image defaults

- **WHEN** both startup inputs are empty
- **THEN** the rendered container SHALL omit command and args
- **AND THEN** Kubernetes SHALL use the image defaults

### Requirement: Releases identify their own Pods

The platform SHALL label every Pod template rendered for a Release with the immutable application-local Release sequence using `cylism.io/release`.

#### Scenario: Render a new Release

- **WHEN** the platform renders a Deployment for Release sequence 8
- **THEN** its Pod template SHALL include `cylism.io/release: "8"`
- **AND THEN** the Deployment selector SHALL remain stable across Releases

### Requirement: Release detail exposes current Pod runtime state

The platform SHALL return current Kubernetes Pod state for the requested Release when Pod association labels are available.

#### Scenario: An associated Pod is Ready

- **WHEN** a Release-associated Pod has all primary containers ready
- **THEN** the release detail SHALL identify the Pod as ready and include its node, phase and restart count

#### Scenario: An associated Pod is restarting

- **WHEN** a Release-associated Pod has `CrashLoopBackOff`, a terminated container, or a non-zero restart count with an unavailable container
- **THEN** the release detail SHALL return a concise diagnostic containing the affected Pod and container state
- **AND THEN** the diagnostic SHALL not expose configured credentials

#### Scenario: An old release cannot be precisely associated

- **WHEN** a Release predates Pod release labels
- **THEN** the release detail SHALL report that the Release does not have precise Pod association data
- **AND THEN** it SHALL not attribute all application Pods to that Release

### Requirement: Release detail keeps runtime state fresh

The release details page SHALL continue refreshing runtime information after a Release reaches a terminal orchestration status.

#### Scenario: A formerly successful Release loses readiness

- **WHEN** a Release is recorded as succeeded but its associated Pod later restarts or becomes non-ready
- **THEN** the details page SHALL show the current runtime anomaly without changing the recorded Release outcome
