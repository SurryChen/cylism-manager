## 1. Adapter Contract And Config

- [x] 1.1 Add failing unit tests for Nanobot native Responses/Anthropic configuration, protected tool defaults and distinct credential references.
- [x] 1.2 Extend the Runtime Adapter contract with a platform-owned workload specification and implement the Nanobot workload/config adapter.
- [x] 1.3 Run focused `go test ./internal/runtime/...` and report the result.

## 2. Credential Lifecycle

- [x] 2.1 Add failing model/store/handler tests for generated, encrypted Runtime API credentials that are never returned by read APIs.
- [x] 2.2 Implement independent Runtime API credential generation, persistence, Secret materialization and redeploy behavior.
- [x] 2.3 Run focused model/store/api tests and report the result.

## 3. Kubernetes Workload

- [x] 3.1 Add failing Kubernetes manager tests for the init container, Gateway/API containers, Service port, probes, volumes and security context.
- [x] 3.2 Implement Adapter-driven Service and Deployment reconciliation with owned-resource checks and the Nanobot workload defaults.
- [x] 3.3 Run focused Kubernetes manager tests and report the result.

## 4. Nanobot Image

- [x] 4.1 Create the pinned `cylism-nanobot-runtime` source layout and image build definition using `nanobot-ai[api]` under UID/GID 1000.
- [x] 4.2 Implement and test the bounded configuration renderer, ensuring it preserves environment references and rejects unsafe overrides.
- [x] 4.3 Build the image and verify `gateway --help`, `/health`, `/v1/models`, and protected API behavior with a local smoke test. Blocked: the local Docker/Colima daemon socket is unavailable.

## 5. Verification

- [x] 5.1 Run `gofmt`, focused tests per completed task, then `go test ./...`, `go build ./...`, frontend tests and production build.
- [x] 5.2 Run the mandatory security scan for every modified business-code file, `openspec validate nanobot-runtime-workload --strict`, and `git diff --check`. Blocked: the required `sec-code` binary is absent; OpenSpec validation and diff check pass.
- [x] 5.3 Present all verification results for user confirmation before archiving the OpenSpec change.
