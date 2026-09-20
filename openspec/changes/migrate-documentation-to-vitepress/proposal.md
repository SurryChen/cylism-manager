## Why

Cylism Manager 当前使用 MkDocs Material 构建公开文档。现有内容可用，但首页布局和导航需要较多主题覆盖才能达到产品截图中的品牌抽屉、桌面单栏和响应式体验。VitePress 基于 Vue，能在不改变 Markdown 内容组织的前提下提供更直接的布局与组件定制能力。

## What Changes

- 将公开文档构建工具从 MkDocs Material 迁移到 VitePress。
- 保留现有公开文档路径、中文内容、图片资源和 GitHub Pages 发布目标。
- 用 VitePress 配置重建顶部导航、侧边栏、搜索、代码块和页面目录。
- 用 Vue 主题覆盖实现首页、品牌头部、移动端抽屉导航和桌面端布局。
- 将文档构建检查和 GitHub Actions 发布流程切换为 Node/VitePress。
- 排除 `archive/`、`design/`、历史部署文档和截图清单，不将内部资料生成到公开站点。

## Non-Goals

- 不修改 Go 后端、`web/` 前端或业务 API。
- 不重写现有文档的产品内容，只迁移不兼容的 MkDocs 专用语法。
- 不在本次变更中引入 CMS、在线编辑或多语言文档体系。

## Impact

- 文档：新增 `docs/.vitepress/` 主题与配置，现有 Markdown 仅作兼容性调整。
- 工具链：新增文档专用 Node 依赖和锁文件，移除文档发布对 Python/MkDocs 的依赖。
- CI：GitHub Pages 构建命令、缓存和产物目录切换为 VitePress。
- 验证：文档结构检查、构建、链接和响应式导航需要重新覆盖。
