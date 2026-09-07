## ADDED Requirements

### Requirement: Runtime agent identity is separate and least-privileged

The system SHALL authenticate each managed Runtime agent through a Runtime-specific projected Kubernetes ServiceAccount Token with the `cylism-manager-agent` audience. The identity SHALL be distinct from model credentials, Runtime chat API credentials, browser JWTs, and Kubernetes API authorization.

#### Scenario: Authenticated Agent API request

- **WHEN** a managed Runtime calls an Agent API endpoint with a valid projected token
- **THEN** the Manager SHALL resolve the Runtime identity from the validated token subject and Runtime mapping
- **AND THEN** the Manager SHALL NOT trust a Runtime ID supplied in the request body or command arguments
- **AND THEN** the Runtime identity SHALL have no Kubernetes RBAC granted by this feature

#### Scenario: Revoked or invalid Runtime identity

- **WHEN** a Runtime calls the Agent API with an expired, invalid, unmapped, or revoked identity
- **THEN** the Manager SHALL reject the request before evaluating a capability
- **AND THEN** the rejection SHALL be recorded without exposing credentials

### Requirement: Agent capabilities are explicitly granted and scoped

The system SHALL authorize Agent requests only when an enabled capability grant for the authenticated Runtime permits the requested action and target scope.

#### Scenario: Scoped diagnostic request

- **WHEN** a Runtime has `workload.read` for namespace `production` and requests a Deployment in `production`
- **THEN** the Manager SHALL return only the permitted, bounded diagnostic result

#### Scenario: Out-of-scope request

- **WHEN** a Runtime requests an action or resource outside its enabled capability grant
- **THEN** the Manager SHALL deny the request
- **AND THEN** it SHALL record the Runtime, requested action, target summary, correlation ID, and denial reason

#### Scenario: Grant is revoked while Runtime remains deployed

- **WHEN** an administrator disables a Runtime capability grant
- **THEN** subsequent Agent API requests using that Runtime identity SHALL be denied without requiring a Runtime redeploy

### Requirement: Runtime can inspect effective capability status

The system SHALL expose the authenticated Runtime's effective capability state through a fixed Agent API and CLI command. The response SHALL include every registered capability, whether it is enabled, its effective namespace scope, and whether a human approval is required.

#### Scenario: Capability status query

- **WHEN** an authenticated Runtime invokes `cylism-cli capability status --output json`
- **THEN** the Manager SHALL return the effective grants for that Runtime only
- **AND THEN** a cluster-scoped grant SHALL be represented by namespace `*`
- **AND THEN** the response SHALL not include tokens, grant editor credentials, or data from another Runtime

#### Scenario: Capability status after revocation

- **WHEN** an administrator revokes or changes a capability grant
- **THEN** the next capability status query SHALL reflect the new effective state without requiring a Runtime restart

### Requirement: Agent supports bounded Pending Pod diagnostics

The system SHALL provide fixed, read-only CLI operations for Pod status, related Events, PVC status, and node status without exposing generic Kubernetes access. Pod status SHALL require `workload.read`; related Events SHALL require `events.read`; PVC status SHALL require `storage.read`; node status SHALL require cluster-scoped `cluster.read`.

#### Scenario: Diagnose a Pending Pod

- **WHEN** a Runtime with `workload.read` and `events.read` for `kube-system` requests a named Pod and its related Events
- **THEN** the Manager SHALL return the Pod phase, bounded scheduling/container status fields, and at most 30 redacted Events for that Pod
- **AND THEN** the Runtime SHALL not receive Pod environment variables, Secret values, kubeconfig, or arbitrary resource content

#### Scenario: Out-of-scope diagnostic query

- **WHEN** a Runtime requests a Pod, Event, or PVC outside the namespace granted for its corresponding capability
- **THEN** the Manager SHALL deny the request before reading the resource

#### Scenario: Node diagnosis

- **WHEN** a Runtime with cluster-scoped `cluster.read` requests a named node
- **THEN** the Manager SHALL return only Ready conditions, taints, schedulability, and allocatable resource summaries

### Requirement: The CLI exposes only registered platform operations

The system SHALL provide `cylism-cli` as a JSON-oriented client with a fixed command and parameter schema for registered Agent platform operations.

#### Scenario: Registered CLI operation

- **WHEN** the Runtime invokes a supported CLI command with valid arguments
- **THEN** the CLI SHALL submit a typed request with a correlation and idempotency identifier
- **AND THEN** it SHALL emit a bounded JSON result envelope without printing credentials

#### Scenario: Arbitrary command or endpoint attempt

- **WHEN** the Runtime attempts an unknown command, arbitrary HTTP request, `kubectl` passthrough, or free-form shell command
- **THEN** the CLI SHALL reject it locally
- **AND THEN** the Manager SHALL not receive an executable arbitrary command payload

### Requirement: The Manager builds and controls CLI installation

The system SHALL build `cylism-cli` from the Manager module and store its versioned binary and checksum manifest in the Manager image. A Runtime SHALL receive the binary only through an authenticated Manager internal artifact endpoint and a platform-managed installation init container; it SHALL NOT download from arbitrary URLs or be modified by Kubernetes `exec`/`cp`.

#### Scenario: Manager image exposes its CLI artifact

- **WHEN** a Manager image is built or promoted
- **THEN** it SHALL contain the matching `cylism-cli` binary and a manifest with build version, target platform, size and checksum
- **AND THEN** the internal artifact endpoint SHALL return only the matching manifest and binary to an authenticated installation identity

#### Scenario: Install CLI into a Runtime

- **WHEN** an administrator requests CLI installation for a Runtime
- **THEN** the Manager SHALL record the desired Manager-provided CLI manifest and update the Runtime Deployment with an `emptyDir`, installation init container, and installation identity
- **AND THEN** the init container SHALL validate the manifest and atomically install the binary to the mounted `emptyDir`
- **AND THEN** the platform tool SHALL not be registered until the installed CLI matches the desired manifest

#### Scenario: Installation failure

- **WHEN** the artifact cannot be retrieved or fails version, checksum, size, or executable-format validation
- **THEN** the Runtime tool SHALL remain unavailable
- **AND THEN** the Manager SHALL report the installation as failed without executing a partial binary

#### Scenario: Uninstall CLI from a Runtime

- **WHEN** an administrator requests CLI uninstallation for a Runtime
- **THEN** the Manager SHALL immediately revoke the Runtime's Agent authorization and tool registration
- **AND THEN** it SHALL update the Deployment to remove the installer and CLI volume
- **AND THEN** the installed binary SHALL disappear when the prior Pod's `emptyDir` is terminated

### Requirement: Nanobot uses a dedicated platform tool without generic shell execution

The system SHALL integrate `cylism-cli` through a Runtime-owned Nanobot tool adapter while retaining the restrictive default Nanobot execution policy.

#### Scenario: Platform tool invocation

- **WHEN** Nanobot selects a granted Cylism platform operation
- **THEN** the adapter SHALL invoke only the registered CLI command and typed arguments
- **AND THEN** `tools.exec.enable` SHALL remain `false`

### Requirement: Local actions use explicit approval policies

The system SHALL model Runtime-local actions as registered action IDs with one of `deny`, `auto`, or `approval_required` policies. The platform MAY provide recommended built-in action templates, but every Runtime SHALL default to `deny`; an administrator SHALL configure the effective policy per Runtime and MAY later disable any built-in action. It SHALL never treat an arbitrary shell string as an approvable action.

#### Scenario: Default local action policy

- **WHEN** an administrator creates or deploys a Runtime without saving a local action policy
- **THEN** every built-in local action SHALL be denied
- **AND THEN** the built-in action directory SHALL remain available only as a configuration template

#### Scenario: Low-risk local action without approval

- **WHEN** an administrator has explicitly configured an enabled `auto` policy for a bounded read-only action such as `workspace.list`
- **THEN** the Manager SHALL allow only the registered path, arguments, environment and output limits
- **AND THEN** the action SHALL be audited as an Agent operation

#### Scenario: Disable a previously configured built-in action

- **WHEN** an administrator changes a Runtime's built-in action policy from `auto` or `approval_required` to `deny`
- **THEN** subsequent requests for that action SHALL be denied without a Runtime redeploy
- **AND THEN** the denial SHALL be recorded as an Agent operation

#### Scenario: Local mutation or elevated action

- **WHEN** a Runtime requests an action configured as `approval_required`
- **THEN** the Manager SHALL create a pending operation bound to the exact action ID and normalized arguments
- **AND THEN** the local executor SHALL not run until a valid, unexpired, single-use approval permit is presented

#### Scenario: Arbitrary local command

- **WHEN** a Runtime requests `sh`, `bash`, an interpreter, command concatenation, arbitrary `curl`, or an unregistered executable
- **THEN** the Manager and local adapter SHALL reject the request even if a user has previously approved another action

### Requirement: Agent write operations require immutable human approval

The system SHALL require human approval for every first-release Agent mutation and SHALL execute approved mutations only from the Manager control plane.

#### Scenario: Agent requests deployment scale

- **WHEN** an authorized Runtime requests a Deployment scale operation in scope
- **THEN** the Manager SHALL create a pending operation containing the action, sanitized parameters, parameter hash, grant version, target resource version, impact summary, and expiry
- **AND THEN** the CLI SHALL receive `pending_approval` and an operation ID
- **AND THEN** the target workload SHALL remain unchanged until approval

#### Scenario: Approve unchanged operation

- **WHEN** an administrator approves an unexpired pending operation and the target resource version remains unchanged
- **THEN** the Manager SHALL execute the exact approved operation asynchronously
- **AND THEN** it SHALL record the approver, execution outcome, and result summary

#### Scenario: Target changes before approval execution

- **WHEN** an administrator approves a pending operation whose target resource version has changed
- **THEN** the Manager SHALL mark the operation stale and SHALL NOT execute it
- **AND THEN** a new Agent request and approval SHALL be required

### Requirement: Agent activity is fully auditable and bounded

The system SHALL retain an immutable record for every Agent request, including allowed reads, denied requests, pending approvals, approvals, rejections, successes, failures, and expirations.

#### Scenario: Bounded untrusted diagnostic data

- **WHEN** an Agent requests logs, events, or resource metadata
- **THEN** the Manager SHALL apply response size limits, field allowlists, and sensitive-value redaction before returning the data
- **AND THEN** the response content SHALL not alter authorization or approval decisions

#### Scenario: Audit an Agent operation

- **WHEN** an Agent operation reaches a terminal status
- **THEN** the audit view SHALL identify the Runtime, capability, correlation/session reference, target summary, approval actor where applicable, and terminal status
- **AND THEN** it SHALL not expose plaintext secrets or workload credentials
