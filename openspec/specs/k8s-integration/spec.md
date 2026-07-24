# k8s-integration Specification

## Purpose
TBD - created by archiving change k3s-native-arch. Update Purpose after archive.
## Requirements
### Requirement: K8s 客户端集成
系统 SHALL 通过 client-go 的 InClusterConfig 连接 K8s API，封装节点、IngressRoute 和 Certificate 的 CRUD 操作。

#### Scenario: 集群内认证
- **WHEN** Platform 作为 Pod 运行在 K3s 集群中
- **THEN** 自动读取 /var/run/secrets/kubernetes.io/serviceaccount 获取认证凭据

#### Scenario: CRD 操作
- **WHEN** 系统操作 IngressRoute 或 Certificate 资源
- **THEN** 通过 client-go dynamic client 或 typed client 进行 CRUD

### Requirement: 旧代码清理
系统 SHALL 删除所有 gRPC Agent、SSH deployer、心跳监控和 mTLS 相关代码。

#### Scenario: 代码精简
- **WHEN** 迁移完成后
- **THEN** 删除 cmd/agent/、internal/agent/、internal/service/deployer/、internal/service/server/、api/proto/ 目录及所有引用

