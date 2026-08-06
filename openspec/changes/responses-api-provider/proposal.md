## Why

The assistant runtime currently exposes a generic provider form but always configures PydanticAI through the OpenAI Chat Completions transport. This prevents the platform from using the Responses API protocol, which is the intended minimal provider integration for tool-using agent workflows.

## What Changes

- Add a managed assistant provider capability for Responses API-compatible endpoints.
- **BREAKING** Remove `openai` and `openai_compatible` provider modes; new and updated provider records must use `openai_responses`.
- Configure the managed PydanticAI runtime with an OpenAI Responses model, API key, and optional compatible endpoint URL.
- Reconcile the installed Runtime when its default Provider changes so updated model credentials and endpoint settings take effect without a separate deployment action.
- Update the System Settings provider form and runtime deployment manifests to describe Responses API usage.

## Capabilities

### New Capabilities
- `assistant-model-provider`: Configure and deploy the assistant runtime with a Responses API-compatible provider.

### Modified Capabilities

- None.

## Impact

The Manager assistant API, Kubernetes reconciler, System Settings UI, and the `cylism-ops-agent` Python runtime and deployment documentation are affected. Existing provider records using Chat Completions modes must be recreated with the Responses API mode before deployment. Updating the active Provider restarts the Runtime Pod through a Deployment rollout.
