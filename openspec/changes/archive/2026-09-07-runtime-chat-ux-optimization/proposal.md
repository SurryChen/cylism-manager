# Runtime Chat UX Optimization

## Why

Runtime chat currently keeps one global message and stream state. A user who switches sessions while a response is streaming can see deltas rendered into the wrong session. Every completed response also reloads the current session history, and the runtime session proxy returns only the oldest 200 messages.

## What Changes

- Isolate messages, loading state, errors, abort handles, and stream request IDs by session ID.
- Keep switching and creating sessions available while another session streams.
- Show explicit thinking, stopped, and retryable failure states.
- Refresh session summaries after a response without reloading the full active history.
- Add cursor-based session history pagination and return the newest messages first.
- Keep local new-session state until its first request is persisted by the Runtime.

## Non-Goals

- No Manager-side conversation persistence.
- No exact-once delivery guarantee for retrying a request whose upstream result is unknown.
- No change to nanobot's model or memory semantics.

## Impact

- `web/src/components/ChatDrawer.vue` and tests.
- `web/src/api/index.js` and tests where pagination contracts are covered.
- `cylism-nanobot-runtime/cylism_session_api.py` and tests.
- Runtime chat OpenSpec contract updates.
