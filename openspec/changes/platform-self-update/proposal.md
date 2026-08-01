# 平台自更新

## Summary

允许 GitHub Actions 在构建并推送镜像后主动通知 Cylism，将平台自身 Deployment 更新到不可变镜像 digest。平台不需要访问 GitHub；只要求 GitHub Runner 能访问 `https://cylism.crazycoding.top`，平台和节点能访问配置的镜像仓库。

## Capabilities

- 新增 `platform-self-update`：安全接收外部自动部署请求、更新受限的平台 Deployment、展示发布和回滚状态。
- 修改 `system-settings`：管理部署 Webhook 密钥、平台镜像仓库允许前缀和当前部署状态。
- 修改 `.github/workflows/docker-image.yml`：推送成功后使用 GitHub Secret 向平台提交 SHA 标签对应的镜像 digest。

## Goals

- 使用 `image@sha256:...` 部署，避免 `latest` 标签漂移。
- 将发布请求、旧镜像、目标镜像与运行状态持久化，平台重启后可继续查询和收敛结果。
- 使用独立 HMAC Webhook 凭据，不将管理员 JWT 暴露给 GitHub Actions。
- 限制自更新只能修改固定的平台工作负载，不能成为任意 Kubernetes Deployment 写入入口。

## Non-goals

- 不拉取 GitHub 源码、不访问 GitHub API、不支持 GitHub 以外的 CI 集成首期配置界面。
- 不自动回滚；首期提供受确认的人工回滚，避免网络或镜像仓库瞬态问题触发反复切换。
- 不管理 Helm、Kustomize 或多个平台副本的升级策略。
