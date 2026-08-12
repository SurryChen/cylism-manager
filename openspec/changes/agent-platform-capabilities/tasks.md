# Tasks

## 0. Nanobot Integration Proof Of Concept

- [ ] 0.1 In the pinned `nanobot-ai==0.3.0` image, prove the supported custom-tool registration path can invoke a fixed test CLI while `tools.exec.enable=false`; document the API contract and add a regression test.
- [ ] 0.2 If `nanobot serve` cannot safely register the tool, implement and test the smallest Python SDK-based Cylism API adapter that preserves the current OpenAI-compatible streaming/session contract; do not enable generic exec.

## 1. Manager-Owned CLI Artifact

- [ ] 1.1 Create `cmd/cylism-cli` inside the Manager Go module and tests for command parsing, schema validation, JSON envelope, timeout, idempotency headers, and credential redaction.
- [ ] 1.2 Implement the initial read-only commands and reject unknown/free-form commands before making network requests.
- [ ] 1.3 Implement approval status lookup and mutation command DTOs that can return `pending_approval`; test exact request serialization.
- [ ] 1.4 Update the Manager Dockerfile to build and package the static CLI beside the Manager binary, generate its manifest, and test build version/checksum metadata.
- [ ] 1.5 Implement and test the Manager-only internal CLI artifact endpoint with installation-identity authentication and fixed platform/version selection.
  - [x] 1.5.a Add a dedicated installed-CLI update action that preserves grants and rolls the Runtime to download the current Manager artifact.

## 2. Manager Agent Control Plane

- [ ] 2.1 Add models, migrations, store methods, and TDD coverage for Runtime capability grants, CLI installation state/manifests, and immutable Agent operations.
- [ ] 2.2 Add Runtime-specific ServiceAccount, separate projected installation and Agent audience-scoped token volumes, no RoleBinding, and TokenReview authentication middleware; test valid, invalid, revoked, and mismatched identities.
- [ ] 2.3 Extract the initial K8s/application diagnostic and mutation operations into shared services without regressing existing browser API behavior; run impact searches for each moved interface.
- [ ] 2.4 Implement Agent API capability/scope/rate/response-size enforcement and read-only endpoints; test in-scope, out-of-scope, redaction, and request idempotency cases.
  - [x] 2.4.a Add fixed Pod, related Event, PVC, and node diagnostics with scoped capabilities and bounded/redacted responses.
- [ ] 2.5 Implement immutable approval creation, approval/rejection, resource-version revalidation, asynchronous Manager execution, expiry, and terminal result lookup; test no mutation occurs before approval.
- [ ] 2.6 Extend audit persistence and audit query responses for all Agent states, Runtime identity, approval actor and correlation/session references; test denied and failed requests are retained.
- [ ] 2.7 Add Runtime-local action policies (`deny`, `auto`, `approval_required`), one-time approval permits bound to normalized action arguments, and tests for path/argv/environment/output restrictions.

## 3. Runtime Installer And Tool Integration

- [ ] 3.1 Add the fixed Cylism tool adapter and non-root installation init-container entrypoint to `cylism-nanobot-runtime`; test checksum validation, atomic install to `emptyDir`, and that disabled tools, unregistered arguments, and arbitrary executables cannot reach a process invocation.
- [ ] 3.2 Update Manager Runtime workload generation for install/uninstall state, conditional `emptyDir`/init-container/tool registration, separate projected identities, internal Manager endpoint configuration, and least-privilege NetworkPolicy; preserve non-root, read-only root filesystem, dropped capabilities, and disabled generic exec.
- [ ] 3.3 Build the Runtime image and perform an integration test for install success, corrupt artifact rejection, uninstall/revocation, permitted read, denied out-of-scope read, pending mutation, approved mutation, and revoked grant.

## 4. Manager UI

- [ ] 4.1 Add Runtime detail CLI install/update/uninstall state and Agent capability controls with structured scope selection, approval policy, grant revocation, rollout-impact notice, and last-use status; add Vue tests.
  - [x] 4.1.a Move capability editing into a modal and support explicit namespace selection, including mutually exclusive `*` scope.
  - [x] 4.1.b Add a Runtime capability status endpoint and fixed CLI/tool query so Nanobot can inspect effective permissions.
- [ ] 4.2 Add an approval queue with exact impact summary, expiry, approve/reject actions, stale state, and operation result; add Vue and API tests.
  - [x] 4.2.a Add a chat-window entry point for pending approvals and permission management; approval remains Manager/JWT-only.
- [ ] 4.3 Extend the audit view to distinguish Agent action, denial, approval, and execution without displaying sensitive values.

## 4.4 Registry Diagnostics

- [x] 4.4.a Add cluster-scoped `registry.read`, `registry.verify`, and approval-required `registry.pull_check` grants, CLI schemas, and Nanobot fixed operations.
- [x] 4.4.b Return only sanitized Manager-owned mirror/proxy status and correlate Pod image-pull state with configuration evidence.
- [x] 4.4.c Verify only platform-managed node and configured endpoint pairs; request and execute fixed verification-image pulls through the existing browser approval boundary.

## 5. Verification And Rollout

- [ ] 5.1 Complete focused TDD checks after every task, then run `go test ./...`, `go build ./...`, CLI tests/build, Runtime Python tests, frontend tests and production build.
- [ ] 5.2 Run the required `sec-code submit` command for every modified business-code file, `openspec validate agent-platform-capabilities --strict`, `git diff --check`, and image security/integration checks.
- [ ] 5.3 Deploy with no grants by default; verify read-only grants in a non-production namespace before enabling always-approved mutation capabilities.
- [ ] 5.4 Present all verification results for user confirmation before archiving the OpenSpec change.
