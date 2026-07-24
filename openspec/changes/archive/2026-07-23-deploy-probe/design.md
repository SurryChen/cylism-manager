## 背景

当前部署是同步阻塞的"一键操作"：按钮 → POST /deploy → 全流程执行 → 返回结果。没有前置探测步骤，用户无法知道远端状态，也无法做出是否覆盖的决策。

## 目标 / 非目标

**目标：**
- 部署前通过 SSH 探测远端 Agent 状态（进程、systemd、二进制存在性、版本）
- 探测结果返回给前端，弹出确认对话框
- 支持 force 覆盖模式（停止旧服务 → 覆盖二进制 → 重启）
- Agent 支持 `--version` 输出版本号
- 探测失败直接阻断，返回失败原因

**非目标：**
- Agent 版本自动比对与升级策略（一期仅做覆盖）
- 部署回滚
- 多版本 Agent 并存

## 技术决策

### 1. 探测接口独立于部署接口

新增 `POST /api/servers/:id/deploy/probe` 独立端点，不耦合到部署流程中。前端可独立调用探测，用户确认后再调部署。

- 备选：将探测作为 deploy 的 query param 一部分 — 逻辑耦合，难以扩展

### 2. ProbeAgent 探测命令

```
ps aux | grep cylism-agent          → 检测进程
systemctl is-active cylism-agent    → 检测 systemd 状态
test -f /opt/cylism-manager/agent   → 检测二进制
/opt/cylism-manager/agent --version → 获取版本
```

任一命令失败不影响其他探测项的采集，但若 SSH 连接本身失败则直接返回失败。

### 3. force 部署流程

```
1. systemctl stop cylism-agent (若 service 存在)
2. pkill -f cylism-agent (兜底杀进程)
3. 上传新二进制覆盖
4. systemctl start cylism-agent (若 service 存在)
   or nohup 启动 (若无 systemd)
5. 建立 gRPC 连接
```

### 4. Agent --version

在 `cmd/agent/main.go` 入口处注册 `--version` flag，输出格式：
```
cylism-agent version 1.0.0 (linux/amd64)
```

### 5. 前端交互

```
点击"部署" 
  → 调 probe 接口（按钮 loading 态）
  → 弹窗显示探测结果：
      无 Agent："未检测到 Agent，将全新部署。是否继续？"
      有 Agent："检测到 Agent v1.0.0 正在运行。覆盖部署？"
  → 用户确认 → 调 deploy?force=true → 刷新列表
  → 用户取消 → 关闭弹窗
```

## 风险与权衡

- **[探测命令兼容性]** 某些精简 Linux 无 systemctl → 探测时仅标记 systemd 不可用，不阻断
- **[探测耗时]** 多次 SSH 命令串行执行（~2-3s） → 可接受，前端有 loading 态
