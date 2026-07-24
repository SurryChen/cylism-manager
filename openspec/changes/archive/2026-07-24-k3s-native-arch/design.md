## 背景

Cylism Manager 现有约 7000+ 行 Go 代码，其中 ~4000 行与自研 Agent/SSH/mTLS 相关。K3s 生态可直接替代这些自研组件。切换到 K3s 原生架构可将代码量减少 ~50%，同时获得生产级的节点管理、证书自动续期、流量管理能力。

## 目标 / 非目标

**目标：**
- Platform 自身容器化，作为 K3s Deployment 运行
- K3s 集群自举：初始化 → SSH 扩展节点 → 显示/驱逐/删除节点
- Traefik IngressRoute 管理（CRUD on CRD）
- cert-manager 集成（自动签发/续期 Let's Encrypt 证书）
- 删除所有 Agent/SSH/mTLS 相关代码

**非目标：**
- 多集群管理（一期单集群）
- K3s 版本升级管理
- Helm Chart 商店
- Pod 日志/终端

## 技术决策

### 1. K8s 客户端

使用 `client-go` + 集群内 ServiceAccount 认证（Pod 自动注入 token）。Platform 无需外部 kubeconfig，通过 `rest.InClusterConfig()` 获取。

### 2. 节点管理流程

```
添加节点：
  1. SSH 到目标 IP（复用现有 SSH 客户端代码，去掉 deployer 层）
  2. 执行: curl -sfL https://get.k3s.io | K3S_URL=https://<control-plane>:6443 K3S_TOKEN=<token> sh -
  3. 等待节点 Ready → 显示在节点列表

驱逐节点: kubectl drain <node> --ignore-daemonsets --delete-emptydir-data
删除节点: kubectl delete node <node> + SSH pkill k3s-agent（可选）
```

SSH 连接复用现有 `golang.org/x/crypto/ssh`，但干掉 `internal/service/deployer/` 目录，精简为 `internal/ssh/client.go`。

### 3. Server 表扩展（而非替换）

Server 表保留现有字段（name/host/SSH 凭据等），删除 Agent 相关字段，新增集群相关字段：

```
Server{ 
  // 保留字段
  Name, Host, SSHHost, SSHPort, SSHUser, SSHAuthType,
  SSHPassword, SSHKey, SSHPassphrase, Status,
  
  // 删除 Agent 相关
  ~~Port, AgentVersion, AgentDeployPath, AgentDeployedAt~~
  
  // 新增集群字段
  ClusterRole   string  // 空=未加入, "control-plane", "worker"
  K8sNodeName   string  // K8s Node 资源名称
}
```

服务器与集群节点是两阶段：
1. 添加服务器 → 存 SSH 凭据 → 可做连通性测试
2. 加入集群 → SSH 安装 k3s agent → 更新 cluster_role + k8s_node_name

### 4. Site 表 → IngressRoute CRD

```
旧: Site{ ServerID, Domain, Port, SSLEnabled, RootPath, ... }

新: 不再用 Site 表。直接操作 Traefik IngressRoute CRD：
    apiVersion: traefik.io/v1alpha1
    kind: IngressRoute
    spec:
      routes:
      - match: Host(`example.com`)
        services:
        - name: my-service
          port: 80
```

前端"站点"页面展示 IngressRoute 列表，增删改直接调 K8s API。

### 5. Cert 表 → Certificate CRD

```
旧: Cert{ SiteID, Domains, Provider, CertPath, KeyPath, ... }

新: 不再用 Cert 表。通过 cert-manager Certificate CRD：
    apiVersion: cert-manager.io/v1
    kind: Certificate
    spec:
      secretName: example-tls
      dnsNames: [example.com]
      issuerRef: { name: letsencrypt-prod, kind: ClusterIssuer }
```

前端"证书"信息从 Certificate CRD status 读取。

### 6. 保留的 SQLite 表

- `users` — 用户认证
- `audit_logs` — 审计日志
- `operation_logs` — 操作日志
- `nodes` — 节点缓存信息（可选，减少 K8s API 频繁查询）

### 7. 服务器 → 节点分阶段管理

服务器注册和集群节点是两个阶段，不绑定：

```
阶段 1: 添加服务器（保留现有 Server 模型）
  输入 IP + SSH 凭据 → 存入 Server 表 → 可做 SSH 连通性测试
  
阶段 2: 加入集群（新增操作）
  对已注册服务器触发"加入集群" → SSH 安装 k3s agent → 
  Server.cluster_role = "worker" / "control-plane" → K8s Node API 可见
```

Server 表新增字段：
- `cluster_role`: 空=未加入, "control-plane", "worker"
- `k8s_node_name`: K8s Node 资源名称

前端节点页面两个 tab：服务器列表 + 集群节点列表。

### 8. CRD 依赖检测与自动安装

Platform 启动时检测必要的 CRD 是否已安装：

```
检测项：
  - Traefik: kubectl get crd ingressroutes.traefik.io
  - cert-manager: kubectl get crd certificates.cert-manager.io

若缺失：
  - 前端顶部显示 banner 警告
  - 提供一键安装入口：
      Traefik: helm install traefik traefik/traefik
      cert-manager: kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

### 9. SQLite 持久化方案

使用 K3s 内置 Local Path Provisioner + PVC：

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
  storageClassName: local-path
```

Deployment 挂载 PVC 到 `/data`，SQLite 文件路径 `data/cylism.db`。Pod 重建后只要调度到同一节点，数据不丢失。

初始化脚本确保节点上 `/var/lib/cylism-manager/` 目录存在且挂载正确。

## 风险与权衡

- **[K8s API 依赖]** Platform 不可用时 K8s 集群仍正常运行，只是管理面板不可访问
- **[cert-manager 前置依赖]** 需在 K3s 集群中先部署 cert-manager → Platform 初始化脚本自动完成
- **[Traefik CRD 依赖]** K3s 默认带 Traefik，无需额外安装
