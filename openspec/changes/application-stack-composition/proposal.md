# 应用栈编排

## Summary

将当前“一个应用包含多个 Kubernetes 组件”的应用栈实现改为项目环境级的应用组合。应用始终对应一个独立服务；应用栈负责一次性创建、维护和发布多个彼此关联的应用。

## Goals

- 保持 Application 是独立的 Deployment、Service、发布记录、域名与可观测单元。
- 将 Stack Template 归属到 Environment，使它可编排多个独立应用。
- 将 Karakeep 作为内置栈模板，而非应用详情页的专用组件模型。
- 支持按组件独立指定版本，以独立 Application Release 执行发布。
- 保留现有单应用模板、PVC、ConfigMap、Secret、域名和发布 API。

## Non-goals

- 不实现跨环境或跨项目的应用栈。
- 不引入任意 YAML、Helm 或 Compose 作为栈定义输入。
- 不实现通用依赖图、自动回滚或跨应用事务性发布。

## Migration

当前未提交的应用内栈模板和发布记录不进入正式模型。开发数据库由 AutoMigrate 生成新表；已存在的单应用、模板和发布不受影响。
