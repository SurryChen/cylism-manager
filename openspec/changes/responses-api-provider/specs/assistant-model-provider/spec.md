## ADDED Requirements

### Requirement: OpenAI Responses provider configuration

The system SHALL accept assistant provider configurations only when `provider_type` is `openai_responses`, the provider name and model are non-empty, and an API key is supplied when creating a provider. The provider MAY include a Base URL for an implementation of the Responses API protocol.

#### Scenario: Create an OpenAI Responses provider
- **WHEN** an authenticated administrator creates a provider with `provider_type` set to `openai_responses`, a name, a model, and an API key
- **THEN** the system stores the API key encrypted and returns the provider without exposing the key

#### Scenario: Reject a legacy Chat Completions provider type
- **WHEN** an authenticated administrator creates or updates a provider with `provider_type` set to `openai` or `openai_compatible`
- **THEN** the system rejects the request with a validation error that states only the OpenAI Responses API is supported

#### Scenario: Configure a Responses-compatible endpoint
- **WHEN** an authenticated administrator creates a provider with `provider_type` set to `openai_responses` and a Base URL
- **THEN** the system stores the normalized Base URL with the provider

### Requirement: Responses Runtime deployment

The system SHALL deploy the managed PydanticAI Runtime only with an enabled `openai_responses` provider and SHALL configure its model, API key, and optional Base URL for the Responses API protocol.

#### Scenario: Deploy with a supported provider
- **WHEN** an authenticated administrator selects an enabled `openai_responses` provider and submits a valid Runtime deployment request
- **THEN** the Runtime receives the configured model and API key without an `openai-chat:` model prefix

#### Scenario: Deploy with a compatible endpoint
- **WHEN** an enabled `openai_responses` provider includes a Base URL
- **THEN** the Runtime receives the Base URL through `OPENAI_BASE_URL`

#### Scenario: Update the active Provider
- **WHEN** an administrator updates the model, API key, or Base URL of the Provider selected by the installed Runtime
- **THEN** the system updates the Runtime Secret and ConfigMap and rolls the Runtime Deployment so new Pods use the updated configuration

#### Scenario: Reject a persisted legacy provider
- **WHEN** an authenticated administrator selects a persisted provider whose type is not `openai_responses`
- **THEN** the system rejects the deployment before creating or updating Runtime resources

### Requirement: Responses-only provider settings

The System Settings assistant provider form SHALL state that it configures the Responses API protocol, SHALL allow an optional Responses-compatible Base URL, and SHALL not offer a Chat Completions mode.

#### Scenario: View the provider form
- **WHEN** an administrator opens System Settings
- **THEN** the provider form identifies the Responses API protocol, displays an optional Base URL input, and does not display a Chat Completions mode

#### Scenario: Edit a saved Provider
- **WHEN** an administrator selects a saved Provider for editing
- **THEN** the form permits changing the model, API key, Base URL, enabled state, and default Provider selection
