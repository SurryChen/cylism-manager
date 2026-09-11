# Proposal: Harden Frontend Page State and Request Handling

## Why

The frontend has already standardized high-frequency requests and polling in several core views, but a number of pages still manage requests, errors, authentication refreshes, and large UI regions independently. This creates inconsistent loading behavior, makes stale responses easier to render, leaves important workflows under-tested, and keeps terminal/chart dependencies in larger-than-necessary route chunks.

## What Changes

- Migrate remaining data-heavy views to named domain API functions and `useAsyncResource` where request cancellation and latest-result handling are needed.
- Make access-token refresh single-flight and standardize request errors without changing REST endpoints.
- Extract only coherent UI boundaries from oversized views while keeping page components responsible for business orchestration.
- Add focused tests for high-risk pages, API modules, loading/error states, mutations, and authentication transitions.
- Lazily load optional terminal/chart capabilities where practical and remove confirmed empty or obsolete frontend artifacts.

## Goals

- Prevent stale or unmounted page requests from updating visible state.
- Keep authentication stable when several requests expire at the same time.
- Make large pages easier to maintain without creating arbitrary one-function files.
- Protect deployment and administration workflows with regression tests.
- Reduce initial route payloads without changing user-facing behavior.

## Non-goals

- No backend endpoint, payload, or response contract changes.
- No Pinia store, global data cache, or adoption of a query library.
- No broad visual redesign or route hierarchy change.
- No extraction of components that do not represent a reusable or independently testable UI boundary.
