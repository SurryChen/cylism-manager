# k3s-node Specification

## Purpose
TBD - created by archiving change k3s-native-arch. Update Purpose after archive.
## Requirements
### Requirement: 服务器注册
系统 SHALL 保留服务器注册功能，允许用户添加远程机器（IP + SSH 凭据），不要求立即加入 K3s 集群。

#### Scenario: 注册服务器
- **WHEN** 用户提交服务器名称、IP 地址和 SSH 凭据
- **THEN** 系统存储服务器记录，cluster_role 为空

#### Scenario: 服务器升级为集群节点
- **WHEN** 用户对已注册服务器触发"加入集群"操作
- **THEN** 系统 SSH 安装 k3s agent，成功后更新 cluster_role 和 k8s_node_name

### Requirement: 节点添加
系统 SHALL 通过 SSH 在远程机器上安装 K3s agent 并将其加入集群。

#### Scenario: SSH 安装 k3s agent
- **WHEN** 用户输入目标 IP、SSH 凭据并触发添加节点
- **THEN** 系统 SSH 连接到目标机器，执行 k3s agent 安装脚本，节点加入集群并显示在节点列表

#### Scenario: 节点加入失败回报错
- **WHEN** SSH 连接失败或安装脚本返回错误
- **THEN** 系统返回具体错误信息

### Requirement: 节点列表
系统 SHALL 展示集群中所有节点的名称、IP、状态、角色、K8s 版本和资源使用情况。

#### Scenario: 查看节点列表
- **WHEN** 用户访问节点管理页面
- **THEN** 系统列出所有 K8s Node，包含 Ready 状态、角色标签、CPU/内存使用

### Requirement: 节点驱逐
系统 SHALL 支持安全驱逐节点上的工作负载。

#### Scenario: 驱逐节点
- **WHEN** 用户对某工作节点触发驱逐
- **THEN** 系统执行 kubectl drain，将 Pod 迁移到其他节点

### Requirement: 节点删除
系统 SHALL 支持从集群中移除节点。

#### Scenario: 删除节点
- **WHEN** 用户确认删除已驱逐的节点
- **THEN** 系统调用 K8s API 删除 Node 资源，节点从列表移除

### Requirement: API 响应格式
该 capability 的所有 API 响应 SHALL 使用统一的 APIResponse 格式，包含 code/message/data 字段，替代原有裸 gin.H 或裸对象返回。

#### Scenario: 响应使用统一格式
- **WHEN** 调用该 capability 的任意 API
- **THEN** 响应 body 必须是 `{"code": 0, "message": "ok", "data": ...}` 格式

