# Design: Frontend Application API Boundary

## Technical Decisions

### Domain API ownership

Use named functions in the existing `web/src/api/applications.js` for application-workspace operations. Put application-detail-only operations in one adjacent module only if the resulting boundary contains multiple related operations; otherwise keep them in `applications.js`. Every function accepts request options as a separate final argument so reads can receive `{ signal }` without mixing transport options into a business payload.

Views will no longer assemble application endpoint paths or call the generic `api` object for these operations. Existing HTTP methods, paths, URL encoding, payload shapes, and unwrapped return values remain unchanged.

### Read lifecycle

Use `useAsyncResource` for application list, project/workspace, detail, templates, and related inventory reads that can overlap. Route or selected-application changes call `refresh`; the composable aborts the previous request and only commits the latest result. Existing polling remains separate and is stopped on scope disposal or when the selected release changes.

### Error ownership

Keep errors close to the region that can recover from them:

- workspace/list loading error;
- application detail and related-resource loading error;
- mutation error for the active form/action;
- release polling error where applicable.

The migration will not change existing success/error copy unless needed to distinguish regions.

### Test strategy

Add API unit tests for paths, payloads, encoding, and options. Extend view tests with deferred promises to prove stale results are discarded, aborts do not become visible errors, unmount stops updates, and mutation failures remain local. Preserve existing endpoint contract tests.

## Alternatives Considered

### Keep direct `api` calls in views

Rejected: it keeps endpoint knowledge and payload construction distributed across large components and makes consistent cancellation difficult.

### Introduce a global application store

Rejected for this scope: the pages do not yet require cross-route shared mutable state, and a store would add lifecycle and invalidation complexity beyond the problem.

### Split every operation into a separate API file

Rejected: tiny files would make navigation harder. Files should represent a stable domain boundary, not individual endpoints.

## Risks and Mitigations

- **Risk:** payload or path changes during extraction. **Mitigation:** copy existing calls exactly, add API contract tests, and run the full frontend test/build suite.
- **Risk:** a stale response is still committed by a non-managed helper. **Mitigation:** inventory all `watch`, route-change, and refresh call sites before editing and test deferred responses.
- **Risk:** local errors leave unrelated regions without feedback. **Mitigation:** retain a page-level fallback only where no narrower region exists and assert each critical mutation error in tests.
- **Risk:** API module grows too large. **Mitigation:** keep a single application domain module until a stable subdomain has several operations and independent tests.
