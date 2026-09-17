#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  [[ -f "$1" ]] || { echo "缺少文档站文件: $1" >&2; exit 1; }
}

for file in \
  mkdocs.yml \
  requirements-docs.txt \
  CONTRIBUTING.md \
  SECURITY.md \
  LICENSE \
  docs/index.md \
  docs/getting-started.md \
  docs/installation/prerequisites.md \
  docs/installation/install-script.md \
  docs/installation/install-helm.md \
  docs/operations/configuration.md \
  docs/operations/troubleshooting.md \
  docs/security.md \
  docs/assets/screenshots/README.md \
  .github/workflows/docs-pages.yml; do
  require_file "$file"
done

grep -q 'mkdocs-material' requirements-docs.txt
grep -q 'site_name: Cylism Manager' mkdocs.yml
grep -q 'repo_url: https://github.com/SurryChen/cylism-manager' mkdocs.yml
grep -q 'archive/\*\*' mkdocs.yml
grep -q 'design/\*\*' mkdocs.yml
grep -q 'assets/screenshots/README.md' mkdocs.yml
grep -q '安装部署:' mkdocs.yml
grep -q '使用指南:' mkdocs.yml
grep -q '运维与排障:' mkdocs.yml
grep -q '安全:' mkdocs.yml
grep -q 'actions/configure-pages@' .github/workflows/docs-pages.yml
grep -q 'actions/upload-pages-artifact@' .github/workflows/docs-pages.yml
grep -q 'actions/deploy-pages@' .github/workflows/docs-pages.yml
grep -q 'workflow_dispatch:' .github/workflows/docs-pages.yml
grep -q 'mkdocs build --strict' .github/workflows/docs-pages.yml
grep -q 'branches: \[main, dev\]' .github/workflows/docs-pages.yml
grep -q "if: github.ref == 'refs/heads/main'" .github/workflows/docs-pages.yml
grep -q 'mkdocs build --strict' CONTRIBUTING.md
grep -q 'Apache License' LICENSE
grep -q 'Go 1.25+' README.md
grep -q '1440 x 900' docs/assets/screenshots/README.md
grep -q '不得包含' docs/assets/screenshots/README.md

if rg -q 'assets/screenshots/README.md' docs --glob '*.md' --glob '!assets/screenshots/README.md'; then
  echo '公开文档不应链接到已排除的截图清单。' >&2
  exit 1
fi

if grep -R -E 'crpi-c5u9bb8i5qxw1m72|CYLISM_DEV_DEPLOY|ACR_(USERNAME|PASSWORD)' \
  README.md docs --exclude-dir=archive; then
  echo '公开文档不应包含内部镜像仓库或测试部署配置。' >&2
  exit 1
fi

echo '文档站结构检查通过。'
