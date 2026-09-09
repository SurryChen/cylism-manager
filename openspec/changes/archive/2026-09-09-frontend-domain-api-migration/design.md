# Design: Complete Frontend Domain API Migration

## Scope

Create small modules under `web/src/api/` for the following read domains:

- application details and related resources;
- managed domains;
- persistent storage;
- system components;
- runtimes and agent state;
- logging workspace;
- alerting workspace.

Each module exposes named functions that build the existing endpoint and query parameters, and accepts an optional request options object so `AbortSignal` can be forwarded to `api.get`.

## Page integration

Use `useAsyncResource` for initial loads and filter/route-dependent reads. A page owns the resource instance and assigns only the current result. Existing mutation handlers continue to call `api.post`, `api.put`, or `api.delete` directly, then refresh through the domain read function.

Polling must use the same managed read function and stop on component unmount. A stale or aborted result must not replace current data or error state.

## Compatibility

- Keep endpoint paths, HTTP methods, payloads, and response transformations unchanged.
- Keep public component props, routes, and user-facing text unchanged.
- Do not require callers to know the generic API client's URL construction details.

## Testing

Add module tests for URL/query construction and signal forwarding. Add page tests for route/filter races, aborted requests, and preservation of existing data on failures where the page already displays data.
