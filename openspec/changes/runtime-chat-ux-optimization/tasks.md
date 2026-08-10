## 1. OpenSpec And Contracts

- [x] 1.1 Confirm the design and scenarios against the existing runtime-chat-sessions contract.
- [x] 1.2 Extend session proxy contracts with bounded newest-page and cursor fields.

## 2. Frontend TDD

- [x] 2.1 Add failing tests for interleaved session streams and per-session stop.
- [x] 2.2 Add failing tests for thinking, failure, retry, and summary-only completion refresh.
- [x] 2.3 Add failing tests for local new-session preservation and history pagination.

## 3. Frontend Implementation

- [x] 3.1 Replace global stream/message state with session-scoped views.
- [x] 3.2 Implement explicit progress and retry states without disabling session switching.
- [x] 3.3 Refresh summaries without reloading active history after completion.
- [x] 3.4 Add older-history loading and local session merge behavior.

## 4. Runtime TDD And Implementation

- [x] 4.1 Add sidecar tests for newest-page ordering and cursors.
- [x] 4.2 Implement bounded history query parsing and newest-page reads.
- [x] 4.3 Preserve compatibility for existing no-query session requests.

## 5. Verification

- [x] 5.1 Run affected frontend and Python tests after each task.
- [x] 5.2 Run `npm --prefix web test`, `npm --prefix web run build`, `python3 -m unittest discover -s tests -v`.
- [x] 5.3 Run `go test ./...`, `go build ./...`, and `git diff --check`.

## 6. Markdown Output

- [x] 6.1 Add failing tests for safe Markdown rendering.
- [x] 6.2 Add Markdown parsing and sanitization dependencies.
- [x] 6.3 Render assistant messages as sanitized Markdown without changing the composer.
- [x] 6.4 Run focused and full frontend verification.
