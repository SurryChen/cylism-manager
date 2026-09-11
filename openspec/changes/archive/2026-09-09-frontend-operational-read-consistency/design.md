## Context

The frontend already has small domain API modules and `useAsyncResource`, but adoption is uneven. Workloads, Configs, AuditLogs, and NodeRegistryMirrors still construct request paths in views. Config and audit reads can race when the user changes context, and NodeRegistryMirrors has one page-wide error plus a long-lived apply polling loop. The goal is a local, incremental refactor that keeps current REST contracts and visual structure intact.

## Goals / Non-Goals

**Goals:**

- Make all overlapping reads cancellable and authoritative only for the latest request.
- Give each independent page region its own loading/error outcome and preserve previous successful data on refresh failure.
- Centralize request path and query encoding in a small number of domain API modules.
- Keep mutations observable with deterministic submitting state and visible failure feedback.
- Make polling start, stop, abort, and failure behavior lifecycle-safe.
- Establish focused tests that encode the above contracts.

**Non-Goals:**

- No backend API or payload changes.
- No new global state management or data-fetching dependency.
- No broad visual redesign or route restructuring.
- No full rewrite of `ChatDrawer`; only stable list-like regions may be extracted.
- No speculative splitting of already coherent API or view files.

## Decisions

1. **Use one API module per business domain.** Extend `kubernetes.js` for Kubernetes workload/config operations, add `audit.js` for audit queries, and add a node-registry-mirrors module for mirror/proxy/diagnostic/apply operations. Existing duplicate node functions will have one canonical owner and compatibility imports where needed.
   - *Alternative considered:* one file per endpoint. Rejected because it increases indirection and makes related request contracts harder to discover.

2. **Use `useAsyncResource` for reads and explicit refs for mutations.** Each independent read resource receives an `AbortSignal`; mutations retain local `submitting`/`mutationError` state and refresh the relevant resource after success.
   - *Alternative considered:* a global request store or Pinia. Rejected because state is page-local and a global store would expand the change surface.

3. **Keep previous data when a refresh fails.** Resource errors are rendered beside the affected region; an `AbortError` is ignored. Error fields are named by purpose (`listError`, `detailError`, `mutationError`, `pollingError`) rather than one page-wide string.
   - *Alternative considered:* clear data on every failure. Rejected because it causes avoidable flicker and hides the last known operational state.

4. **Model apply polling as a managed lifecycle.** Polling uses one cancellable resource/timer, starts only for active apply IDs, stops when all IDs finish, and stops on unmount. A polling error is local and must not overwrite list, proxy, or mutation errors.
   - *Alternative considered:* leave the current shared `usePolling` callback unchanged. Rejected because it cannot distinguish polling cancellation from user-visible failures without page-specific state.

5. **Extract only stable ChatDrawer regions.** Session history and approval list presentation may become child components with explicit props/events; SSE stream state, retry, and approval orchestration remain in `ChatDrawer.vue`.
   - *Alternative considered:* split by line count. Rejected because it would create cross-component coupling around the stream state.

## Risks / Trade-offs

- [Risk] Existing tests mock `api.get` paths directly and may fail after imports move. → Update tests to mock domain functions where appropriate, while retaining API-module contract tests for exact paths.
- [Risk] Some callers rely on duplicate exports from `system-components.js`. → Keep a temporary re-export or update all imports in one change, then verify with `rg` and the full test suite.
- [Risk] Abort support depends on the request layer honoring `signal`. → Pass signals through every new API function and add tests that assert the option; do not treat cancellation as an error.
- [Risk] Polling may leave a timer after a modal or route closes. → Register stop/cancel with component disposal and test unmount during an active poll.
- [Risk] ChatDrawer extraction can regress event ordering. → Extract presentation-only components first and preserve parent handlers and existing interaction tests.

## Migration Plan

1. Add or extend API modules and contract tests.
2. Migrate one page at a time with tests written before implementation.
3. Consolidate duplicate API exports and run `rg` to verify no direct calls remain in target read paths.
4. Apply the local error model and polling lifecycle.
5. Extract only approved ChatDrawer regions.
6. Run frontend tests, `vite build`, and the repository verification commands. Rollback is a normal git revert because no data or backend contract changes are introduced.

## Open Questions

- Whether the existing `usePolling` composable should gain an AbortSignal-aware overload or whether NodeRegistryMirrors should use a page-local timer wrapper. Decide during implementation after reviewing its current tests.
- Which ChatDrawer region has the smallest stable boundary after inspecting its current test coverage.
