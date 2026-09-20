## Context

仓库根目录是 Go 项目，Web 应用位于 `web/`，公开文档位于 `docs/`。当前 MkDocs 配置负责文档导航、中文界面、搜索、代码复制和 GitHub Pages 发布。迁移需要避免把文档依赖混入 Web 应用依赖，也不能让归档设计文档被意外发布。

## Decision

### 1. 文档应用边界

在 `docs/` 内维护独立的 VitePress 文档应用：

```text
docs/
  .vitepress/
    config.ts
    theme/
      index.ts
      Layout.vue
      styles.css
  assets/
  *.md
  package.json
```

使用 `npm --prefix docs` 执行开发、构建和预览，避免修改 `web/package.json`。

### 2. 路由与内容

- 保留现有 Markdown 文件路径，使公开链接和跨页引用保持稳定。
- VitePress 使用 `srcExclude` 排除 `archive/**`、`design/**`、`k3s-tailscale-deployment.md` 和 `assets/screenshots/README.md`。
- 使用 `cleanUrls` 和 GitHub Pages base path；本地开发使用 `/`，CI 根据 `SITE_URL` 或仓库名计算部署前缀。
- 对 MkDocs 专用的属性列表、HTML 容器或扩展语法逐页处理，优先转换为标准 Markdown 或 VitePress 容器语法。

### 3. 导航与视觉

- 在 `config.ts` 中集中定义顶部导航、侧边栏分组、页面标题和仓库链接。
- 主题层保留 VitePress 默认的搜索、键盘可访问性、面包屑和目录能力。
- `Layout.vue` 与 `styles.css` 负责品牌 Logo、蓝色抽屉头部、仓库信息区、白色菜单行、当前项状态、子菜单箭头、遮罩层和首页单栏布局。
- 桌面端隐藏首页重复的左右导航栏；移动端保留可展开的侧边抽屉。
- 颜色、间距、断点和焦点样式通过 CSS 变量集中管理。

### 4. 发布与验证

- GitHub Actions 使用 Node.js，执行 `npm ci --prefix docs` 和 `npm run build --prefix docs`。
- VitePress 输出 `docs/.vitepress/dist`，上传该目录到 GitHub Pages。
- 保留一个文档站检查脚本，验证关键文件、导航入口、排除规则、公开文档敏感内容和构建产物。
- 验证顺序：文档检查、VitePress 构建、链接检查、桌面/移动截图检查、`git diff --check`。

## Alternatives Considered

- 继续覆盖 MkDocs Material：改动初期较少，但首页和抽屉导航会持续依赖主题内部 DOM，定制成本和脆弱性较高。
- 将文档迁入仓库根目录的 VitePress：配置更常见，但会把文档依赖和 Go/Web 项目混在一起，不利于现有仓库边界。

## Rollback

在 VitePress 构建和页面验收完成前保留 MkDocs 配置与发布文件；迁移完成后再删除旧配置。若迁移失败，可恢复 `mkdocs.yml`、`requirements-docs.txt` 和 MkDocs workflow，Markdown 内容不受影响。
