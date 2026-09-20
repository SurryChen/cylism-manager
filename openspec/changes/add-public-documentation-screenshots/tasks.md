## 1. Screenshot contract tests

- [ ] 1.1 Add failing documentation-site checks for the 18 approved PNG assets, their local Markdown references and removal of the corresponding placeholders.
- [ ] 1.2 Update the screenshot inventory with the fixed viewport, full-screen page-crop and data-safety contract.

## 2. Capture and replacement

- [ ] 2.1 Capture, review and add the homepage plus application-delivery screenshots; replace their placeholders with accessible Markdown images.
- [ ] 2.2 Capture, review and add the supply-chain and platform screenshots; replace their placeholders with accessible Markdown images.
- [ ] 2.3 Capture, review and add the operations, governance and automation screenshots; replace their placeholders with accessible Markdown images.

## 3. Verification

- [ ] 3.1 Run the documentation structure check and VitePress production build.
- [ ] 3.2 Inspect the documentation homepage and representative product pages in desktop and mobile layouts; confirm images render, remain readable and do not expose browser chrome or sensitive data.
- [ ] 3.3 Run `git diff --check` and `openspec validate add-public-documentation-screenshots --strict`; present the results and obtain approval before archiving.
