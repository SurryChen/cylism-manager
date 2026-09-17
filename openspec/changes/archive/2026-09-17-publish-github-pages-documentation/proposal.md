## Why

Cylism Manager 准备以开源项目形式发布，但当前文档以仓库内的部署说明和设计记录为主。新用户无法从一个公开、可搜索的入口了解产品边界、完成安装、处理常见故障，维护者也缺少将文档稳定发布到 GitHub Pages 的机制。

## What Changes

- 使用 MkDocs Material 将仓库既有 `docs/` 目录构建为中文 GitHub Pages 文档站点。
- 新增首页、安装前置条件、脚本安装、Helm 安装、配置、首次使用、日常运维、排障和安全说明；保留既有部署指南作为内容来源并拆分为面向用户的任务型文档。
- 为可复用的产品截图建立受控的 `docs/assets/screenshots/` 目录、截图清单和脱敏规则；第一期不提交含真实环境数据的图片。
- 新增从 `main` 和 `dev` 构建、仅从 `main` 部署的 GitHub Pages 工作流；站点 URL 由 GitHub Pages 运行时配置提供，不在仓库中写死个人账户或域名。
- 更新 README，使其成为开源项目的简洁入口，指向在线文档和两种真实支持的安装路径。
- 补齐开源协作所需的贡献、安全和许可文档，其中许可证选择在实施前由维护者确认。

## Capabilities

### New Capabilities

- `public-documentation-site`: 提供可在 GitHub Pages 发布的公开文档站点、安装与运维路径、截图规范及安全边界说明。

### Modified Capabilities

- None.

## Impact

- 新增 `mkdocs.yml`、文档依赖清单、GitHub Pages workflow、用户文档、开源协作文档与截图占位目录。
- 修改 `README.md` 和现有部署文档中的公开入口、版本前置条件及内部环境信息。
- 不修改业务后端、前端运行时、Kubernetes 清单或发布镜像行为；现有 Docker 镜像发布 workflow 保持独立。
