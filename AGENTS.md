# AGENTS.md — Cylism Manager 项目约定

## 技术栈

- 后端: Go 1.22, Gin, GORM, SQLite
- 前端: Vue 3 + Vite
- 通信: gRPC (Agent), REST (Web UI)
- 认证: JWT (HMAC-SHA256), bcrypt

## 代码风格

- Go: 遵循标准 Go 风格，`gofmt` 格式化
- Vue: 使用 Composition API (`<script setup>`)
- 测试文件与源文件同目录，`*_test.go`
- 中文 UI 文案，英文代码标识符
- 使用 Design Tokens（CSS 变量）管理样式
