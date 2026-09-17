# 产品截图清单

截图用于解释真实产品界面，不应用作装饰性素材。第一期建议按以下顺序补充：

| 文件名 | 页面 | 用途 |
| --- | --- | --- |
| `overview-dashboard.png` | 概览 | 文档首页与 README 产品入口。 |
| `server-onboarding.png` | 服务器纳管或预检 | 快速开始。 |
| `cluster-management.png` | 集群节点或系统组件 | 集群管理说明。 |
| `registry-management.png` | 制品库或镜像目录 | 镜像仓库说明。 |
| `resource-management.png` | Kubernetes 资源列表 | 资源管理说明。 |
| `certificate-management.png` | 证书管理 | 证书运维说明。 |

## 拍摄规范

- 使用 `1440 x 900` 桌面视口、默认薄荷配色和当前稳定 UI。
- 文件使用小写英文、短横线命名，采用 PNG；原始未裁剪文件不要提交。
- 一张截图只表达一个任务或状态，保留页面标题、关键操作和必要上下文。
- 图片加入公开页面前，本地执行 `python -m mkdocs build --strict` 确认相对路径有效。

## 脱敏要求

截图不得包含 Token、私钥、密码、完整 kubeconfig、真实公网或内网地址、主机标识、用户名、真实域名、镜像仓库端点、客户名称，或未脱敏错误日志。使用演示数据时也不要展示看似可用的凭据。
