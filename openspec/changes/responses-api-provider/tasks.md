## 1. Responses Provider Contract

- [x] 1.1 Add Manager handler tests for accepting `openai_responses` and rejecting legacy provider types.
- [x] 1.2 Update provider validation and Runtime installation validation to require `openai_responses`.
- [x] 1.3 Update Kubernetes Runtime configuration to use a Responses model value without the Chat Completions prefix.
- [x] 1.4 Reconcile the default Provider into an installed Runtime and trigger a Deployment rollout on configuration changes.

## 2. Runtime Integration

- [x] 2.1 Add Runtime tests that verify the configured model is instantiated through PydanticAI's Responses adapter.
- [x] 2.2 Replace the Runtime Chat Completions model construction and default configuration with the Responses API model.
- [x] 2.3 Update the Runtime local deployment manifest and documentation for the Responses API.

## 3. Settings UI

- [x] 3.1 Update the provider form to identify the OpenAI Responses API and remove incompatible fields and modes.
- [x] 3.2 Verify the Vue production build succeeds.
- [x] 3.3 Add Provider editing, default selection, and protected deletion controls.

## 4. Verification

- [x] 4.1 Run focused Manager and Runtime tests.
- [x] 4.2 Run Manager Go tests and build, Runtime tests, and the Manager web production build.
