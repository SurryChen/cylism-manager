## 1. Resource APIs and Authorization

- [x] 1.1 Add failing API and Kubernetes client tests for ConfigMap and Opaque Secret create/update/delete, Namespace ownership and Secret redaction.
- [x] 1.2 Implement resource CRUD APIs, audit entries and resource metadata/key listing without exposing Secret values.
- [x] 1.3 Add failing tests for template/release reference lookup and deletion protection.
- [x] 1.4 Implement reusable dependency resolution, deletion protection and publish-time dependency diagnostics.

## 2. Certificate Dependencies

- [x] 2.1 Add failing tests for listing environment-scoped managed TLS certificates and validating `tls.crt` / `tls.key` Secret keys.
- [x] 2.2 Implement template certificate selection metadata and reuse the managed-domain certificate request path from the application context.
- [x] 2.3 Add release preflight tests for pending, failed, missing and Ready certificate dependencies.

## 3. Resource Management UI

- [x] 3.1 Add ConfigMap and Opaque Secret create/edit/delete flows with masked Secret editing and reference-aware delete states.
- [x] 3.2 Add focused frontend tests for resource creation, masked Secret updates and delete protection errors.

## 4. Template and Application UI

- [x] 4.1 Replace free-text file source names with Namespace-scoped resource and key selectors.
- [x] 4.2 Add a reusable quick-create resource dialog that returns the selected resource to the template form.
- [x] 4.3 Add a generic TLS certificate select/request workflow that generates the two standard Secret mounts without creating Ingress.
- [x] 4.4 Add an application dependency status view and frontend tests for Ready, pending and missing dependencies.

## 5. Verification

- [x] 5.1 Run gofmt and focused Go tests after each backend task, including callers affected by API changes.
- [x] 5.2 Run go test ./... and go build ./... before completion.
- [x] 5.3 Run npm --prefix web test -- --run and npm --prefix web run build.
- [x] 5.4 Submit required security scans for every modified business code file and run git diff --check.
