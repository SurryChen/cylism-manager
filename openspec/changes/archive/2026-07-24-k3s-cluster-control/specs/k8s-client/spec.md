## ADDED Requirements

### Requirement: K8s 客户端双模式初始化
平台 SHALL 支持集群内（InCluster）和集群外（环境变量）两种 K8s 连接模式，初始化失败时降级而非崩溃。

#### Scenario: InCluster 成功
GIVEN Platform 运行在 K3s Pod 内
WHEN main.go 调用 k8s.NewClient()
THEN 使用 InClusterConfig 初始化 clientset 并返回 Client

#### Scenario: InCluster 失败，使用环境变量
GIVEN Platform 不在 Pod 内且设置了 K8S_API_HOST
WHEN main.go 调用 k8s.NewClient()
THEN 使用 K8S_API_HOST:K8S_API_PORT 作为 API Server 地址

#### Scenario: K8s 不可用降级
GIVEN InCluster 和环境变量均不可用
WHEN main.go 调用 k8s.NewClient()
THEN 返回错误，main.go log 警告，api.K8s 设为 nil，不 panic

#### Scenario: K8s 未连接时 handler 降级
GIVEN api.K8s == nil
WHEN 任意 K8s handler 被调用
THEN 返回 HTTP 200，body 含 "error":"K8s 集群未连接" 和空数据
