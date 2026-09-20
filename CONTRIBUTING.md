# Contributing to Cylism Manager

感谢贡献。提交前请先阅读 [README](README.md)、[公开文档](docs/index.md) 和 [AGENTS.md](AGENTS.md)。

## 开发环境

- Go 1.25+
- Node.js 24+，用于 VitePress 文档站
- 可选：Docker、Helm、可访问的 K3s 测试集群

安装前端依赖：

```bash
npm --prefix web ci
```

## 变更流程

涉及行为、接口、部署、数据模型或用户流程的改动必须遵循 OpenSpec：

1. 在 `openspec/changes/<change-name>/` 创建 proposal、design、spec 和 tasks。
2. 等待维护者审查 artifacts 后再实现。
3. 使用测试驱动方式完成 tasks，验证受影响的旧调用点与回归范围。
4. 在获得验证审查确认后归档 change，并同步基线 spec。

纯文案、拼写或不改变公开行为的小型修正也欢迎提交，但仍请保持文档链接和构建有效。

## 验证

提交 Pull Request 前运行与改动范围相符的检查：

```bash
go test ./...
go build ./...
npm --prefix web test
npm --prefix web run build
bash scripts/test-docs-site.sh
npm ci --prefix docs
npm run build --prefix docs
git diff --check
```

若环境无法运行某项检查，请在 Pull Request 中说明原因和已完成的替代验证。

## Pull Request

- 一个 Pull Request 聚焦一个可审查的目的。
- 说明用户可见行为、测试结果和可能的部署影响。
- 不提交 Secret、私钥、Token、真实基础设施地址或未脱敏截图。
- 安全问题不要通过公开 Issue 或 Pull Request 报告，请遵循 [SECURITY.md](SECURITY.md)。
