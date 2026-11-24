## MODIFIED Requirements

### Requirement: Platform 容器化
Platform SHALL 以容器化方式运行在兼容 Kubernetes API 的集群中，通过 Dockerfile 构建镜像，以 Deployment 方式部署；默认部署 SHALL NOT 依赖宿主机 Tailscale socket、Tailscale CLI 或特定 control-plane 主机名。

#### Scenario: Platform 启动后访问 Kubernetes API
- **WHEN** Platform Pod 启动
- **THEN** 通过 ServiceAccount 自动获取集群内认证，连接 Kubernetes API Server
- **AND THEN** Platform 启动 SHALL NOT 因 `/run/tailscale` 不存在而失败

### Requirement: SQLite 持久化
系统 SHALL 通过默认的 PersistentVolumeClaim 将 SQLite 数据库持久化，并允许操作员按集群存储能力配置存储；默认部署 SHALL NOT 使用控制面宿主机 hostPath 保存平台数据。

#### Scenario: Pod 重建后数据保留
- **WHEN** Platform Pod 被删除并重建
- **THEN** PersistentVolumeClaim 重新挂载，SQLite 数据库文件完整保留

#### Scenario: 默认 Helm 渲染
- **WHEN** 操作员使用未覆盖的 Helm values 渲染部署
- **THEN** 生成的工作负载 SHALL 使用 PVC 数据卷
- **AND THEN** 不包含 `/run/tailscale` hostPath、Tailscale 相关 volume 或硬编码节点主机名

## ADDED Requirements

### Requirement: K3s compatibility profile
系统 SHALL 将 K3s 视为 Kubernetes 兼容平台的可选增强配置，而不是启动和资源管理的全局前提。

#### Scenario: 部署到标准 Kubernetes
- **WHEN** Platform 连接到非 K3s 的 Kubernetes 集群
- **THEN** 系统 SHALL 提供通用 Kubernetes 资源管理能力
- **AND THEN** 不要求安装 K3s 或 Tailscale

#### Scenario: 部署到 K3s
- **WHEN** Platform 识别到 K3s 集群
- **THEN** 系统 SHALL 仅为已实现的 K3s 特定能力提供显式入口

## REMOVED Requirements

### Requirement: K3s 集群自举
**Reason**: Cylism Manager 管理已有的 Kubernetes 兼容集群，不再将首次部署定义为创建单节点 K3s 集群。

**Migration**: Operators install or connect an existing Kubernetes-compatible cluster before deploying the Manager; K3s remains a supported optional profile.
