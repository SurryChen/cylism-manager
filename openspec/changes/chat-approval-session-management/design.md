# Design: Chat Approval And Session Management

## Approval State Model

`AgentOperation` remains the immutable source of truth. Its existing request parameters, hash, resource version, status, approver, and terminal result are not rewritten by the chat UI. The API exposes two read views:

```text
GET /api/runtimes/:id/agent-operations?status=pending_approval
GET /api/runtimes/:id/agent-operations?session_id=:sid&limit=50&before=:cursor
```

The response is ordered newest first and carries a cursor for older history. A terminal transition caused by approval execution failure, expiry, stale resource detection, or another browser is represented in the response body even when the resolve endpoint returns a non-2xx result.

The chat drawer always reloads the pending query in `finally` after a resolve attempt. It shows the server message locally and never keeps a row actionable solely because a prior list response said it was pending. Each operation has a separate in-flight state.

## Session Correlation

The browser provides an opaque `session_id` only to the Runtime chat request. Existing Manager records retain a supplied session reference as informational metadata. New propagation from a Nanobot tool call is deferred: the current Nanobot tool API does not expose a trusted active-session context, and a model-controlled schema argument would permit false associations.

The correlation ID is informational: approval authorization, parameters, target versions, expiry, and execution continue to come exclusively from the stored operation. The chat drawer can display any retained reference without treating it as an authorization input.

## Session Lifecycle

Nanobot v0.3.0 already owns the relevant storage and lifecycle primitives. Sessions are JSONL files under the workspace `sessions/` directory; the SDK exposes `export`, `clear`, and `delete`, and the WebUI persists `archived_keys` and title overrides in sidebar state. The sidecar remains the authenticated adapter to those primitives. Its contract gains:

```text
PATCH  /v1/sessions/{id}       { "title": "..." }
DELETE /v1/sessions/{id}
```

Manager proxies these through JWT-protected Runtime routes. The sidecar accepts only public IDs that map to `api:<id>`. It uses Nanobot's sidebar-state helpers for archive, restore, and title overrides, so these operations do not alter a session file. It uses `SessionManager.delete_session` for permanent deletion, which invalidates Nanobot's cache and unlinks only the matching session file. Before permanent deletion, the sidecar can return Nanobot's export snapshot to the browser as an explicit backup boundary. It never follows user-provided paths and never touches durable memory files.

Nanobot's idle compaction is independent from archive. With the upstream default of 15 idle minutes, it rewrites a session to retain a recent suffix and appends an LLM summary, or a bounded raw fallback, to `memory/history.jsonl`. The original structured prefix is not recoverable from the session file after that compaction. The UI must disclose this fact before permanent deletion and must not claim that archive causes memory extraction.

The UI allows rename and delete from a session menu. Deleting a session with an active stream is blocked until that session is stopped; streams in other sessions remain unaffected. A confirmation dialog makes the memory boundary explicit.

## Alternatives Considered

- Optimistically remove a pending row without a reload: rejected because resolve can atomically move the row to a terminal failure while returning an HTTP error.
- Store chat messages or approval events in Manager: rejected because Runtime already owns conversation data and duplicated histories would drift.
- Move session files to a Cylism-specific recycle directory: rejected because Nanobot already provides a cache-safe native delete primitive and a native UI-only archive state.
- Resume the original model stream after approval: rejected because HTTP/SSE request lifetimes are bounded. The operation result is persisted and surfaced in the original conversation instead.

## Risks And Mitigations

| Risk | Mitigation |
|---|---|
| A runtime forges a different session reference | The reference is never authorization input; new propagation remains disabled until it can use trusted runtime context. |
| Upstream session storage changes | Delegate archive/title/delete/export to Nanobot's own helpers inside the sidecar and test only the stable sidecar contract. |
| A failed approval is mistaken for still-pending | Refresh authoritative pending data after every attempt and render returned terminal status. |
| History reveals sensitive operation inputs | Expose the existing sanitized summary and error summary, not raw parameter JSON or credentials. |
