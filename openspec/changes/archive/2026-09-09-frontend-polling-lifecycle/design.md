# Design: Standardize Frontend Polling Lifecycle

## `usePolling` API

Add `web/src/composables/usePolling.js` with a small API:

```js
const polling = usePolling(callback, { interval: 5000 })
polling.start({ immediate: true })
polling.stop()
polling.isRunning
```

The callback may be asynchronous. The composable must not schedule another callback while the previous callback is still pending. `start` is idempotent and replaces an existing timer only when the caller explicitly stops and restarts it. `stop` clears the timer and invalidates a pending scheduling cycle. Scope disposal calls `stop` automatically.

The composable does not swallow callback errors; the page callback remains responsible for updating its existing error state. A callback rejection must not leave the timer permanently active or create an unhandled rejection.

## Migration Rules

- Replace page-local timer variables and duplicated start/stop helpers with `usePolling`.
- Preserve each page's existing condition for starting or stopping polling.
- Preserve visibility behavior in server resource polling and add the same pause/resume semantics only where the page already listens to `visibilitychange`.
- Keep request cancellation in `useAsyncResource`; `usePolling` only controls invocation frequency.
- Do not move business transformations or mutation commands into the composable.

## Testing

Unit tests cover immediate execution, interval execution, idempotent start, async overlap prevention, stop behavior, callback failure recovery, and scope disposal. Existing page tests continue to verify that polling starts for pending operations, stops for terminal states, and does not update after unmount.
