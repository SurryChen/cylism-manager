## 1. Resource APIs and Authorization

- [ ] 1.1 Add failing API and Kubernetes client tests for ConfigMap and Opaque Secret create/update/delete, Namespace ownership and Secret redaction.
- [ ] 1.2 Implement resource CRUD APIs, audit entries and resource metadata/key listing without exposing Secret values.
- [ ] 1.3 Add failing tests for template/release reference lookup and deletion protection.
- [ ] 1.4 Implement reusable dependency resolution, deletion protection and publish-time dependency diagnostics.

## 2. Certificate Dependencies

- [ ] 2.1 Add failing tests for listing environment-scoped managed TLS certificates and validating `tls.crt` / `tls.key` Secret keys.
- [ ] 2.2 Implement template certificate selection metadata and reuse the managed-domain certificate request path from the application context.
- [ ] 2.3 Add release preflight tests for pending, failed, missing and Ready certificate dependencies.

## 3. Resource Management UI

- [ ] 3.1 Add ConfigMap and Opaque Secret create/edit/delete flows with masked Secret editing and reference-aware delete states.
- [ ] 3.2 Add focused frontend tests for resource creation, masked Secret updates and delete protection errors.

## 4. Template and Application UI

- [ ] 4.1 Replace free-text file source names with Namespace-scoped resource and key selectors.
- [ ] 4.2 Add a reusable quick-create resource dialog that returns the selected resource to the template form.
- [ ] 4.3 Add a generic TLS certificate select/request workflow that generates the two standard Secret mounts without creating Ingress.
- [ ] 4.4 Add an application dependency status view and frontend tests for Ready, pending and missing dependencies.

## 5. Verification

- [ ] 5.1 Run gofmt and focused Go tests after each backend task, including callers affected by API changes.
- [ ] 5.2 Run go test ./... and go build ./... before completion.
- [ ] 5.3 Run npm --prefix web test -- --run and npm --prefix web run build.
- [ ] 5.4 Submit required security scans for every modified business code file and run git diff --check.
