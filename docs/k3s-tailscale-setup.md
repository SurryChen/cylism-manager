# K3s + Tailscale 集成部署文档

> 在阿里云 ECS 上将 k3s 节点 IP 切换为 Tailscale IP，实现集群节点间通过 WireGuard 隧道通信。

## 架构概览

```
本机 (ubuntudev)               阿里云 ECS
┌─────────────────┐           ┌──────────────────────┐
│  Tailscale       │           │  K3s Server           │
│  100.76.79.27    │ ◄─────── │  100.121.176.105      │
│                 │  Tailnet  │  (Tailscale IP)        │
└─────────────────┘           │                        │
                               │  Node External IP:     │
                               │  100.76.126.100(旧)    │
                               │  100.121.176.105(新)   │
                               └──────────────────────┘
```

## 前置条件

- 阿里云 ECS 一台（已安装 k3s）
- [Tailscale 账号](https://login.tailscale.com)
- 可复用的 Tailscale Auth Key（[生成地址](https://login.tailscale.com/admin/settings/keys)）

## 操作步骤

### 1. 卸载已有 Tailscale（如果存在）

```bash
# 停服务
sudo systemctl stop tailscaled
sudo systemctl disable tailscaled

# 删除二进制
sudo rm -f /usr/local/bin/tailscale /usr/local/bin/tailscaled
sudo rm -f /usr/sbin/tailscale /usr/sbin/tailscaled

# 删除数据和配置
sudo rm -rf /var/lib/tailscale /run/tailscale /etc/default/tailscaled
sudo rm -f /etc/systemd/system/tailscaled.service

sudo systemctl daemon-reload
```

**检查 DNS：** 确认 `/etc/resolv.conf` 没有 Tailscale DNS（`100.100.100.100`）。如果有，恢复为阿里云 DNS：

```
nameserver 223.5.5.5
nameserver 223.6.6.6
```

### 2. 停 k3s + 清理 TLS 证书

```bash
sudo systemctl stop k3s

# 清理旧 TLS（绑定了旧节点 IP，改 IP 后必须重建）
sudo rm -rf /var/lib/rancher/k3s/server/tls
```

> 清理 TLS 不影响 etcd 数据和已有工作负载，k3s 重启时会自动重建。

### 3. 安装 Tailscale CLI

```bash
# 下载 tailscale 二进制（1.98.9）
curl -fsSL https://pkgs.tailscale.com/stable/tailscale_1.98.9_amd64.tgz -o /tmp/ts.tgz
tar xzf /tmp/ts.tgz -C /tmp/
sudo cp /tmp/tailscale_*/tailscale /usr/local/bin/
sudo cp /tmp/tailscale_*/tailscaled /usr/local/bin/
rm -rf /tmp/ts.tgz /tmp/tailscale_*
```

### 4. 启动 tailscaled 并认证

```bash
# 创建数据目录
sudo mkdir -p /var/lib/tailscale /run/tailscale

# 启动 tailscaled（后台）
sudo nohup /usr/local/bin/tailscaled \
  --state=/var/lib/tailscale/tailscaled.state \
  --socket=/run/tailscale/tailscaled.sock \
  --port=41641 > /dev/null 2>&1 &

sleep 3

# 使用 Auth Key 加入 tailnet（禁止接管 DNS）
sudo /usr/local/bin/tailscale up \
  --auth-key=tskey-auth-xxxxxxxxxxxx \
  --hostname=cylism-control-plane \
  --accept-dns=false

# 获取 Tailscale IP
sudo /usr/local/bin/tailscale ip -4
```

> **`--accept-dns=false` 必须加**，否则 Tailscale 会覆盖 `/etc/resolv.conf`，导致 k3s 无法解析 ACR 等域名导致镜像拉取失败。

### 5. 启动 k3s（带 VPN 集成）

```bash
# 下载安装脚本（如果不可用则直连 GitHub）
curl -sf4L -o /tmp/k3s-install.sh \
  https://raw.githubusercontent.com/k3s-io/k3s/master/install.sh
chmod +x /tmp/k3s-install.sh

# 执行安装（--vpn-auth 让 k3s 自动管理 tailscale）
INSTALL_K3S_SKIP_DOWNLOAD=true \
INSTALL_K3S_EXEC="server \
  --vpn-auth=name=tailscale,joinKey=tskey-auth-xxxxxxxxxxxx \
  --node-external-ip=<Tailscale IP>" \
  /tmp/k3s-install.sh
```

**参数说明：**

| 参数 | 作用 |
|------|------|
| `--vpn-auth` | k3s 内置 VPN 集成，自动安装/调用 tailscale |
| `--node-external-ip` | 节点对外注册的 IP（设为 Tailscale IP） |
| `INSTALL_K3S_SKIP_DOWNLOAD=true` | 跳过 k3s 二进制下载（已有旧版本） |

> **注意：** `get.k3s.io` 在某些环境（如阿里云）可能不可达，改用 GitHub RAW 直链下载安装脚本。`curl` 加 `-4` 强制 IPv4 避免 IPv6 网络不可用导致超时。

### 6. 验证

```bash
# 检查节点状态
sudo /usr/local/bin/k3s kubectl get nodes -o wide

# 预期输出：
# NAME   STATUS  ROLES  AGE  VERSION  INTERNAL-IP       EXTERNAL-IP
# node   Ready   cp     xx   v1.36.x  100.121.176.105   100.76.126.100

# 检查 tailscale 状态
sudo /usr/local/bin/tailscale status
```

### 7. 更新 kubeconfig

节点 IP 变更后，外部通过 `kubectl` 连接的 `server` 地址需要更新：

```bash
# 从 k3s 重新获取 kubeconfig
sudo /usr/local/bin/k3s kubectl config view --raw > ~/.kube/config-new

# 或手动修改现有 kubeconfig 中的 server 字段
# server: https://100.121.176.105:6443
```

## 常见问题

### Q: k3s 启动失败，报 `tailscale: executable file not found`

**原因：** `--vpn-auth` 需要 `tailscale` 二进制在 PATH 中，但只安装了 tailscale CLI 没启动 tailscaled。

**解决：** 先按第 4 步启动 tailscaled，再启动 k3s。

### Q: k3s 启动失败，报 `tailscaled doesn't appear to be running`

**原因：** tailscaled 后台进程已退出或没启动。

**解决：**
```bash
# 重新启动
sudo nohup /usr/local/bin/tailscaled \
  --state=/var/lib/tailscale/tailscaled.state \
  --socket=/run/tailscale/tailscaled.sock \
  --port=41641 > /dev/null 2>&1 &
sleep 3
sudo systemctl start k3s
```

### Q: k3s 无法拉取 ACR 镜像 (ImagePullBackOff)

**原因：** Tailscale 接管了 DNS 导致无法解析 ACR 域名。

**解决：** 确保 `tailscale up` 时加了 `--accept-dns=false`，并检查 `/etc/resolv.conf` 是阿里云 DNS。

### Q: 节点 IP 变了，cilism-manager 需要重新部署吗？

不需要。cilism-manager 通过 k8s API 通信（内部 ClusterIP），不受节点 IP 影响。但如果 cilism-manager 的配置文件中有写死旧 IP，需要更新。

### Q: SSH 到阿里云变慢（卡 20-30 秒）

**原因：** 之前 Tailscale 覆盖 DNS 时，PAM 模块 `pam_systemd` 依赖 DNS 解析主机名导致超时。

**解决：** 卸载 Tailscale 并恢复 DNS 后重启服务器。

## 回退方案

如需回退到原始配置：

```bash
# 1. 停 k3s
sudo systemctl stop k3s

# 2. 移除 VPN 配置
sudo rm -f /etc/systemd/system/k3s.service.d/*.conf
sudo systemctl daemon-reload

# 3. 清理 TLS（切回旧 IP 需要重建）
sudo rm -rf /var/lib/rancher/k3s/server/tls

# 4. 启动 k3s（不加 --vpn-auth）
sudo systemctl start k3s
```

## 参考

- [K3s VPN Integration Docs](https://docs.k3s.io/advanced#vpn-integration)
- [Tailscale Kubernetes Operator](https://tailscale.com/kb/1236/kubernetes-operator)
