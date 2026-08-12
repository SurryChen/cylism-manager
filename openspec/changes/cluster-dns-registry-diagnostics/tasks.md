# Tasks

## 1. Core DNS policy domain and Kubernetes adapter

- [x] 1.1 Add failing model/store tests for one active versioned Cluster DNS policy, immutable history, validation result persistence, and rollback-as-new-revision semantics.
- [x] 1.2 Implement validated policy persistence and encrypted/field-bounded DTOs; reject arbitrary Corefile fragments and unsupported resolver forms.
- [ ] 1.3 Add failing Kubernetes adapter tests proving only the Manager-owned CoreDNS custom override is created, updated, or deleted, while base Corefile and CoreDNS Deployment lifecycle fields remain unchanged.
- [ ] 1.4 Implement CoreDNS custom override reconciliation, Ready Pod discovery, fixed query validation, and controlled rollout observation.
- [x] 1.5 Add browser API tests for read status, validation failure, apply success, rollback validation, and inherited-forwarding restoration.

## 2. Registry Proxy egress diagnostics and outbound proxy configuration

- [x] 2.1 Add failing model/store tests for encrypted outbound proxy settings and non-sensitive response projection.
- [x] 2.2 Extend Registry Proxy validation and Deployment rendering to inject only configured proxy environment variables; verify updates preserve existing Registry, cache, NodePort, and node placement behavior.
- [x] 2.3 Add failing service tests for fixed managed-Pod selection, bounded diagnostic parsing, and deterministic DNS/TCP/TLS/Registry outcome classification.
- [x] 2.4 Implement the constrained workload-context diagnostic helper/exec path and browser endpoint; reject non-managed proxies, non-ready Pods, free-form URLs, commands, images, and Pod names.
- [x] 2.5 Add Registry Proxy handler tests covering ready-versus-egress distinction, redaction, timeout classification, and failed diagnostic execution.

## 3. Agent capability and CLI integration

- [x] 3.1 Add capability schema and authorization tests for `dns.read` and `registry.proxy_diagnose`, including denied and allowlisted-domain requests.
- [x] 3.2 Add fixed CLI command parsing and Agent API endpoints for DNS status, allowlisted resolution, and configured Registry Proxy diagnostics; cover bounded/redacted envelopes.
- [x] 3.3 Register the matching Nanobot operations and update catalog synchronization tests so enabled grants appear in the tool operation list.

## 4. Cluster console UI

- [x] 4.1 Add a Cluster Hub `集群 DNS` tab and component tests for route selection and rendered policy state.
- [x] 4.2 Implement `ClusterDNS.vue` with health summary, resolver editor, validation/apply/rollback flow, rollout status, and history; add Vue tests for success and error states.
- [x] 4.3 Extend Registry Mirrors Proxy cards/modal with egress status, diagnostic details, explicit diagnostic action, and redacted outbound proxy configuration; add Vue tests.

## 5. Component-specific availability baselines

- [x] 5.1 Add failing backend tests for component availability profiles, including supported CoreDNS HA, unsupported metrics-server/local-path-provisioner replica increases, and unknown-profile behavior.
- [x] 5.2 Implement profile-driven baseline and restore-default API behavior; remove the static-controller-mode-wide two-replica default and preserve existing CoreDNS migration controls only where explicitly supported.
- [x] 5.3 Add preflight checks for Ready/schedulable nodes, bounded resource availability, component health, and managed image-pull diagnostics before a supported replica increase; test that every blocker prevents mutation.
- [x] 5.4 Update System Components UI with availability labels, prerequisite summaries, blocked-state evidence, and explicit unsupported/default-restoration flows; add Vue tests.

## 6. Verification and rollout

- [x] 5.1 Run focused Go and Vue tests after every completed task, then `go test ./...`, `go build ./...`, frontend tests, and production build.
- [x] 5.2 Run `openspec validate cluster-dns-registry-diagnostics --strict` and `git diff --check`.
- [ ] 5.3 Deploy first with inherited forwarding displayed but no active policy; validate from all CoreDNS and Registry Proxy Pods before applying a production resolver policy.
- [ ] 5.4 Present verification results for user confirmation before archiving the change.
