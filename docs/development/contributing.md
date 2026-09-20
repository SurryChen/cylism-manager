# 开发与贡献

完整贡献政策位于仓库根目录的 `CONTRIBUTING.md`。本页保留文档站内的开发验证入口。

## 本地验证

```bash
go test ./...
go build ./...
npm --prefix web ci
npm --prefix web test
npm --prefix web run build
npm ci --prefix docs
npm run build --prefix docs
```

行为、接口、部署方式或用户流程发生变化时，先创建 OpenSpec change，完成 proposal、design、spec 和 tasks 审查后再实现。纯文案修正仍应运行文档严格构建。
