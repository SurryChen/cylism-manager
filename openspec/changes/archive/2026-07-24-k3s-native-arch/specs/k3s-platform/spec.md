## ADDED Requirements

### Requirement: Platform 容器化
Platform SHALL 以容器化方式运行在 K3s 集群中，通过 Dockerfile 构建镜像，以 Deployment 方式部署。

#### Scenario: Platform 启动后访问 K8s API
- **WHEN** Platform Pod 启动
- **THEN** 通过 ServiceAccount 自动获取集群内认证，连接 K8s API Server

### Requirement: K3s 集群自举
Platform SHALL 在首次部署时作为单节点 K3s 集群启动。

#### Scenario: 初始化 K3s 集群
- **WHEN** Platform 首次部署到目标机器
- **THEN** 目标机器运行 K3s server，Platform 作为 Deployment 部署其上

### Requirement: CRD 依赖检测
系统 SHALL 在启动时检测 Traefik IngressRoute 和 cert-manager Certificate CRD 是否已安装，未安装时前端展示警告和安装入口。

#### Scenario: Traefik CRD 缺失
- **WHEN** 集群中不存在 ingressroutes.traefik.io CRD
- **THEN** 前端显示"Traefik 未安装"banner，提供一键安装按钮

#### Scenario: cert-manager CRD 缺失
- **WHEN** 集群中不存在 certificates.cert-manager.io CRD
- **THEN** 前端显示"cert-manager 未安装"banner，提供一键安装按钮

#### Scenario: 所有 CRD 已就绪
- **WHEN** Traefik 和 cert-manager CRD 均已安装
- **THEN** 不显示任何警告

### Requirement: SQLite 持久化
系统 SHALL 通过 PVC 将 SQLite 数据库持久化到节点本地路径，Pod 重建后数据不丢失。

#### Scenario: Pod 重建后数据保留
- **WHEN** Platform Pod 被删除并重建
- **THEN** PVC 重新挂载，SQLite 数据库文件完整保留
