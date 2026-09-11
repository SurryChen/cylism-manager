# Design: Frontend Observability API Boundary

## Technical Decisions

### Domain API ownership

Use the existing `certificates.js` and `monitoring.js` modules for their complete page domains, including mutations. Add or complete one managed-OCI-registry module only if the current API files do not already provide a coherent owner. Domain functions accept request options separately from business payloads and preserve existing URL encoding and query parameter names.

### Managed reads and polling

Use `useAsyncResource` for certificate inventory/status reads, monitoring dashboard queries, and managed Registry status/option reads that overlap with route, tab, range, or form changes. Keep `usePolling` responsible only for timer mechanics. Each poll callback must use the current selection, avoid overlapping requests, and stop on unmount or when the monitored workflow ends.

### Error ownership and data retention

Keep independent errors for status, trends/queries, certificate operations, registry options, and registry mutations. A failed refresh must not clear the corresponding last successful data. Abort errors remain silent; actionable server and business errors retain the existing Chinese messages.

### Test strategy

Add API tests for methods, paths, query encoding, payloads, and signal forwarding. Extend page tests with deferred promises for stale-result protection, cancellation, polling stop, data retention, and mutation failure. Tests should assert that an unrelated successful region remains rendered after a local failure.

## Alternatives Considered

### Keep direct calls in the pages

Rejected: these pages combine several independent workflows, so endpoint construction and request lifecycle logic would remain duplicated and difficult to reason about.

### Introduce a shared observability store

Rejected: certificate, monitoring, and registry state have different lifecycles and do not currently require cross-route shared mutation. Local resources are sufficient and easier to dispose.

### Create a module per operation

Rejected: domain-sized modules make endpoint ownership discoverable without spreading a small page domain across many files.

## Risks and Mitigations

- **Risk:** installation or migration actions change behavior during extraction. **Mitigation:** preserve exact methods and payloads, add focused mutation tests, and run existing full page tests.
- **Risk:** polling keeps running after a tab or component is gone. **Mitigation:** use `usePolling` stop hooks and test unmount plus terminal status paths.
- **Risk:** independent errors make the template inconsistent. **Mitigation:** use explicit region names and keep one shared fallback only where a page truly has no narrower region.
- **Risk:** status reads become stale while a mutation is running. **Mitigation:** invalidate or refresh only the affected resource after a successful mutation, without clearing other data.
