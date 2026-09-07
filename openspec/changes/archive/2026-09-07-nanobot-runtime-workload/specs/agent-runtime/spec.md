## ADDED Requirements

### Requirement: Nanobot uses a native multi-container workload

The system SHALL deploy each managed Nanobot Runtime as one single-replica Kubernetes Deployment with a shared Runtime PVC, a configuration-rendering init container, a Nanobot Gateway container, and a Nanobot Agent API container.

#### Scenario: Deploy a Nanobot Runtime

- **WHEN** an administrator deploys a valid Nanobot Runtime
- **THEN** the system SHALL create or update the owned PVC, ConfigMap, Secret, Service and Deployment
- **AND THEN** the Deployment SHALL contain the rendering init container, `gateway`, and `api` containers sharing `/data`
- **AND THEN** the Service SHALL expose only the API container on port 8900

### Requirement: Nanobot configuration follows the selected model protocol

The system SHALL transform the platform Runtime model fields into a Nanobot native configuration without persisting model credential values in a ConfigMap or PVC.

#### Scenario: Deploy a Responses-backed Runtime

- **WHEN** a Nanobot Runtime selects the `responses` model protocol
- **THEN** the generated configuration SHALL use the `openai` provider with `apiType` set to `responses`
- **AND THEN** it SHALL use `${CYLISM_MODEL_API_KEY}` as the provider key reference

#### Scenario: Deploy an Anthropic-backed Runtime

- **WHEN** a Nanobot Runtime selects the `anthropic` model protocol
- **THEN** the generated configuration SHALL use the `anthropic` provider and its configured API base
- **AND THEN** it SHALL use `${CYLISM_MODEL_API_KEY}` as the provider key reference

### Requirement: Agent API credentials are separate from model credentials

The system SHALL create and manage a Runtime API credential independently from the model provider credential.

#### Scenario: API is accessible only with the Runtime credential

- **WHEN** Nanobot serves its API on a non-loopback interface
- **THEN** the native configuration SHALL require `${CYLISM_RUNTIME_API_KEY}`
- **AND THEN** the API container SHALL receive that value from a Kubernetes Secret distinct from the model credential key
- **AND THEN** Runtime list, detail, health and log responses SHALL NOT include either plaintext credential

### Requirement: Runtime defaults deny arbitrary host execution

The system SHALL apply a restrictive Nanobot tool policy and Pod security settings by default.

#### Scenario: Default Runtime execution policy

- **WHEN** the system generates Nanobot configuration
- **THEN** it SHALL disable Nanobot exec tools and remote WebUI package installation
- **AND THEN** it SHALL restrict file tools to the Runtime workspace
- **AND THEN** the Pod SHALL run as non-root without an automatically mounted Kubernetes ServiceAccount token or Linux capabilities

### Requirement: Runtime health reflects the public Agent API

The system SHALL use the Nanobot Agent API health endpoint as the managed Runtime health contract.

#### Scenario: Check a deployed Runtime

- **WHEN** the platform checks a deployed Nanobot Runtime
- **THEN** it SHALL call the API Service `GET /health` on port 8900
- **AND THEN** the Deployment readiness result SHALL require both Gateway and API containers to be ready
