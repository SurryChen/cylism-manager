## 1. VitePress application

- [x] 1.1 Add `docs/package.json` and a committed lockfile with VitePress scripts.
- [x] 1.2 Add `docs/.vitepress/config.ts` with base path, title, description, navigation, sidebar, search and excluded source paths.
- [x] 1.3 Add the Vue theme entry, layout and design-token stylesheet.

## 2. Content compatibility

- [x] 2.1 Audit public Markdown for MkDocs-only syntax and convert incompatible constructs.
- [x] 2.2 Preserve existing public URLs, image paths, Chinese labels and code examples.
- [x] 2.3 Verify excluded archive/design material is not emitted into the production output.

## 3. Visual and interaction implementation

- [x] 3.1 Implement the homepage single-column layout and product content hierarchy.
- [x] 3.2 Implement the blue branded mobile drawer, repository area, active states, arrows and overlay.
- [x] 3.3 Implement desktop navigation, search, table of contents, focus styles and responsive breakpoints.

## 4. Tooling and publishing

- [x] 4.1 Replace MkDocs workflow steps with Node/VitePress build and Pages artifact upload.
- [x] 4.2 Update documentation checks and contributor instructions for VitePress commands.
- [x] 4.3 Remove obsolete MkDocs configuration and requirements after the new build is verified.

## 5. Verification

- [x] 5.1 Run documentation structure checks and VitePress production build.
- [x] 5.2 Verify desktop and mobile navigation, search, nested sections, links and asset loading in a local preview.
- [x] 5.3 Run `git diff --check` and confirm no backend/Web UI files changed.
