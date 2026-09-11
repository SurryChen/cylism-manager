## 1. Define composable contracts with tests

- [x] 1.1 Add `useRoutedTab.test.js` covering defaults, invalid tabs, tab selection, and preservation of unrelated query parameters
- [x] 1.2 Add `useBodyScrollLock.test.js` covering activation, restoration, nested locks, and scope disposal
- [x] 1.3 Add `useActionState.test.js` covering successful actions, failed actions, repeated runs, reset, and running cleanup

## 2. Implement reusable composables

- [x] 2.1 Implement `useRoutedTab.js` with allowed-tab validation and query-preserving navigation
- [x] 2.2 Implement `useBodyScrollLock.js` with lifecycle cleanup and safe multiple-lock handling
- [x] 2.3 Implement `useActionState.js` with running, error, result, reset, and rejection behavior

## 3. Migrate repeated page logic

- [x] 3.1 Migrate Monitoring, ClusterHub, NetworkHub, ResourceHub, and SystemSettings to `useRoutedTab`
- [x] 3.2 Migrate ServerTerminal and PodTerminal to `useBodyScrollLock` without changing terminal disposal behavior
- [x] 3.3 Migrate a bounded set of structurally similar save/install/update actions to `useActionState`, preserving page-specific error messages and refresh behavior
- [x] 3.4 Add or update page tests for query preservation, action failure recovery, and terminal cleanup

## 4. Verify the change

- [x] 4.1 Search for stale duplicated Tab and body overflow implementations in the migrated scope
- [x] 4.2 Run the frontend test suite and Vite production build
- [x] 4.3 Run OpenSpec validation and repository-level verification
