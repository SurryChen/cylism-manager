## 1. Catalog service and API contract

- [x] 1.1 Add failing Registry catalog service tests for paginated repository listing, Tag / manifest metadata resolution, safe error mapping, and credential redaction.
- [x] 1.2 Implement the service-side OCI Distribution client using the existing managed Registry credential boundary, request timeout, bounded metadata concurrency, and opaque page continuation handling.
- [x] 1.3 Add delivery routes, request/response DTOs, handler tests, and frontend API methods for repository and Tag browsing.

## 2. Destructive catalog safety

- [x] 2.1 Add failing tests for digest-shared Tag discovery and Release / deployment-template image reference matching.
- [x] 2.2 Implement persistent reference lookup and delete preflight for Tags, manifests, and repositories; return conflicts for any protected reference or stale preflight impact.
- [x] 2.3 Add failing handler tests for explicit confirmation, remote deletion success/failure, and safe upstream error responses.
- [x] 2.4 Implement confirmed manifest and repository deletion with execution-time digest validation and audit records for success, rejection, and failure.

## 3. Registry management experience

- [x] 3.1 Add failing view tests for the primary `概览` / `镜像` header tabs, catalog loading states, repository search, Tag inspection, and pull-reference copying.
- [x] 3.2 Refactor the self-hosted Registry page to use `SectionTabsHeader`, retaining all existing overview, deploy, edit, repair, refresh, and delete-Registry behavior.
- [x] 3.3 Implement the image catalog view with pagination, search, repository / Tag detail, copy actions, and unavailable / empty / stale states.
- [x] 3.4 Add deletion preflight and confirmation dialogs that show shared-digest Tag impact and protected platform references, then refresh catalog state after successful deletion.

## 4. Verification

- [x] 4.1 Run focused Go service, handler, frontend API, and Registry view tests after each task group.
- [x] 4.2 Run the complete frontend test suite and `npm run build`.
- [x] 4.3 Run `go test ./...`, `go build ./...`, `git diff --check`, and `openspec validate managed-registry-catalog-management --strict`.
- [x] 4.4 Present verification results for review and obtain approval before archiving the change.
