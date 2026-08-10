# Design: Runtime Chat UX Optimization

## Current Behavior

The chat component stores `messages`, `streaming`, `error`, and one `activeStream` globally. The stream callback appends to the last global message. `done` calls session-list loading, which selects the current session and reads its complete message history again. The sidecar slices the first 200 messages from a session file.

## Frontend State

Maintain an in-memory `sessionViews` map keyed by session ID and a separate `activeSessionId`. Each view owns its messages, history loading state, pagination cursor, error, and stream state. A stream callback closes over both session ID and request ID and may only mutate that view. Switching and new-session actions never abort another view's stream.

New sessions receive a UUID locally and are marked `localOnly` until the first successful Runtime response. A failed request keeps the local view and failed message for retry while the modal remains open. Local-only views are not represented as persisted Runtime history after a full page reload.

## Stream State

Assistant placeholders use `pending`, `streaming`, `completed`, `stopped`, and `error` states. `pending` renders `正在思考...`; the first delta changes it to `streaming`. Stop only aborts the matching session's request. Completion updates the local message and refreshes summaries only.

The current chat request body remains `{session_id, message}`. Optional frontend request IDs are local correlation data and are not forwarded as model content.

## Refresh And Pagination

Opening the modal loads session summaries and then the selected session's newest history page. Selecting a session reads its newest page. A successful stream refreshes summaries but does not fetch the active history again. Older history is requested explicitly with `before` and `limit` query parameters.

The sidecar history contract becomes:

```text
GET /v1/sessions/{id}/messages?limit=50&before=<cursor>
```

Message responses contain `messages`, `has_more`, and `next_cursor`. The first page is the newest page, returned in chronological order for rendering. Runtime file parsing remains internal to the sidecar.

## Failure And Retry

Network and Runtime errors mark the assistant message as failed and keep the original session ID. Retry sends the same user message again. The UI must communicate that retry can duplicate a message when the upstream result was unknown; exact-once delivery is outside this change.

## API Compatibility

Existing no-query session endpoints remain valid and return their default bounded page. Manager proxies pagination parameters without exposing Runtime credentials. Session IDs are validated as bounded opaque IDs before being used to build sidecar paths.

## Verification

- Interleaved streams for two sessions update only their own messages.
- Switching and creating sessions during a stream remains functional.
- Completion refreshes summaries without a second history request.
- History returns newest messages and supports older-page loading.
- Thinking, stopped, error, and retry states are visible and testable.
