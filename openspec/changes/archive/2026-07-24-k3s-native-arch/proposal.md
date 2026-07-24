## Why

当前架构基于自研 gRPC Agent + SSH 部署 + mTLS + 手动 NGINX/acme.sh 管理，每个组件都需要独立实现和维护。K3s 生态已内置了节点管理（kubelet）、流量管理（Traefik）、证书管理（cert-manager）、容器运行时（containerd），直接复用这些成熟组件可以大幅减少自研代码量，提升可靠性。

目标：Cylism Manager 成为 K3s 集群的 Web 管理面板，自身运行在 K3s 上，通过 K8s API 管理一切。

## What Changes

**新增：**
- Platform 容器化，Dockerfile + K3s Deployment + PVC 持久化
- K3s 集群初始化（单节点 → 扩展多节点）
- 服务器 → 节点分阶段管理（先注册服务器，再升级为节点）
- 节点管理：SSH 远程安装 k3s agent、列出节点、驱逐、删除
- CRD 依赖检测：启动时检测 Traefik/cert-manager CRD，缺失时前端提示一键安装
- Traefik IngressRoute 管理：创建/查看/删除路由规则
- cert-manager 集成：创建 Certificate 资源，自动签发/续期
- K8s API 客户端封装（client-go）

**删除：**
- 全部 gRPC Agent 代码（`cmd/agent/`、`internal/agent/`、`api/proto/`）
- SSH deployer（`internal/service/deployer/`）
- 心跳监控（`internal/service/server/`）
- mTLS 证书管理（`internal/crypto/tls.go` 中部署相关部分）
- deploy-probe / agent-info 相关 handler 代码
- Server/Site 表中 Agent 相关字段
- 前端部署/SSH 测试/状态同步按钮

**保留并适配：**
- 用户认证（JWT + bcrypt）
- 审计日志 + 操作日志
- 数据管理模块
- NGINX 模板渲染（适配 Traefik YAML 生成）
- 配置解析器（适配 IngressRoute 格式）
- Web UI 框架

## Capabilities

### 新增能力
- `k3s-platform`: Platform 容器化 + K3s 集群自举
- `k3s-node`: 节点全生命周期管理
- `k3s-traefik`: Traefik IngressRoute CRD 管理
- `k3s-cert`: cert-manager Certificate 自动管理

### 修改的能力
- `k8s-integration`: K8s client-go 集成层

## 影响范围

**新增文件：**
- `Dockerfile` + `k8s/platform-deployment.yaml`
- `internal/k8s/` — K8s client 封装（node/ingress/cert 操作）
- `cmd/init-k3s.sh` — K3s 初始化脚本

**删除文件/目录：**
- `cmd/agent/` — Agent 二进制
- `internal/agent/` — Agent server + pool + proto
- `internal/service/deployer/` — SSH deployer
- `internal/service/server/` — 心跳监控
- `api/proto/` — gRPC proto 定义
- `config/config.yaml` 中 agent/tls 段

**修改文件：**
- `internal/model/models.go` — Server 表格重构
- `internal/api/` — handler 适配 K8s API
- `web/src/views/` — 前端适配
