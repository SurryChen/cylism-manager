## Context

The existing frontend convention is a flat module per operational domain under `web/src/api/`, with views responsible for UI state and named API functions responsible for HTTP method, path, encoding, payload, and request options. Most views follow that convention. Remaining exceptions are direct calls in Login, Runtime Management, DB Admin, and Certificate Operations. In addition, `ChatDrawer.vue` uses named Runtime chat functions, but those functions are currently mixed into `api/index.js`, which should remain the transport, token, refresh, and response-unwrapping boundary.

`RuntimeManagement.vue` already consumes read functions from `runtimes.js`; its mutations and Agent actions remain direct. `DBAdmin.vue` already uses cancellable reads from `admin.js`; its mutations remain direct. `CertificateOperations.vue` loads only on mount, so a reused route component can display an earlier certificate's result while the next request is pending. `Login.vue` directly constructs the two authentication requests.

## Goals / Non-Goals

**Goals:**

- Make every endpoint used by the affected production views reachable through an explicit, domain-named API function.
- Keep `api/index.js` limited to transport concerns and token primitives, moving Runtime chat and Agent endpoints to `runtimes.js` and authentication requests to `auth.js`.
- Preserve each request's method, path, path encoding, query string, body, response shape, authentication handling, and existing visible workflow.
- Use `useAsyncResource` for certificate-operation reads so route changes and component disposal cannot commit stale state.
- Preserve active forms, confirmation targets, loaded data, and visible local errors on rejected mutations.
- Establish focused regression tests before the implementation is considered complete.

**Non-Goals:**

- No backend endpoint, response, authorization, token, or data-model change.
- No global state store, generic operation registry, extra API directory layer, or arbitrary view/component splitting.
- No redesign of the Runtime management screen, Chat Drawer, login flow, database editor, or certificate detail UI.
- No changes to chat streaming protocol semantics beyond moving its owner module.

## Decisions

### Keep one flat API module per domain

Add `auth.js` for login calls; extend `runtimes.js` with lifecycle, Agent, session, and SSE chat functions; extend `admin.js` with table record writes; and extend `certificates.js` with certificate-operation reads. This makes module ownership obvious without increasing directory nesting.

The alternative is to retain generic `api` imports in special views or create a generic `operations.js`. The former leaves endpoint ownership inconsistent. The latter obscures business ownership and conflicts with the existing module pattern, so neither is selected.

### Preserve transport and token primitives in `api/index.js`

`api/index.js` continues to own `api`, token read/write/clear functions, refresh coordination, and response unwrapping. `auth.js` calls the transport for login endpoints; Login can retain direct access to `setTokens`. Runtime chat functions move out because they encode Runtime-specific routes and SSE parsing.

The alternative is to move all auth primitives into `auth.js`. That would create avoidable circular imports or a second transport boundary, while token refresh is used by every domain module.

### Use a route-keyed resource for certificate operations

`CertificateOperations.vue` will request operations through a `useAsyncResource` loader parameterized by namespace and certificate name. Route parameter changes trigger a new request; stale and unmounted requests are aborted or ignored by the existing composable. The page commits only the active request result and treats aborts as non-errors.

The alternative is an `onMounted` request with a route watcher and manual request version counter. It duplicates the lifecycle logic already standardized in the application and makes loading/error cleanup easier to regress.

### Keep mutation state local to the initiating view

Domain functions remain stateless. Runtime, database, and login workflows retain their existing local loading, notice, form, and confirmation state. A rejected request reports its local error and does not close or reset the initiating UI state.

The alternative is a global error notification or mutation store. It would broaden scope and risks losing the direct relationship between an error and its retryable action.

## Risks / Trade-offs

- [Moving wrappers changes an HTTP contract] -> Add API contract tests before migration, including encoded identifiers, optional options, body, and SSE behavior.
- [Runtime chat import migration leaves stale exports or consumers] -> Search all imports of the moved symbols, update every production consumer, and remove feature-specific exports from `api/index.js` once tests cover the new module.
- [Certificate route changes show stale data or an abort error] -> Test deferred requests for route replacement and component unmount, using `useAsyncResource` behavior as the acceptance boundary.
- [Database or Runtime mutation failures close retry UI] -> Add focused view tests that reject writes and assert retained form or confirmation state with visible feedback.
- [Scope becomes a broad refactor] -> Restrict changes to the five identified workflows and existing flat modules.

## Migration Plan

1. Add red API contract and lifecycle tests for each missing named function and affected view behavior.
2. Implement the smallest module extensions, then migrate production imports and remove feature-specific endpoint functions from `api/index.js`.
3. Run focused frontend tests after each task, followed by frontend tests/build, Go tests/build, strict OpenSpec validation, and diff checks.
4. Rollback is a source-only revert of the API wrappers and calling imports; no backend, database, or deployment migration is required.

## Open Questions

- None.
