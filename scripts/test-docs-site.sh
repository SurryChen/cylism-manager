#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  [[ -f "$1" ]] || { echo "缺少文档站文件: $1" >&2; exit 1; }
}

for file in \
  docs/package.json \
  docs/package-lock.json \
  docs/.vitepress/config.ts \
  docs/.vitepress/theme/index.ts \
  docs/.vitepress/theme/Layout.vue \
  docs/.vitepress/theme/styles.css \
  docs/.vitepress/theme/components/ScreenshotPlaceholder.vue \
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
  docs/application-delivery/projects-and-environments.md \
  docs/application-delivery/application-workspace.md \
  docs/application-delivery/releases.md \
  docs/application-delivery/domains-and-access.md \
  docs/supply-chain/managed-registry.md \
  docs/supply-chain/registry-proxy.md \
  docs/supply-chain/node-registry-mirrors.md \
  docs/supply-chain/helm-chart-repositories.md \
  docs/platform/servers-and-terminal.md \
  docs/platform/cluster-and-system-components.md \
  docs/platform/kubernetes-resources.md \
  docs/platform/network-and-certificates.md \
  docs/platform/storage.md \
  docs/operations/metrics.md \
  docs/operations/logs.md \
  docs/operations/alerts.md \
  docs/operations/disk-growth.md \
  docs/operations/maintenance-and-troubleshooting.md \
  docs/governance/audit-logs.md \
  docs/governance/operation-history.md \
  docs/governance/system-settings.md \
  docs/governance/database-management.md \
  docs/governance/identity-permissions-security.md \
  docs/automation/agent-assistant.md \
  docs/automation/automated-operations.md \
  docs/assets/screenshots/README.md \
  .github/workflows/docs-pages.yml; do
  require_file "$file"
done

grep -q '"vitepress"' docs/package.json
grep -q '"build": "vitepress build ."' docs/package.json
grep -q "srcExclude" docs/.vitepress/config.ts
grep -q "provider: 'local'" docs/.vitepress/config.ts
grep -q '首页' docs/.vitepress/config.ts
grep -q '应用交付' docs/.vitepress/config.ts
grep -q '制品与供应链' docs/.vitepress/config.ts
grep -q '资源与平台' docs/.vitepress/config.ts
grep -q '可观测与运维' docs/.vitepress/config.ts
grep -q '治理与系统' docs/.vitepress/config.ts
grep -q '自动化助手' docs/.vitepress/config.ts
grep -q 'ScreenshotPlaceholder' docs/.vitepress/theme/index.ts
grep -q 'VPNavBarAppearance' docs/.vitepress/theme/styles.css
grep -q 'pointer-events: auto' docs/.vitepress/theme/styles.css
grep -q "darkModeSwitchTitle: '切换深色模式'" docs/.vitepress/config.ts
grep -q "lightModeSwitchTitle: '切换浅色模式'" docs/.vitepress/config.ts
grep -q 'actions/setup-node@' .github/workflows/docs-pages.yml
grep -q 'npm ci --prefix docs' .github/workflows/docs-pages.yml
grep -q 'npm run build --prefix docs' .github/workflows/docs-pages.yml
grep -q 'docs/.vitepress/dist' .github/workflows/docs-pages.yml
grep -q 'branches: \[main, dev\]' .github/workflows/docs-pages.yml
grep -q "if: github.ref == 'refs/heads/main'" .github/workflows/docs-pages.yml
grep -q 'npm run build --prefix docs' CONTRIBUTING.md
grep -q 'Apache License' LICENSE
grep -q 'Go 1.25+' README.md
grep -q 'https://surrychen.github.io/cylism-manager/' README.md
grep -q 'actions/workflows/docs-pages.yml/badge.svg' README.md
grep -q 'license-Apache--2.0' README.md
grep -q '1440 x 900' docs/assets/screenshots/README.md
grep -q '不得包含' docs/assets/screenshots/README.md

if rg -q 'assets/screenshots/README.md' docs --glob '*.md' --glob '!assets/screenshots/README.md'; then
  echo '公开文档不应链接到已排除的截图清单。' >&2
  exit 1
fi

if ! rg -q '<ScreenshotPlaceholder' \
  docs/application-delivery \
  docs/supply-chain \
  docs/platform \
  docs/operations \
  docs/governance \
  docs/automation; then
  echo '产品文档必须包含可替换的截图占位组件。' >&2
  exit 1
fi

if rg -n 'filename="(?!assets/screenshots/)[^"]+"' \
  docs/application-delivery \
  docs/supply-chain \
  docs/platform \
  docs/operations \
  docs/governance \
  docs/automation --pcre2; then
  echo '截图占位文件名必须位于 assets/screenshots/ 下。' >&2
  exit 1
fi

if rg -q 'python -m mkdocs|mkdocs-material|mkdocs.yml|requirements-docs' \
  README.md CONTRIBUTING.md SECURITY.md docs .github/workflows --glob '!docs/archive/**'; then
  echo '公开文档站不应继续引用 MkDocs 构建链。' >&2
  exit 1
fi

if grep -R -E 'crpi-c5u9bb8i5qxw1m72|CYLISM_DEV_DEPLOY|ACR_(USERNAME|PASSWORD)' \
  README.md docs --exclude-dir=archive; then
  echo '公开文档不应包含内部镜像仓库或测试部署配置。' >&2
  exit 1
fi

echo 'VitePress 文档站结构检查通过。'
