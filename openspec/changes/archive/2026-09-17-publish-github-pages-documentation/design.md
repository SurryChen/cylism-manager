## Context

项目已有 `docs/k3s-tailscale-deployment.md`、设计文档和发布 workflow，但没有静态文档站生成器、Pages workflow、开源协作文件或面向新用户的任务型手册。产品具有高权限边界：部署实例会访问 Kubernetes RBAC、宿主机 Tailscale socket，并可通过 SSH 私钥纳管主机，因此安装和安全文档不能只描述 Happy Path。

现有部署方式只有脚本和 Helm Chart；不应在文档中承诺 Docker Compose 等未提供且未验证的路径。

## Decisions

### 1. 采用 MkDocs Material 和现有 `docs/` 目录

根目录的 `mkdocs.yml` 定义中文站点标题、导航、搜索和静态资源。MkDocs 直接以 `docs/` 为 source directory，避免复制现有 Markdown 或为文档站维护第二套内容。Material 主题提供响应式布局、搜索、代码复制和清晰的信息层级，适合以运维操作为主的项目。

Python 文档依赖使用单独的 requirements 文件固定主版本范围，GitHub Actions 和本地预览使用同一依赖来源。项目的 Go 与 Node 依赖不用于构建文档站。

### 2. Pages 使用独立、最小权限的部署 workflow

新增 `.github/workflows/docs-pages.yml`，在 `main` 或 `dev` 上涉及文档、站点配置或 workflow 的变更时运行，也支持手工触发。两条分支均执行严格构建，只有 `main` 上传 Pages artifact 并部署。workflow 使用 GitHub 官方的 Pages configure、artifact upload 和 deploy actions；构建 job 仅有 `contents: read`，部署 job 仅授予 `pages: write` 与 `id-token: write`。

站点的最终 URL 由 `configure-pages` 输出和 `site_url` 环境变量传递给构建过程，不写死个人 GitHub 账号、组织名或自定义域名。仓库管理员仍需在 GitHub Settings -> Pages 将 Source 设置为 GitHub Actions，这是一次仓库设置动作，不由代码自动完成。

### 3. 首期文档按用户任务组织

导航分为：开始、安装、配置、使用、运维与排障、安全、开发与贡献。安装只覆盖实际支持的脚本与 Helm 路径；每个安装页面都应给出前置条件、可复制命令、成功验证、升级/回滚入口和常见故障。

README 保持短小：一句定位、适用与不适用场景、两条安装链接、核心能力、安全提示、开发入口和文档站链接。详细操作移入 Pages 文档，避免 README 变成会漂移的部署手册。

### 4. 截图遵循目录、命名和脱敏约束

创建 `docs/assets/screenshots/README.md`，列出首批建议截图、尺寸、命名和隐私要求。文档页面以明确的占位提示标记尚未提供的图片，不能引用不存在的文件或制造看似真实的截图。

截图采用 1440 x 900 桌面视口，默认薄荷配色；不得包含 Token、私钥、真实公网 IP、内网地址、域名、仓库地址、用户名、主机名或错误日志中可识别的数据。截图提交后引用本地相对路径，保证 Pages、GitHub README 和本地预览一致。

### 5. 开源治理文件与许可证独立审查

新增 `CONTRIBUTING.md` 和 `SECURITY.md`，分别描述开发验证、OpenSpec 流程、Pull Request 预期和私下漏洞报告方式。许可证会新增为标准文本，但 Apache-2.0 与 AGPL-3.0 对使用者权利有实质差异：前者最大化采用，后者要求网络服务修改开源。因此在 artifacts 审查后、实现前必须由维护者选择其一。

## Alternatives Considered

1. **只扩展 README**：不能提供稳定导航、全文搜索、细粒度链接和可维护的安装/排障内容。
2. **使用 VitePress**：可复用 Node 工具链，但本项目文档不需要 Vue 组件或定制站点，MkDocs Material 的运维文档能力和更低配置成本更合适。
3. **将 Pages 部署到 `gh-pages` 分支**：会引入生成文件分支和历史噪声；Actions artifact deployment 更清晰，也不需要 workflow 写仓库内容。
4. **首期加入版本化文档**：发布版本数量尚少，版本矩阵会增加维护成本；第一期只发布 `main` 对应的最新稳定文档。

## Risks and Mitigations

- **公开文档泄漏内部信息**：将 ACR、测试部署 Webhook、个人镜像地址等移出面向用户的页面，使用通用占位符；提交前以关键词检查密钥和内部域名。
- **文档与脚本漂移**：安装命令直接围绕 `scripts/deploy-platform.sh --help`、Helm Chart 和现有清单编写；CI 构建文档保证链接与 Markdown 语法有效。
- **用户误解权限边界**：在 README、安装前置条件和安全页重复说明 Tailscale socket、SSH 私钥、Kubernetes RBAC 与 SQLite hostPath 的风险和保护要求。
- **截图暴露真实数据**：先提交截图规范和目录；只有脱敏、审查通过的图片才进入站点。

## Verification Strategy

- 先添加文件级测试或脚本断言，验证导航、关键页面、Pages workflow 权限、触发条件、文档链接和截图规范。
- 运行 MkDocs strict build，确认全部页面、内部链接和静态资源可构建。
- 在本地预览检查桌面和移动导航、代码块、图片占位与中文排版。
- 运行 `go test ./...`、`go build ./...`、`npm --prefix web test`、`npm --prefix web run build`、`openspec validate publish-github-pages-documentation --strict` 和 `git diff --check`。
