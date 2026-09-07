# Chat Approval And Session Management

## Why

Chat approvals currently present an action queue but do not reliably refresh it after a terminal server-side transition. This leaves operations visible as pending after they have failed, expired, become stale, or been resolved elsewhere. The queue also hides the stored approval history and does not correlate an operation with its originating chat session.

Runtime conversations can be created and read but cannot be renamed or removed. Operators therefore cannot keep a usable conversation list without deleting the entire Runtime PVC and its unrelated durable memory.

## What Changes

- Refresh the pending approval queue after every approval attempt and show the returned terminal status or failure reason.
- Add a bounded approval history view with status, immutable request summary, requested and approval timestamps, terminal result, and any existing originating session reference.
- Preserve existing opaque session references, while deferring new correlation propagation until Nanobot exposes a trusted tool-invocation session context.
- Render operation status and results in the chat drawer without allowing the browser or Runtime to change the approved operation.
- Extend the Runtime session contract using Nanobot's native session mechanisms: archive and title metadata use its persisted sidebar state, while permanent deletion delegates to `SessionManager.delete_session`. Exports use Nanobot's native session snapshot before an irreversible delete.

## Non-Goals

- Do not make Nanobot wait indefinitely inside a model request while an administrator reviews an approval.
- Do not permit arbitrary commands, parameters, or a replacement operation after approval.
- Do not delete `MEMORY.md`, `history.jsonl`, workspace files, or the Runtime PVC when deleting a conversation; a session may already have been compacted, so historical summaries remain separate from the deleted session file.
- Do not introduce multi-user conversation isolation or Manager-side message persistence.

## Impact

- Manager approval APIs, operation store queries, and API tests.
- Runtime session sidecar contract and session tests.
- Chat drawer, API client, and Vue tests.
