# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

小团队运维/DevOps 工程师。日常通过 Web UI 管理多台服务器的 NGINX 站点配置和 SSL 证书生命周期，替代手工 SSH + 编辑配置文件 + 手动 acme.sh 的工作流。
<!-- 推断自讨论，待用户确认 -->

## Product Purpose

Cylism Manager 是一个 NGINX + acme.sh 一体化管理平台。它让运维人员通过 Web UI 集中管理多台服务器上的站点和证书，消除手工配置带来的漂移、遗忘和误操作。

## Positioning

元数据驱动 + Agent 架构的 NGINX/SSL 管理面板：站点配置是元数据的产物而非手工编辑结果，Agent 通过 gRPC 执行实际操作，所有变更可审计。
<!-- 推断自讨论 -->

## Operating Context

- 部署在单台服务器上，通过 SSH 远程管理多台被管服务器
- 被管服务器需安装 NGINX 和 acme.sh
- Agent 通过 gRPC 与 Platform 保持长连接
- 单用户/小团队场景，操作频率：日常站点上下线、证书续期

## Capabilities and Constraints

**能力：**
- NGINX 配置双向同步（元数据生成 + 反向导入）
- acme.sh 证书完整生命周期（签发/续期/吊销）
- SSH 远程部署 Agent
- 仪表盘概览 + 操作审计日志

**约束：**
- 后端 Go + SQLite，前端 Vue 3 SPA
- 单用户优先，架构预留多用户
- 一期不支持非 NGINX Web Server

**未决定：** 多用户权限模型、Agent 自动升级、DNS 托管集成
<!-- 以上均来自 openspec/changes/nginx-acme-manager/ -->

## Brand Commitments

产品名称：Cylism Manager。无现有品牌资产或视觉约束。
<!-- 推断：新项目，无既有品牌 -->

## Evidence on Hand

- OpenSpec 设计文档：`openspec/changes/nginx-acme-manager/`
- Spec 场景覆盖：server-management, site-management, cert-management, nginx-config-sync, dashboard-audit
- 无真实用户数据、案例或证言。一期为 MVP，所有内容均为功能描述而非市场声明。

## Product Principles

1. **元数据是权威来源** — 配置由元数据生成，不逆向编辑产物
2. **操作可审计** — 所有变更留下记录，可追溯可回滚
3. **降低门槛而非隐藏复杂度** — 封装 acme.sh/NGINX 操作但不黑盒化
4. **先单机再扩展** — 一期单服务器部署，架构为多服务器预留
<!-- 推断自设计讨论 -->

## Accessibility & Inclusion

无特定无障碍需求已确认。默认遵循 Web 标准可访问性。
