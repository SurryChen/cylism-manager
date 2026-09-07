# Tasks

## 1. Backend Contract And Rendering

- [x] 1.1 Add failing API tests for Traefik timeout acceptance, rejection, values rendering for both entrypoints, and preservation of existing managed rollout fields.
- [x] 1.2 Implement typed Traefik timeout parsing and validation (positive duration, 1 minute through 1 hour), and reject the field for non-Traefik charts.
- [x] 1.3 Extend system component response state with desired/effective Traefik timeout data and render managed Helm values without duplicating arguments.
- [x] 1.4 Run focused Go tests and complete sec-code scanning for each changed business source file.

## 2. Kubernetes Effective-State Detection

- [x] 2.1 Add failing K8s tests for extracting both Traefik entrypoint timeout arguments and detecting absent or mismatched values.
- [x] 2.2 Implement Deployment argument inspection and expose pending/effective status through the system component handler.
- [x] 2.3 Run focused Go tests and complete sec-code scanning for each changed business source file.

## 3. Console

- [x] 3.1 Add failing Vue tests covering the Traefik-only timeout control, default/effective labels, and omission for other charts.
- [x] 3.2 Implement the structured select and compact status display in the existing System Components modal/table using existing design tokens.
- [x] 3.3 Run focused Vue tests and complete sec-code scanning for the changed Vue business source file.

## 4. Verification

- [x] 4.1 Run `go test ./...`, `go build ./...`, `npm --prefix web test`, `npm --prefix web run build`, `openspec validate traefik-upload-timeout-management --strict`, and `git diff --check`.
- [x] 4.2 Present verification results for user confirmation before archiving the change.
