# AGENTS.md — Cylism Manager 开发流程规范

## 技术栈

- 后端: Go 1.22, Gin, GORM, SQLite
- 前端: Vue 3 + Vite
- 通信: gRPC (Agent), REST (Web UI)
- 认证: JWT (HMAC-SHA256), bcrypt

## OpenSpec + Superpowers 协作流程

本项目使用 OpenSpec 作为工程框架，Superpowers 作为方法论层，两者搭配使用。

### 新需求开发路径

```
用户提需求
  → brainstorming（探索 + 设计 + 审批）
    → openspec-propose（生成 proposal/design/specs/tasks）
      → openspec-apply-change + TDD + verification（实现 + 测试）
        → verification-before-completion（全量验证）
          → openspec-archive-change（归档 + 同步 specs）
```

### 各阶段详解

**阶段一：需求 → 设计（不写代码）**

| 步骤 | 工具 | 产出 | 检查点 |
|------|------|------|--------|
| 1 | brainstorming | 设计文档 | 用户审批设计 |
| 2 | openspec-propose | 4 artifacts | `openspec status` 显示 4/4 complete |

- brainstorming 负责探索项目上下文、逐一提问、方案对比、呈现设计
- openspec-propose 将设计转为结构化 artifact
- spec 中的 WHEN/THEN 场景作为后续测试用例来源
- **⛔ 审查门禁：** 设计通过后，openspec-propose 生成的 4 个 artifact（proposal/design/specs/tasks）必须停下来等待用户审查确认，用户明确同意后才能进入阶段二编码

**阶段二：实现（写代码 + 测试）**

| 步骤 | 工具 | 说明 |
|------|------|------|
| 3 | openspec-apply-change | 按 tasks.md 逐项实现 |
| 4 | test-driven-development | 每个 task 先写测试再实现（横切 + 影响范围分析） |
| 5 | verification-before-completion | 每个 task 完成后验证（横切） |
| 6 | systematic-debugging | 遇到 bug 时使用（按需） |
| - | **⛔ 审查门禁：** | 每个 task 完成后 `go test` 验证，结果呈报用户 |

### TDD 影响范围分析规则

实现时若修改了已有接口（函数签名变更、废弃旧方法、新增替代 API），必须：

1. `rg` 全量搜索旧 API/旧调用方式，列出所有调用点
2. 逐一检查是否遗漏替换（如 `pool.Connect` → `pool.ConnectWithTLS`）
3. 优先将旧方法标记为 deprecated 或移除，让编译器帮你发现遗漏

测试不仅覆盖新增代码，还必须验证：
- 旧调用点是否已全部更新
- 回归测试：全量 `go test ./...` 通过

**阶段三：收尾**

| 步骤 | 工具 | 说明 |
|------|------|------|
| 7 | verification-before-completion | 全量 `go test ./...` + `go build ./...` + `vite build` |
| - | **⛔ 审查门禁：** | 全量验证结果必须呈报用户，用户确认后才能 archive |
| 8 | openspec-archive-change | 归档 change，delta spec 同步到 `openspec/specs/` |

### 禁止的行为

- ❌ 跳过 brainstorming 直接写代码
- ❌ 跳过 TDD 直接实现（不写测试的代码不算完成）
- ❌ 未验证就声称完成（必须跑完编译+测试+构建）
- ❌ 完成后不 archive（必须同步 spec 基线）
- ❌ 在未验证时 archive

### 代码风格

- Go: 遵循标准 Go 风格，`gofmt` 格式化
- Vue: 使用 Composition API (`<script setup>`)
- 测试文件与源文件同目录，`*_test.go`
- 中文 UI 文案，英文代码标识符
- 使用 Design Tokens（CSS 变量）管理样式
