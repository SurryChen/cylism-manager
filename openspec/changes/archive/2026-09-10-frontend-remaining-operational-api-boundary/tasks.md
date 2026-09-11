## 1. API Contracts

- [x] 1.1 Inventory the remaining direct operational REST calls and map domains, routes, image registries, Chart repositories and project environments to their API modules.
- [x] 1.2 Extend `domains.js`, `sites.js` and `applications.js`; add image registry and Chart repository modules with named reads and mutations that preserve existing contracts.
- [x] 1.3 Add focused API contract tests for all newly owned functions, including encoded path segments and optional request options.

## 2. Page Lifecycle And Errors

- [x] 2.1 Migrate `Domains.vue`, `Sites.vue`, `ImageRegistries.vue` and `ChartRepositories.vue` to named domain API functions without changing their workflows.
- [x] 2.2 Migrate `ProjectEnvironments.vue` to named application API functions and use `useAsyncResource` for active-project reads.
- [x] 2.3 Keep route, repository, domain and environment failures local while retaining their form, confirmation target and already loaded data.

## 3. Regression Coverage And Verification

- [x] 3.1 Add page tests for Chart repositories, route-operation failures and project-environment stale/unmount reads.
- [x] 3.2 Add mutation-failure regression tests for affected repository and environment workflows.
- [x] 3.3 Run frontend tests and build, backend regression tests and build, `openspec validate --all --strict`, and `git diff --check`; report results before archive.
