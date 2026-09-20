## Why

产品域文档已经具备可替换的截图占位符，但读者仍需在没有界面上下文的情况下理解入口、筛选条件和状态信息。现在已具备通过本地已登录 Edge 页面采集无浏览器框架截图的能力，可以用真实、脱敏的产品画面替换这些占位符。

## What Changes

- 采集并提交 18 张与现有产品页面一一对应的真实 UI 截图，使用既有 `assets/screenshots/` 文件名。
- 将对应 Markdown 页的 `ScreenshotPlaceholder` 替换为带有中文替代文本的真实图片。
- 将文档站检查扩展为验证已替换页面的图片资源和 Markdown 引用，保留占位组件供未来页面使用。
- 将截图清单改为记录已采集范围与统一脱敏、裁切和视口要求。

## Capabilities

### New Capabilities

- `public-documentation-screenshots`: 公开产品文档使用真实、已脱敏且可构建验证的产品截图解释关键界面。

### Modified Capabilities

- None.

## Impact

- 受影响内容：`docs/assets/screenshots/`、文档首页及 17 个产品 Markdown 页面、`scripts/test-docs-site.sh` 和截图清单。
- 不修改产品功能、前端路由、后端 API 或认证方式。
