# Proposal: Complete Frontend Domain API Migration

The frontend now has small domain API modules for applications, servers, monitoring, and dashboard reads, but several high-frequency pages still call the generic API client directly for managed reads. This leaves request construction, cancellation, and response handling inconsistent across pages.

Migrate the remaining high-frequency read paths into focused domain API modules and connect them to the existing `useAsyncResource` lifecycle. Keep command-style mutations in the owning page and preserve all existing endpoints and response shapes.

## Goals

- Make managed reads discoverable by domain and reusable by tests.
- Prevent stale responses from route changes, filter changes, and polling updates.
- Preserve existing UI behavior and backend contracts.
- Avoid introducing a global store or deeper component hierarchy.

## Non-goals

- No backend API changes.
- No migration of every low-frequency page in this change.
- No conversion of mutation commands into a new abstraction.
- No visual redesign.
