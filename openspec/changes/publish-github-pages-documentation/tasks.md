## 1. Documentation foundation (TDD)

- [x] 1.1 Add failing documentation-site checks that require a MkDocs configuration, an explicit Chinese navigation, a checked-in documentation dependency manifest, and a strict build command.
- [x] 1.2 Add the MkDocs Material configuration and dependency manifest; ensure local builds use the existing `docs/` directory without copying content.
- [ ] 1.3 Run the focused documentation-site checks and a failing-then-passing strict MkDocs build.

## 2. Public user documentation (TDD)

- [x] 2.1 Add failing checks for the public documentation information architecture: product boundary, prerequisites, script install, Helm install, configuration, first use, operations, troubleshooting and security.
- [x] 2.2 Write the task-oriented user documentation, deriving commands from the current script and Helm Chart; avoid unsupported installation paths and private environment details.
- [x] 2.3 Update README with the accurate Go version, concise public entry points, supported installation links and high-privilege deployment warning.
- [x] 2.4 Add screenshot inventory and redaction guidance with placeholders only; do not commit unreviewed screenshots.
- [ ] 2.5 Run focused documentation checks and inspect desktop/mobile local documentation previews.

## 3. Pages publication and open-source governance (TDD)

- [x] 3.1 Add failing checks for an isolated GitHub Pages workflow with main/dev/manual triggers, least-privilege permissions and main-only artifact deployment.
- [x] 3.2 Add the Pages workflow and verify it does not modify the existing image release workflow.
- [x] 3.3 Add `CONTRIBUTING.md` and `SECURITY.md`; add the maintainer-approved standard license text.
- [ ] 3.4 Run focused workflow/governance checks and inspect the generated navigation and deployment artifact locally.

## 4. Verification

- [ ] 4.1 Run strict MkDocs build, complete frontend tests/build, backend tests/build, strict OpenSpec validation and `git diff --check`.
- [ ] 4.2 Present the documentation preview and all verification results; obtain approval before archiving.
