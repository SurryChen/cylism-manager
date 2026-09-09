# Proposal: Standardize Frontend Polling Lifecycle

## Why

Several frontend pages implement polling independently with `setInterval` and `clearInterval`. The implementations differ in cleanup behavior, visibility handling, duplicate-request prevention, and error handling. This makes it easy for a page to keep polling after navigation or to start overlapping requests when a refresh is slower than the interval.

## What Changes

Introduce a small `usePolling` composable and migrate the existing status-refresh flows for managed domains, monitoring migration, persistent-volume operations, release details, node registry mirror application, server resource statistics, and system settings.

The composable owns timer lifecycle and guarantees that only one poll callback runs at a time. Pages continue to own business conditions such as whether an operation is pending, which resource is selected, and whether the document is hidden. Existing endpoints, intervals, UI copy, and mutation behavior remain unchanged.

## Goals

- Stop timers reliably when a component is unmounted or polling is disabled.
- Avoid overlapping polling callbacks.
- Support immediate refresh when polling starts.
- Allow pages to pause polling while the document is hidden.
- Keep polling logic local and explicit without introducing global state.

## Non-goals

- No backend changes.
- No migration of WebSocket or streaming behavior.
- No global cache, Pinia store, or query library.
- No change to polling intervals or user-facing behavior unless required for correctness.
