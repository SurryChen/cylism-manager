# Design: Traefik Upload Timeout Management

## Context

K3s 使用 `kube-system/traefik` HelmChart 与同名 HelmChartConfig 管理 Traefik。Cylism Manager 已能识别该控制源、写入 HelmChartConfig、保存期望 valuesContent，并在列表接口中检查实际状态。

OCI Registry 上传经 `websecure` entrypoint，HTTP 模式也可能经 `web` entrypoint。两者的 `transport.respondingTimeouts.readTimeout` 是静态入口参数，必须通过 Helm values 的 `additionalArguments` 在 Traefik Pod 重建时生效。

## Decisions

### 1. 使用 Traefik 专属结构化字段，不暴露任意 YAML 编辑

请求体增加可选的 `traefik_read_timeout`，格式为正 Go duration，允许 `1m`、`5m`、`30m`、`1h` 预设及符合规则的手工时长。未提交该字段表示不覆盖默认值；UI 明确展示默认 60 秒，不会在读取页面时静默创建配置。

选择 `30m` 作为 Registry 上传的推荐值。该值同时保留慢客户端的资源上限，避免无限读取时间带来的慢连接耗尽风险。

### 2. 平台完整拥有 Traefik HelmChartConfig 的 valuesContent

平台将已有受管字段和两个 timeout 参数一起渲染为 HelmChartConfig 的 `valuesContent`：

```yaml
maxUnavailable: 0
maxSurge: 1
additionalArguments:
  - "--entryPoints.web.transport.respondingTimeouts.readTimeout=30m"
  - "--entryPoints.websecure.transport.respondingTimeouts.readTimeout=30m"
```

解析时保留现有 `replicas`、`maxUnavailable`、`maxSurge` 和 timeout 状态。未知 Helm values 不能由此界面保留，页面会提示该配置由平台管理；这与当前 System Components 的受控 ownership 语义一致，避免平台与手工 YAML 同时修改同一个 HelmChartConfig。

### 3. 保存后验证实际启动参数

HelmChartConfig 写入成功不等于 Traefik 已重建。平台保存后读取 Traefik Deployment 的容器参数，确认 `web` 与 `websecure` 都含目标 `readTimeout`。在 Helm 控制器尚未调谐时，将状态标为“等待生效”，保留期望值并允许刷新；控制器报错或参数不一致时显示可操作错误。

### 4. 恢复默认沿用既有系统组件语义

“恢复默认”删除 `kube-system/traefik` HelmChartConfig 和平台保存的 SystemComponentConfig，K3s helm-controller 回归 bundled chart 的默认 60 秒行为。操作需要现有确认流程，不额外删除 Deployment 或 Service。

## API

现有 API 扩展而不新建资源：

```text
GET /api/system-components
  -> Traefik item adds:
     traefik: {
       read_timeout: "30m" | "",
       effective_read_timeout: "30m" | "60s",
       read_timeout_effective: true | false
     }

PUT /api/system-components/traefik
  body: {
    replicas, maxUnavailable, maxSurge,
    traefik_read_timeout: "30m" | ""
  }
```

The backend rejects a timeout for non-Traefik charts, malformed durations, zero/negative values, and values outside the configured safety range of 1 minute through 1 hour.

## UI

Only when editing Traefik in Helm-managed mode, the modal includes an “入口请求读取超时” preset select with a custom option. Presets are default (60 seconds), 5 minutes, 30 minutes (recommended), and 1 hour; the custom input accepts a validated duration such as `15m`. Saving triggers a Traefik rolling update and new settings become effective only after its Deployment reports ready.

The component table configuration column shows the selected timeout and its actual effect state. It does not add explanatory microcopy to unrelated components.

## Risks And Mitigations

- A higher timeout permits slow request bodies for longer: enforce 1 minute to 1 hour and do not offer infinite timeout.
- A malformed Helm value can prevent Traefik rollout: generate static arguments server-side, validate request duration before apply, and retain the prior persisted configuration if Kubernetes write fails.
- Helm reconciliation is asynchronous: distinguish “saved / waiting for effect” from “effective”; provide refresh and preserve the reported mismatch.
- Existing hand-authored values may be overwritten: indicate platform ownership in the Traefik configuration modal; this module remains the single writer for its HelmChartConfig.

## Alternatives Considered

- Ingress annotation or Middleware: these do not change static entrypoint request-body read timeout, so they cannot reliably fix blob uploads.
- Set timeout only on the Registry Ingress: Traefik applies this timeout before routing, therefore it must be configured per entrypoint.
- Allow raw arbitrary Helm YAML: powerful but makes validation, compatibility, ownership, and safe rollback impossible for a platform-managed system component.
