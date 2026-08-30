## 1. Server and Node Domain

- [x] 1.1 Record server/node routes, request/response/error contracts and write failing Cluster Service tests for server lifecycle, node labels and node-operation prechecks.
- [x] 1.2 Extract reusable server and node workflows to `internal/service/cluster` with injected SSH/Agent and Kubernetes adapters.
- [x] 1.3 Move all server/node HTTP adapters and tests to `internal/api/infrastructure`, including network diagnostics, terminal and join-progress. WebSocket and SSH implementations are owned by dedicated infrastructure transport adapters, while route ownership and lifecycle operations are provided by the infrastructure boundary.

## 2. Storage Domain

- [x] 2.1 Record PVC inventory, create/delete, usage, host-directory import and migration task contracts; add Storage Service tests.
- [x] 2.2 Extract PVC lifecycle, protection, import and migration orchestration to `internal/service/storage`; task persistence/state transitions, path policies, PVC lookup and asynchronous dispatch use the service, while SSH/Kubernetes execution remains an injected infrastructure adapter.
- [x] 2.3 Move storage HTTP adapter implementations and tests to `internal/api/infrastructure`; production routes bind the migrated handlers directly and the old root-package forwarding layer is removed.

## 3. Network Domain

- [x] 3.1 Record managed-domain, certificate, Ingress and DNS API contracts; add Network Service tests for lifecycle validation.
- [x] 3.2 Extract domain, certificate, Ingress and DNS workflows to `internal/service/network` with encrypted credential and K8s adapters; domain validation/prerequisites, certificate lifecycle calls, standard Ingress and IngressRoute operations, DNS Provider operations, DNS credential Secret synchronization, and domain certificate reconciliation now use injected service adapters.
- [x] 3.3 Move network HTTP adapter implementations and tests to `internal/api/infrastructure`, preserving routes and error mappings; production routes bind domain and certificate handlers directly.

## 4. Verification and Archive

- [x] 4.1 For every completed task, run focused tests and present results before the next task.
- [x] 4.2 Run `go test ./...`, `go build ./...`, `nvm use 24 && npm --prefix web run build`, `openspec validate api-infrastructure-migration --strict`, `git diff --check`, and security scanning for modified business files. All technical checks pass; security scan submission was attempted, but the required `sec-code` binary is unavailable in this environment.
- [x] 4.3 After user confirmation, archive the OpenSpec change and update baseline specs.
