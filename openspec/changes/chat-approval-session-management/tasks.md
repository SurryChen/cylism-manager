## 1. Contracts And Store

- [x] 1.1 Add Go tests for operation-list status/session filters and sanitized response data.
- [x] 1.2 Implement a bounded filtered Agent operation history using the existing 50-record display limit.
- [ ] 1.3 Add session-correlation propagation only when Nanobot provides trusted tool-invocation context.

## 2. Runtime Session Lifecycle

- [x] 2.1 Add Python tests for native archive/restore/title metadata, exported snapshots, permanent deletion, active-list removal, and invalid session ID rejection.
- [x] 2.2 Implement sidecar calls to Nanobot sidebar-state and `SessionManager` lifecycle helpers without affecting durable memory files.
- [x] 2.3 Add Manager runtime client and proxy routes for rename/archive/restore/export/delete with Go tests.

## 3. Chat UX

- [x] 3.1 Add Vue tests covering pending-queue refresh after resolve errors and visible resolve feedback.
- [x] 3.2 Add approval history tabs and status/result rendering.
- [x] 3.3 Add rename/archive/restore/export/delete session controls, confirmation, active-stream guard, and list refresh behavior.

## 4. Verification

- [ ] 4.1 Run focused Go, Python, and Vue tests while implementing each task.
- [ ] 4.2 Run `go test ./...`, `go build ./...`, `npm --prefix web test`, `npm --prefix web run build`, Python session tests, and `git diff --check`.
