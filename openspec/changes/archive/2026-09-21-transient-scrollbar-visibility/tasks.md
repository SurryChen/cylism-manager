## 1. Scroll visibility foundation (TDD)

- [x] 1.1 Add failing unit tests for document and nested scroll targets, per-target timer reset, and disposal cleanup in a transient-scrollbar composable test.
- [x] 1.2 Implement the lifecycle-safe transient scrollbar composable until the focused tests pass.

## 2. Application integration (TDD)

- [x] 2.1 Add a failing root-application test that proves global transient scrollbar handling is installed once and disposed with the application root.
- [x] 2.2 Install the composable from the root application and add theme-token-based idle/active scrollbar rules for Firefox and WebKit browsers.

## 3. Verification

- [x] 3.1 Run the focused composable and root-application tests, then `npm --prefix web test` and `npm --prefix web run build`.
- [x] 3.2 Manually verify desktop document scrolling, nested modal/table scrolling, and mobile drawer scrolling; confirm no layout shift and no console errors.
