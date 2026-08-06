## Context

The Manager persists a provider type, model name, encrypted API key, and optional base URL. It currently deploys the runtime with `OPS_AGENT_MODEL=openai-chat:<model>`, while the runtime depends on PydanticAI's OpenAI integration. The product decision is to make OpenAI Responses API the sole minimal provider integration and not retain Chat Completions compatibility.

## Goals / Non-Goals

**Goals:**

- Persist and validate `openai_responses` as the only provider type.
- Deploy the Runtime with an OpenAI Responses model, API key, and optional Responses-compatible Base URL.
- Make the System Settings UI accurately describe the supported integration.
- Detect and reject persisted legacy provider types before a Runtime is deployed.

**Non-Goals:**

- Streaming, cancellation, retries, or a generic Runtime `/v1/chat` protocol.
- Anthropic, Gemini, Azure, Bedrock, or Chat Completions-compatible provider adapters.
- Migration of legacy provider secrets or automatic conversion of existing records.

## Decisions

### Use PydanticAI's native OpenAI Responses model

The Runtime will construct `OpenAIResponsesModel(model)` and pass that model to `Agent`. The Manager configures the model using `OPS_AGENT_MODEL` without an `openai-chat:` prefix and the Runtime defaults to a Responses-capable OpenAI model. This keeps Provider protocol selection inside the Runtime and uses the PydanticAI adapter intended for the Responses API.

Alternative: retain a model-string prefix and let PydanticAI select a transport. This would preserve the Chat Completions path and make the selected API style ambiguous, so it is rejected.

### Make provider protocol explicit and single-valued

The Manager accepts only `openai_responses`; the UI supplies that value without a selectable alternative. The optional Base URL is stored with the provider and passed to the Runtime as `OPENAI_BASE_URL`, which PydanticAI's OpenAI provider uses when constructing `OpenAIResponsesModel`.

Alternative: separate provider types for each Responses-compatible endpoint. The protocol is the relevant integration boundary, so separate types would add configuration without changing Runtime behavior.

### Reconcile the active Provider on change

The Manager records the selected Provider ID when installing the Runtime. When that Provider is updated, or another Provider is explicitly set as default, the Manager reuses the existing Runtime node, audit PVC size, and image to update the Secret and ConfigMap. The Deployment Pod template receives a fresh configuration-version annotation so Kubernetes performs a rolling restart and new Pods consume the updated environment variables.

Alternative: require the operator to redeploy after every Provider update. This makes credential rotation and endpoint changes error-prone and leaves the stored desired state different from the running Runtime.

### Reject legacy provider rows at deployment time

Database migration is not required because the existing type column can store the new value. Legacy rows remain readable for operators to replace, but they cannot be deployed. This avoids silently interpreting Chat Completions credentials as a Responses integration.

## Risks / Trade-offs

- [Existing providers cannot be deployed after the change] -> The API returns a precise validation error and the UI makes the supported API style visible.
- [A configured endpoint may not fully implement the Responses API] -> The UI and API identify the required protocol, and the Runtime uses the native Responses adapter rather than falling back to Chat Completions.
- [The database update succeeds but Runtime reconciliation fails] -> The API reports the reconciliation failure explicitly; operators can correct the cluster condition and save the Provider again.
- [The Runtime image may lag behind Manager deployment changes] -> Runtime defaults, local manifests, and README are updated together and validated with tests.

## Migration Plan

1. Deploy the updated Runtime image before or with the Manager update.
2. Create a new Responses API provider in System Settings, optionally configure its compatible endpoint URL, and select it during Runtime deployment.
3. Replace legacy provider records manually; do not reuse them as a different API style.
4. Roll back by restoring the prior Manager and Runtime versions together. No database migration is required.

## Open Questions

- None for the minimal Responses API protocol integration.
