<template>
  <div>
    <SectionTabsHeader class="page-header detail-tabs-header" :title="application?.name || '应用详情'" :tabs="detailTabs" :active-tab="activeTab" @select="activeTab = $event">
      <template #title><div class="detail-heading"><div class="detail-context"><router-link class="back-link" to="/applications"><ArrowLeft :size="16" />返回工作台</router-link></div><h1 v-if="application || readError" class="page-title">{{ application?.name || '应用详情' }}</h1><div v-else class="detail-title-placeholder" aria-hidden="true"></div></div></template>
      <template #actions><button v-if="application" class="btn btn-sm" aria-label="应用设置" @click="openApplicationSettings"><Settings :size="14" />应用设置</button></template>
    </SectionTabsHeader>
    <div v-if="readError" data-testid="application-detail-read-error" class="k8s-banner k8s-banner-warn section-gap">{{ readError }}</div>
    <ErrorNoticeModal :open="Boolean(mutationError)" :message="mutationError" teleport @close="mutationError = ''" />

    <ApplicationRuntime v-if="application && activeTab === 'runtime' && application.environment?.cluster_id" :application-id="applicationID" :cluster-id="application.environment.cluster_id" :releases="releases">
      <template #actions><button class="btn btn-sm" :disabled="restarting || !releases.length" @click="openRestartConfirmation"><RefreshCw :size="14" :class="{ 'is-spinning': restarting }" />{{ restarting ? '重启中...' : '重启应用' }}</button></template>
    </ApplicationRuntime>
    <EmptyState v-else-if="application && activeTab === 'runtime'" message="当前环境尚未绑定集群" />

    <SurfaceCard v-show="activeTab === 'config' && application" as="div" padding="none" class="detail-tab-card detail-config-card">

    <section v-if="application" class="detail-section template-section">
      <div class="section-heading"><div><h2>上线模板</h2><p>保存模板后，发布或滚动重启应用以生效。</p></div><button class="btn btn-sm btn-primary" @click="openTemplateEditor()">新建模板</button></div>
      <div v-if="templates.length" class="table-wrap detail-table-wrap"><table class="data-table"><thead><tr><th>模板</th><th>镜像路径</th><th>默认版本号</th><th>运行配置</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="template in templates" :key="template.id"><td class="cell-primary">{{ template.name }}<small v-if="template.description">{{ template.description }}</small></td><td class="template-image-cell"><OverflowTooltip :text="template.spec.image" /></td><td>{{ template.default_version || '-' }}</td><td class="template-runtime-cell"><div>{{ template.spec.replicas }} 副本 · {{ template.spec.container_port }} 容器端口</div><div class="cell-secondary">{{ templateServiceSummary(template.spec.service) }}<template v-if="template.spec.dns_ndots != null"> · ndots={{ template.spec.dns_ndots }}</template></div></td><td><span class="badge" :class="template.enabled ? 'badge-online' : 'badge-offline'">{{ template.enabled ? template.is_default ? '默认' : '启用' : '停用' }}</span></td><td><div class="btn-group"><button v-if="template.enabled && !template.is_default" class="btn btn-sm" @click="setDefaultTemplate(template)">设为默认</button><button class="btn btn-sm" @click="openTemplateEditor(template)">编辑</button><button class="btn btn-sm btn-danger" @click="deleteTemplate(template)">删除</button></div></td></tr></tbody></table></div>
      <EmptyState v-else message="尚未配置上线模板" />
    </section>

    <section v-if="application" class="detail-section deploy-webhook-section">
      <div class="section-heading"><div><h2>自动发布回调</h2><p>使用默认上线模板和回调提交的镜像版本创建发布。</p></div><div class="detail-heading-actions"><span class="badge" :class="deployWebhookStatus?.configured ? 'badge-online' : 'badge-offline'">{{ deployWebhookStatus === null ? '读取中' : deployWebhookStatus.unavailable ? '读取失败' : deployWebhookStatus.configured ? '已启用' : '未启用' }}</span><button v-if="deployWebhookStatus?.unavailable" class="icon-button" type="button" title="重试读取" aria-label="重试读取" @click="loadDeployWebhookStatus"><RefreshCw :size="15" /></button><button v-else class="btn btn-sm" type="button" :disabled="webhookBusy || deployWebhookStatus === null" @click="showWebhookSecretModal = true">{{ deployWebhookStatus?.configured ? '轮换密钥' : '生成密钥' }}</button><button v-if="deployWebhookStatus?.configured" class="btn btn-sm btn-danger" type="button" :disabled="webhookBusy" @click="showWebhookDisableModal = true">关闭</button></div></div>
      <details class="deploy-webhook-details"><summary>回调地址与 CI 接入</summary>
      <div class="deploy-webhook-endpoint"><input class="form-input" aria-label="自动发布回调地址" readonly :value="deployWebhookURL" /><button class="icon-button" type="button" title="复制回调地址" aria-label="复制回调地址" @click="copyWebhookValue(deployWebhookURL)"><Copy :size="15" /></button></div>
      <div class="deploy-webhook-example"><pre><code>POST {{ deployWebhookURL }}
X-Cylism-Timestamp: Unix 秒
X-Cylism-Nonce: 至少 16 位随机值
X-Cylism-Signature: HMAC-SHA256(应用密钥, timestamp + "." + nonce + "." + 原始请求体) 的十六进制值

{"version":"1.2.3"}</code></pre></div></details>
    </section>

    <details v-if="application && templateDependencies.length" class="detail-section config-dependencies" :open="templateDependencies.some(item => !item.ready)"><summary>运行依赖 · {{ templateDependencies.length }} 项<span v-if="templateDependencies.some(item => !item.ready)" class="badge badge-danger">存在缺失</span></summary><div class="section-heading"><p>模板引用的文件资源会在发布前再次验证。</p><button class="btn btn-sm" @click="openResourceManagement">管理资源</button></div><div class="dependency-list"><div v-for="dependency in templateDependencies" :key="dependency.id" class="dependency-row"><div><strong>{{ dependency.template }}</strong><small>{{ dependency.type }} · {{ dependency.name }} · {{ dependency.key }} → {{ dependency.path }}</small></div><span class="badge" :class="dependency.ready ? 'badge-online' : 'badge-danger'">{{ dependency.ready ? '可用' : '缺失' }}</span></div></div></details>
    </SurfaceCard>

    <SurfaceCard v-show="activeTab === 'access' && application" as="div" padding="none" class="detail-tab-card detail-access-card">

    <section v-if="application" class="detail-section">
      <div class="section-heading"><div><h2>对外域名</h2><p>域名可以作为 HTTP Ingress 入口，也可以只保存为其他协议的访问标识。变更时仅同步实际启用的 Ingress。</p></div><div class="detail-heading-actions"><span class="detail-count">{{ endpoints.length }} 个域名</span><button class="btn btn-sm" @click="openCertificateManagement">导入/申请域名</button><button class="btn btn-sm btn-primary" @click="openEndpointEditor()">绑定域名</button></div></div>
      <div v-if="endpoints.length" class="endpoint-list"><div v-for="endpoint in endpoints" :key="endpoint.id" class="endpoint-row"><div><a v-if="endpoint.ingress_enabled !== false" class="endpoint-url" href="#" @click.prevent="openApplicationEndpoint(endpoint)">{{ endpoint.domain }}</a><span v-else class="endpoint-url endpoint-metadata">{{ endpoint.domain }}</span><small>{{ endpoint.path || '/' }} · Service {{ endpoint.service_port }} · {{ endpoint.protocol || 'TCP' }} · {{ endpoint.tls_enabled ? 'HTTPS' : 'HTTP' }} · {{ endpoint.ingress_enabled === false ? '仅域名标识' : 'Ingress' }} · <span class="endpoint-mode">{{ endpointAccessLabel(endpoint) }}</span></small></div><div class="btn-group"><button v-if="isProtectedConsole(endpoint) && endpoint.ingress_enabled !== false" class="btn btn-sm" @click="openLocalDebugEditor(endpoint)">本地联调</button><button class="btn btn-sm" @click="openEndpointEditor(endpoint)"><Pencil :size="14" /> 编辑</button><button class="btn btn-sm btn-danger" @click="removeEndpoint(endpoint)">解绑</button></div></div></div>
      <div v-else class="empty-inline">当前应用仅限集群内访问</div>
    </section>

    </SurfaceCard>

    <SurfaceCard v-show="activeTab === 'releases' && application" as="div" padding="none" class="detail-tab-card detail-releases-card">
      <div class="detail-card-toolbar"><span>发布记录</span><span class="detail-count">共 {{ releases.length }} 条</span></div>
      <div v-if="releases.length" class="table-wrap detail-table-wrap"><table class="data-table"><thead><tr><th>版本</th><th>镜像</th><th>模板</th><th>状态</th><th>时间</th></tr></thead><tbody><tr v-for="release in releases" :key="release.id" class="clickable" @click="openRelease(release)"><td class="cell-primary">{{ release.version || `Release #${release.sequence}` }}</td><td>{{ release.image }}</td><td>{{ templateName(release.template_id) || '-' }}</td><td><span class="badge" :class="releaseBadge(release.status)">{{ release.status }}</span></td><td>{{ formatTime(release.created_at) }}</td></tr></tbody></table></div>
      <EmptyState v-else message="暂无发布记录" />
    </SurfaceCard>

    <BaseModal :open="showApplicationSettings && !showWorkloadEditor" title="应用设置" dialog-class="application-settings-modal capability-section" :close-on-overlay="!savingCapabilities" :close-on-escape="!savingCapabilities" @close="closeApplicationSettings">
      <section class="application-setting-group">
        <h3>工作负载类型</h3>
        <p class="form-hint">Deployment 适用于无状态服务，StatefulSet 适用于持久服务。</p>
        <div class="settings-workload-row"><SelectMenu v-model="workloadKind" class="form-select" aria-label="工作负载类型" :disabled="savingCapabilities"><option value="deployment">Deployment</option><option value="statefulset">StatefulSet</option></SelectMenu><button class="btn btn-sm" aria-label="修改工作负载类型" :disabled="savingCapabilities || workloadKind === (application?.workload_kind || 'deployment')" @click="showWorkloadEditor = true">迁移</button></div>
      </section>
      <section class="application-setting-group">
      <div class="section-heading"><div><h3>能力标签</h3><p>用于让受权管理台发现应用，不影响部署资源。</p></div><button class="btn btn-sm" :disabled="savingCapabilities" @click="addCapability">添加标签</button></div>
      <div v-if="capabilityItems.length" class="capability-list"><div v-for="(item, index) in capabilityItems" :key="index" class="capability-row"><input v-model="item.value" class="form-input" aria-label="能力标签" maxlength="64" placeholder="例如 hysteria2" :disabled="savingCapabilities" /><button class="icon-button" type="button" title="移除能力标签" :disabled="savingCapabilities" @click="removeCapability(index)"><Trash2 :size="16" /></button></div></div>
      <EmptyState v-else class="settings-empty" message="尚未设置能力标签" />
      </section>
      <template #actions><button class="btn btn-sm" :disabled="savingCapabilities" @click="closeApplicationSettings">取消</button><button class="btn btn-sm btn-primary" :disabled="savingCapabilities" @click="saveCapabilities">{{ savingCapabilities ? '保存中...' : '保存能力标签' }}</button></template>
    </BaseModal>
    <BaseModal :open="showWorkloadEditor" title="确认迁移工作负载" size="small" dialog-class="workload-editor" :close-on-overlay="!saving" :close-on-escape="!saving" @close="closeWorkloadEditor">
      <p class="form-hint">Deployment 适用于无状态服务，StatefulSet 适用于持久服务。迁移会先停止当前 Pod，再创建新的控制器并复用 PVC，服务会短暂中断。</p>
      <p class="workload-migration-summary">{{ application?.workload_kind === 'statefulset' ? 'StatefulSet' : 'Deployment' }} → {{ workloadKind === 'statefulset' ? 'StatefulSet' : 'Deployment' }}</p>
      <template #actions><button class="btn btn-sm" :disabled="saving" @click="closeWorkloadEditor">取消</button><button class="btn btn-sm btn-primary" :disabled="saving || workloadKind === (application?.workload_kind || 'deployment')" @click="updateWorkloadKind">{{ saving ? '迁移中...' : '确认迁移' }}</button></template>
    </BaseModal>

    <BaseModal :open="showWebhookSecretModal" :title="generatedWebhookSecret ? '自动发布密钥' : '生成自动发布密钥'" size="small" teleport @close="closeWebhookSecretModal">
      <template v-if="generatedWebhookSecret"><p>密钥仅显示本次。</p><code class="deploy-webhook-secret">{{ generatedWebhookSecret }}</code></template>
      <p v-else>生成后，原有回调密钥将立即失效。</p>
      <template #actions><button v-if="generatedWebhookSecret" class="btn btn-sm" type="button" @click="copyWebhookValue(generatedWebhookSecret)">复制密钥</button><button class="btn btn-sm btn-primary" type="button" :disabled="webhookBusy" @click="generatedWebhookSecret ? closeWebhookSecretModal() : generateWebhookSecret()">{{ generatedWebhookSecret ? '完成' : webhookBusy ? '生成中...' : '生成密钥' }}</button></template>
    </BaseModal>
    <BaseModal :open="showWebhookDisableModal" title="关闭自动发布回调" size="small" teleport @close="showWebhookDisableModal = false">
      <p>关闭后，现有密钥将立即失效。</p>
      <template #actions><button class="btn btn-sm" type="button" @click="showWebhookDisableModal = false">取消</button><button class="btn btn-sm btn-danger" type="button" :disabled="webhookBusy" @click="disableWebhook">{{ webhookBusy ? '关闭中...' : '确认关闭' }}</button></template>
    </BaseModal>

    <Teleport to="body"><div v-if="showTemplateEditor" class="overlay" @click.self="!saving && closeTemplateEditor()"><div class="modal editor-modal" role="dialog" aria-modal="true" aria-label="上线模板编辑器"><div class="editor-header"><div><h2 class="modal-title">{{ editingTemplate ? '编辑上线模板' : '新建上线模板' }}</h2><p>{{ templateEditorSummary }}</p></div><label class="check-row"><input v-model="templateForm.enabled" type="checkbox" /> 启用</label></div><div class="template-editor-layout"><nav class="editor-nav" aria-label="模板配置分区"><button v-for="section in templateEditorSections" :key="section.id" type="button" :class="{ active: activeTemplateSection === section.id }" :aria-pressed="activeTemplateSection === section.id" :aria-controls="`template-${section.id}`" @click="selectTemplateSection(section.id)"><span>{{ section.label }}</span><small>{{ section.summary }}</small></button></nav><form id="template-editor-form" ref="templateEditorFormElement" class="template-editor-form" @submit.prevent="saveTemplate" @invalid.capture="revealInvalidTemplateSection">
      <section id="template-basic" v-show="activeTemplateSection === 'basic'" class="editor-section"><div class="editor-section-heading"><div><h3>基础信息</h3><p>模板标识与镜像来源。</p></div></div><div class="form-row"><div class="form-group"><label class="form-label">模板名称</label><input v-model.trim="templateForm.name" class="form-input" required placeholder="标准生产配置" /></div><div class="form-group"><label class="form-label">说明</label><input v-model.trim="templateForm.description" class="form-input" placeholder="2 副本、HTTP 就绪检查" /></div></div>
      <div class="form-group"><label class="form-label">镜像仓库</label><SelectMenu v-model.number="templateForm.registry_id" class="form-select"><option :value="0">自定义完整镜像地址</option><option v-for="registry in registries" :key="registry.id" :value="registry.id">{{ registry.name }} · {{ registry.endpoint }}</option></SelectMenu></div>
      <div class="form-row"><div class="form-group"><label class="form-label">镜像路径</label><input v-model.trim="templateForm.image" class="form-input" required :placeholder="templateForm.registry_id ? 'commerce/order-api' : 'registry.example.com/order-api'" /></div><div class="form-group"><label class="form-label">默认版本号</label><input v-model.trim="templateForm.default_version" class="form-input" pattern="[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}" placeholder="1.4.2" /><p class="form-hint">发布时留空则使用此版本；已有模板可以暂不设置。</p></div></div></section>
      <section id="template-deployment" v-show="activeTemplateSection === 'deployment'" class="editor-section"><div class="editor-section-heading"><div><h3>服务部署</h3><p>容器启动方式、副本与节点调度。</p></div></div><details class="editor-disclosure" :open="templateForm.command_items.length > 0 || templateForm.argument_items.length > 0"><summary>自定义启动方式 <small>{{ templateForm.command_items.length || templateForm.argument_items.length ? '已配置' : '使用镜像默认值' }}</small></summary><div class="form-row"><div class="form-group"><div class="volume-heading"><label class="form-label sr-only">启动命令</label><button type="button" class="btn btn-sm" @click="addCommandItem">+ 添加命令</button></div><div v-if="templateForm.command_items.length" class="value-list"><div v-for="(item, index) in templateForm.command_items" :key="index" class="value-row"><input v-model="item.value" class="form-input" placeholder="/usr/bin/chromium-browser" /><button type="button" class="icon-button" title="移除启动命令" @click="removeCommandItem(index)"><Trash2 :size="16" /></button></div></div><p class="form-hint">留空则使用镜像默认入口命令。</p></div><div class="form-group"><div class="volume-heading"><label class="form-label">启动参数</label><button type="button" class="btn btn-sm" @click="addArgumentItem">+ 添加参数</button></div><div v-if="templateForm.argument_items.length" class="value-list"><div v-for="(item, index) in templateForm.argument_items" :key="index" class="value-row"><input v-model="item.value" class="form-input" placeholder="--remote-debugging-port=9222" /><button type="button" class="icon-button" title="移除启动参数" @click="removeArgumentItem(index)"><Trash2 :size="16" /></button></div></div><p class="form-hint">每行一个参数项，不执行 Shell 解析。</p></div></div></details>
      <div class="form-row"><div class="form-group"><label class="form-label">容器端口</label><input v-model.number="templateForm.container_port" type="number" min="1" class="form-input" required /></div><div class="form-group"><label class="form-label">副本数</label><input v-model.number="templateForm.replicas" type="number" min="1" class="form-input" :disabled="templateForm.volumes.length > 0" required /><p v-if="templateForm.volumes.length" class="form-hint">ReadWriteOnce 存储卷仅支持单副本。</p></div></div>
      <div class="form-group"><label class="form-label">部署节点</label><SelectMenu v-model="templateForm.node_name" class="form-select"><option value="">由 Kubernetes 调度</option><option v-for="node in clusterNodes" :key="node.name" :value="node.name">{{ node.display_name }} · hostname={{ node.name }}</option></SelectMenu><p class="form-hint">选择后将使用 kubernetes.io/hostname 标签调度；本地 PVC 必须与其已绑定节点一致，首次挂载 local-path PVC 时将决定数据落盘节点。</p></div></section>
      <section id="template-storage" v-show="activeTemplateSection === 'storage'" class="editor-section"><div class="editor-section-heading"><div><h3>配置与存储</h3><p>应用配置、敏感信息与挂载资源。</p></div></div><TemplateConfigurationEditor :form="templateForm" :configmaps="configResources" :secrets="secretResources" :application-id="applicationID" :template-id="editingTemplate?.id || ''" :revision="editingTemplate?.revision || 0" @error="mutationError = $event" /><details class="editor-disclosure" :open="templateForm.volumes.length > 0"><summary>数据卷挂载 <small>{{ templateForm.volumes.length ? templateForm.volumes.length + ' 项' : '未配置' }}</small></summary><div class="form-group"><div class="volume-heading"><label class="form-label sr-only">数据卷挂载</label><button type="button" class="btn btn-sm" :disabled="!mountableClaims.length" @click="addVolume">+ 添加挂载</button></div><div v-if="templateForm.volumes.length" class="volume-list"><div v-for="(volume, index) in templateForm.volumes" :key="index" class="volume-row"><SelectMenu v-model="volume.claim_name" class="form-select"><option v-for="claim in mountableClaims" :key="claim.name" :value="claim.name">{{ claim.name }} · {{ claim.phase || 'Pending' }} · {{ claim.storage || '容量未知' }}{{ claim.bound_node ? ` · ${claim.bound_node}` : '' }}{{ claim.managed ? ' · 平台' : ' · 已有' }}</option></SelectMenu><input v-model.trim="volume.mount_path" class="form-input" required placeholder="/data" /><label class="check-row"><input v-model="volume.read_only" type="checkbox" /> 只读</label><button type="button" class="icon-button" title="移除数据卷挂载" @click="removeVolume(index)"><Trash2 :size="16" /></button></div></div><p v-else class="form-hint">可绑定当前命名空间的已有 PVC；保存与发布时会检查状态、节点和其他工作负载占用。</p></div></details>
      <details class="editor-disclosure" :open="templateForm.file_mounts.some(item => item.source_type === 'secret' && ['tls.crt', 'tls.key'].includes(item.key))"><summary>TLS 证书 <small>证书文件挂载</small></summary><div class="form-group tls-certificate-field"><div class="volume-heading"><label class="form-label sr-only">TLS 证书</label><button type="button" class="btn btn-sm" @click="openCertificateManagement">申请证书</button></div><div class="tls-helper"><SelectMenu v-model="templateForm.tls_secret_name" class="form-select" aria-label="TLS 证书"><option value="">选择当前环境已就绪证书</option><option v-for="certificate in readyTLSCertificates" :key="certificate.id" :value="certificate.tls_secret_name">{{ certificate.hostname }} · {{ certificate.tls_secret_name }}</option></SelectMenu><input v-model.trim="templateForm.tls_mount_dir" class="form-input" aria-label="TLS 挂载目录" placeholder="/etc/tls" /><button type="button" class="btn btn-sm btn-primary" :disabled="!templateForm.tls_secret_name" @click="addTLSCertificateMounts">添加挂载</button></div><p class="form-hint">会添加标准 Secret 的 tls.crt 和 tls.key 文件挂载，不创建 HTTP Ingress。</p></div></details>
      </section>
      <section id="template-resources" v-show="activeTemplateSection === 'resources'" class="editor-section"><div class="editor-section-heading"><div><h3>资源与健康</h3><p>运行资源与容器就绪状态。</p></div></div><div class="form-row"><QuantityInput label="CPU 请求" v-model:value="templateForm.resources.requests_cpu" v-model:unit="templateForm.resources.requests_cpu_unit" :units="['m', '']" /><QuantityInput label="CPU 限制" v-model:value="templateForm.resources.limits_cpu" v-model:unit="templateForm.resources.limits_cpu_unit" :units="['m', '']" /></div>
      <div class="form-row"><QuantityInput label="内存请求" v-model:value="templateForm.resources.requests_memory" v-model:unit="templateForm.resources.requests_memory_unit" :units="['Mi', 'Gi']" /><QuantityInput label="内存限制" v-model:value="templateForm.resources.limits_memory" v-model:unit="templateForm.resources.limits_memory_unit" :units="['Mi', 'Gi']" /></div>
      <div v-if="!hasTCPServicePort" class="form-hint">仅 UDP Service 不支持当前 HTTP/TCP 健康检查；请通过 Pod 状态和 Service Endpoint 观察运行状态。</div><div v-else class="form-row"><div class="form-group"><label class="check-row"><input v-model="templateForm.health.readiness_enabled" type="checkbox" /> 启用就绪检查</label><div v-if="templateForm.health.readiness_enabled" class="form-row"><SelectMenu v-model="templateForm.health.readiness_type" class="form-select"><option value="http">HTTP</option><option value="tcp">TCP</option></SelectMenu><input v-if="templateForm.health.readiness_type === 'http'" v-model="templateForm.health.readiness_path" class="form-input" /></div></div><div class="form-group"><label class="check-row"><input v-model="templateForm.health.liveness_enabled" type="checkbox" /> 启用存活检查</label><div v-if="templateForm.health.liveness_enabled" class="form-row"><SelectMenu v-model="templateForm.health.liveness_type" class="form-select"><option value="http">HTTP</option><option value="tcp">TCP</option></SelectMenu><input v-if="templateForm.health.liveness_type === 'http'" v-model="templateForm.health.liveness_path" class="form-input" /></div></div></div></section>
      <section id="template-network" v-show="activeTemplateSection === 'network'" class="editor-section"><div class="editor-section-heading"><div><h3>服务网络</h3><p>Service 暴露方式和端口映射。</p></div></div><div class="form-group"><label class="check-row"><input v-model="templateForm.host_network" type="checkbox" aria-label="使用宿主机网络" /> 使用宿主机网络</label></div><div class="form-group"><label class="form-label">DNS 解析策略</label><SelectMenu v-model="templateForm.dns_ndots" class="form-select" aria-label="DNS 解析策略"><option value="">使用集群默认值</option><option :value="1">优先查询完整域名（ndots=1，推荐）</option><option v-if="templateForm.dns_ndots !== '' && templateForm.dns_ndots !== 1" :value="templateForm.dns_ndots">自定义（ndots={{ templateForm.dns_ndots }}）</option></SelectMenu><p class="form-hint">ndots=1 让带点的域名优先直接查询，减少对完整 Service 域名追加搜索后缀的请求；不改变 DNS 服务器，无法消除网络故障。</p></div><div class="form-row"><div class="form-group"><label class="form-label">Service 类型</label><SelectMenu v-model="templateForm.service.type" class="form-select"><option value="ClusterIP">ClusterIP</option><option value="NodePort">NodePort</option><option value="LoadBalancer">LoadBalancer</option></SelectMenu><p class="form-hint">{{ serviceTypeHint }}</p></div><div v-if="templateForm.service.type !== 'ClusterIP'" class="form-group"><label class="form-label">外部流量策略</label><SelectMenu v-model="templateForm.service.external_traffic_policy" class="form-select"><option value="">使用集群默认值</option><option value="Cluster">Cluster</option><option value="Local">Local</option></SelectMenu></div></div>
      <div class="form-group"><div class="volume-heading"><label class="form-label">Service 端口</label><button type="button" class="btn btn-sm" @click="addServicePort">+ 添加端口</button></div><div class="service-port-list" :class="{ 'with-node-port': templateForm.service.type === 'NodePort' }"><div class="service-port-headings"><span>名称</span><span>协议</span><span>Service 端口</span><span>Target Port</span><span v-if="templateForm.service.type === 'NodePort'">NodePort（可选）</span><span></span></div><div v-for="(servicePort, index) in templateForm.service.ports" :key="index" class="service-port-row"><input v-model.trim="servicePort.name" class="form-input" required aria-label="Service 端口名称" placeholder="端口名称" /><SelectMenu v-model="servicePort.protocol" class="form-select" aria-label="传输协议"><option value="TCP">TCP</option><option value="UDP">UDP</option></SelectMenu><input v-model.number="servicePort.port" type="number" min="1" class="form-input" required aria-label="Service 端口" placeholder="Service 端口" /><input v-model.number="servicePort.target_port" type="number" min="1" class="form-input" required aria-label="Target Port" placeholder="Target Port" /><input v-if="templateForm.service.type === 'NodePort'" v-model.number="servicePort.node_port" type="number" min="1" class="form-input" aria-label="NodePort" placeholder="自动分配" /><button type="button" class="icon-button" title="移除 Service 端口" :disabled="templateForm.service.ports.length === 1" @click="removeServicePort(index)"><Trash2 :size="16" /></button></div></div><p v-if="templateForm.service.type === 'NodePort'" class="form-hint">留空由 Kubernetes 自动分配 NodePort；发布完成后使用节点 IP 和实际分配端口访问。</p><p v-if="hasTCPServicePort" class="form-hint">HTTP 服务保存模板并发布后，在页面顶部“对外域名”绑定已就绪域名，平台会创建 HTTP Ingress 并转发到第一个 TCP Service 端口。Ingress 只转发 HTTP/HTTPS；其他 TCP 协议请使用 NodePort 或 LoadBalancer。</p><p v-else class="form-hint">仅 UDP Service 不支持 HTTP Ingress；请使用 NodePort 或 LoadBalancer 直接暴露端口。</p></div></section>
    </form></div><div class="modal-actions editor-actions"><span class="editor-save-hint">保存后需发布或滚动重启以生效</span><button type="button" class="btn" :disabled="saving" @click="closeTemplateEditor">取消</button><button form="template-editor-form" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存模板' }}</button></div></div></div></Teleport>
    <Teleport to="body"><div v-if="showEndpointEditor" class="overlay" @click.self="closeEndpointEditor"><div class="modal"><h2 class="modal-title">{{ editingEndpoint ? '编辑对外域名' : '绑定对外域名' }}</h2><form @submit.prevent="saveEndpoint">
      <div class="form-group"><label class="form-label">受管域名</label><SelectMenu v-model.number="endpointForm.domain_id" class="form-select" required><option :value="0" disabled>选择当前环境的域名</option><option v-for="domain in endpointDomains" :key="domain.id" :value="domain.id">{{ domain.hostname }}</option></SelectMenu><button type="button" class="btn btn-sm domain-management-link" @click="openCertificateManagement">导入/申请域名</button></div>
      <div class="form-row"><div class="form-group"><label class="form-label">Service 端口</label><SelectMenu v-model.number="endpointForm.service_port" class="form-select" required @change="syncEndpointProtocol"><option :value="0" disabled>选择端口</option><option v-for="port in endpointServicePorts" :key="`${port.port}-${port.protocol}`" :value="port.port">{{ port.port }} · {{ port.protocol }}{{ port.name ? ` · ${port.name}` : '' }}</option></SelectMenu></div><div class="form-group"><label class="form-label">协议</label><SelectMenu v-model="endpointForm.protocol" class="form-select" required><option value="TCP">TCP</option><option value="UDP">UDP</option></SelectMenu><p class="form-hint">必须与模板 Service 端口协议一致。</p></div></div>
      <div class="form-group"><label class="form-label">访问路径</label><input v-model.trim="endpointForm.path" class="form-input" required placeholder="/" /></div>
      <div class="form-group"><label class="form-label">访问入口</label><SelectMenu :model-value="endpointForm.ingress_enabled ? 'ingress' : 'record'" class="form-select" aria-label="访问入口" @update:model-value="endpointForm.ingress_enabled = $event === 'ingress'"><option value="record">仅记录域名</option><option v-if="endpointForm.protocol !== 'UDP'" value="ingress">启用 HTTP Ingress</option></SelectMenu><p class="form-hint">仅记录域名不会创建访问入口；HTTP Ingress 仅适用于 HTTP/HTTPS 服务。</p></div>
      <div v-if="endpointForm.ingress_enabled" class="form-group"><label class="form-label">入口用途</label><SelectMenu v-model="endpointForm.access_mode" class="form-select"><option value="public">普通访问</option><option value="protected_console">受保护控制台</option></SelectMenu><p class="form-hint">普通访问直接打开域名；受保护控制台打开前会由平台签发短期一次性会话。</p></div>
      <label class="check-row"><input v-model="endpointForm.tls_enabled" type="checkbox" /> {{ endpointForm.ingress_enabled ? '启用 HTTPS' : '标记为 HTTPS 域名' }}</label>
      <div class="modal-actions"><button type="button" class="btn" @click="closeEndpointEditor">取消</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存绑定' }}</button></div>
    </form></div></div></Teleport>
    <Teleport to="body"><div v-if="showLocalDebugEditor" class="overlay" @click.self="closeLocalDebugEditor"><div class="modal"><h2 class="modal-title">本地联调</h2><p class="form-hint">地址仅保存到当前浏览器。点击“本地打开”后，平台会签发一次性会话并跳转到此地址；本地前端可再代理 API 到云端服务。</p><form @submit.prevent="openLocalDebugEndpoint"><div class="form-group"><label class="form-label">本地访问地址</label><input v-model.trim="localDebugURL" class="form-input" required placeholder="http://localhost:5178" /><p class="form-hint">仅支持 http(s)://localhost、127.0.0.1 或 [::1]，可包含本地路径。</p></div><div class="modal-actions"><button type="button" class="btn" @click="closeLocalDebugEditor">取消</button><button class="btn btn-primary">本地打开</button></div></form></div></div></Teleport>
    <Teleport to="body"><div v-if="showRestartConfirmation" class="overlay" @click.self="closeRestartConfirmation"><div class="modal restart-modal"><h2 class="modal-title">重启应用</h2><p class="form-hint">将使用当前上线模板重建工作负载，并保留最近成功发布的镜像版本。模板中的配置变更将一并生效，服务可能短暂中断。</p><div class="modal-actions"><button type="button" class="btn" :disabled="restarting" @click="closeRestartConfirmation">取消</button><button type="button" class="btn btn-primary" :disabled="restarting" @click="restartApplication"><RefreshCw :size="14" :class="{ 'is-spinning': restarting }" />{{ restarting ? '重启中...' : '确认重启' }}</button></div></div></div></Teleport>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Copy, Pencil, RefreshCw, Settings, Trash2 } from 'lucide-vue-next'
import {
  createDeploymentTemplate,
  createEndpoint,
  createIntegrationHandoff,
  disableApplicationDeployWebhook,
  deleteDeploymentTemplate,
  deleteEndpoint,
  getApplication,
  getApplicationDeployWebhook,
  getApplicationConfigResources,
  getApplicationEndpoints,
  getApplicationSecretResources,
  getDeploymentTemplates,
  getImageRegistries,
  generateApplicationDeployWebhookSecret,
  restartApplication as restartApplicationRequest,
  saveApplicationCapabilities,
  setDefaultDeploymentTemplate,
  updateDeploymentTemplate,
  updateEndpoint,
  updateWorkloadKind as updateWorkloadKindRequest,
} from '../../api/applications.js'
import { getManagedDomains } from '../../api/domains.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePolling } from '../../composables/usePolling.js'
import { getClusterPersistentVolumeInventory } from '../../api/storage.js'
import { getClusterNodes, getClusterServers } from '../../api/cluster.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import BaseModal from '../../components/BaseModal.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import EmptyState from '../../components/EmptyState.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import ApplicationRuntime from './ApplicationRuntime.vue'
import TemplateConfigurationEditor from './TemplateConfigurationEditor.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { formatDateTime as formatTime } from '../../utils/formatters.js'

const QuantityInput = defineComponent({
  props: { label: String, value: Number, unit: String, units: Array }, emits: ['update:value', 'update:unit'],
  setup(props, { emit }) {
    return () => h('div', { class: 'form-group' }, [
      h('label', { class: 'form-label' }, props.label),
      h('div', { class: 'unit-input' }, [
        h('input', { class: 'form-input', type: 'number', min: 1, value: props.value, onInput: event => emit('update:value', Number(event.target.value)) }),
        h('select', { class: 'form-select', value: props.unit, onChange: event => emit('update:unit', event.target.value) }, props.units.map(unit => h('option', { value: unit }, unit || '核'))),
      ]),
    ])
  },
})

const props = defineProps({ applicationID: { type: String, required: true } })
const router = useRouter()
const detailTabs = [{ id: 'runtime', label: '运行概览' }, { id: 'releases', label: '发布记录' }, { id: 'config', label: '部署配置' }, { id: 'access', label: '访问入口' }]
const route = useRoute()
const activeTab = ref(detailTabs.some(tab => tab.id === route?.query?.tab) ? route.query.tab : 'runtime')
watch(() => route?.query?.tab, tab => { if (detailTabs.some(item => item.id === tab)) activeTab.value = tab })
const application = ref(null)
const releases = ref([])
const templates = ref([])
const endpoints = ref([])
const domains = ref([])
const registries = ref([])
const configResources = ref([])
const secretResources = ref([])
const persistentVolumeClaims = ref([])
const mountableClaims = computed(() => persistentVolumeClaims.value.filter(claim => !claim.read_only && (!claim.managed || !claim.environment_id || Number(claim.environment_id) === Number(application.value?.environment_id))))
const clusterNodes = ref([])
const storageResourcesLoaded = ref(false)
const storageResourcesLoading = ref(false)
const readError = ref('')
const mutationError = ref('')
const saving = ref(false)
const restarting = ref(false)
const deployWebhookStatus = ref(null)
const webhookBusy = ref(false)
const showWebhookSecretModal = ref(false)
const showWebhookDisableModal = ref(false)
const generatedWebhookSecret = ref('')
let deployWebhookStatusRequest = 0
const deployWebhookURL = computed(() => `${window.location.origin}/api/applications/${encodeURIComponent(props.applicationID)}/deployments`)
const showTemplateEditor = ref(false)
const activeTemplateSection = ref('basic')
const templateEditorFormElement = ref(null)
const editingTemplate = ref(null)
const showEndpointEditor = ref(false)
const editingEndpoint = ref(null)
const showLocalDebugEditor = ref(false)
const localDebugEndpoint = ref(null)
const showRestartConfirmation = ref(false)
const localDebugURL = ref('')
const workloadKind = ref('deployment')
const capabilityItems = ref([])
const savingCapabilities = ref(false)
const showApplicationSettings = ref(false)
const showWorkloadEditor = ref(false)
const templateForm = ref(newTemplateForm())
const endpointForm = ref(newEndpointForm())
const applicationResource = useAsyncResource(async ({ signal }) => {
  const result = await getApplication(props.applicationID, { signal })
  const namespace = result.application?.environment?.namespace || ''
  const clusterID = result.application?.environment?.cluster_id
  const [loadedTemplates, loadedEndpoints, loadedDomains, loadedRegistries, configmaps, secrets] = await Promise.all([
    getDeploymentTemplates(props.applicationID, { signal }),
    getApplicationEndpoints(props.applicationID, { signal }),
    getManagedDomains({ environmentID: result.application?.environment_id }, { signal }),
    getImageRegistries({ projectID: result.application?.project_id }, { signal }),
    clusterID ? getApplicationConfigResources(clusterID, namespace, { signal }) : Promise.resolve([]),
    clusterID ? getApplicationSecretResources(clusterID, namespace, { signal }) : Promise.resolve([]),
  ])
  return { result, loadedTemplates, loadedEndpoints, loadedDomains, loadedRegistries, configmaps, secrets }
}, null)
const releaseHistoryResource = useAsyncResource(({ signal }) => getApplication(props.applicationID, { signal }), null)
const releaseHistoryPolling = usePolling(refreshReleaseHistory, { interval: 5000 })
const storageResource = useAsyncResource(({ signal }, clusterID, namespace) => Promise.all([
  getClusterPersistentVolumeInventory(clusterID, { namespace }, { signal }),
  getClusterNodes({ signal }, clusterID),
  getClusterServers({ signal }),
]), null)

const readyDomains = computed(() => domains.value.filter(domain => domain.enabled && domain.certificate?.status === 'Ready'))
const endpointDomains = computed(() => domains.value.filter(domain => domain.enabled && (!endpointForm.value.ingress_enabled || !endpointForm.value.tls_enabled || domain.certificate?.status === 'Ready')))
const endpointServicePorts = computed(() => {
  const template = templates.value.find(item => item.is_default) || templates.value.find(item => item.enabled) || templates.value[0]
  return template ? servicePortsFromSpec(template.spec?.service, template.spec?.container_port || 8080) : []
})
const readyTLSCertificates = computed(() => readyDomains.value.filter(domain => domain.tls_secret_name))
const templateDependencies = computed(() => templates.value.flatMap(template => (template.spec?.file_mounts || []).map((mount, index) => {
  if (mount.source_type === 'application_config' || mount.source_type === 'application_secret') {
    const isSecret = mount.source_type === 'application_secret'
    return { id: `${template.id}-${index}`, template: template.name, type: isSecret ? '应用 Secret' : '应用 ConfigMap', name: `${application.value?.name || '应用'}-${isSecret ? 'secret' : 'config'}`, key: mount.key, path: mount.mount_path, ready: Object.prototype.hasOwnProperty.call(isSecret ? (template.spec?.secrets || {}) : (template.spec?.config || {}), mount.key) }
  }
  const resource = fileMountResources(mount.source_type).find(item => item.name === mount.source_name)
  return { id: `${template.id}-${index}`, template: template.name, type: mount.source_type === 'secret' ? 'Secret' : 'ConfigMap', name: mount.source_name, key: mount.key, path: mount.mount_path, ready: !!resource && (resource.keys || []).includes(mount.key) }
})))
const hasTCPServicePort = computed(() => templateForm.value.service.ports.some(port => port.protocol === 'TCP'))
const templateEditorSections = computed(() => {
  const form = templateForm.value
  return [
    { id: 'basic', label: '基础信息', summary: form.name || '模板与镜像' },
    { id: 'deployment', label: '服务部署', summary: `${form.replicas} 副本 · ${form.node_name || '自动调度'}` },
    { id: 'storage', label: '配置与存储', summary: `${[...form.config_items, ...form.secret_items].filter(item => !item.file_only).length} 个变量 · ${form.file_mounts.length} 个文件 · ${form.volumes.length} 个卷` },
    { id: 'resources', label: '资源与健康', summary: `${form.resources.limits_cpu}${form.resources.limits_cpu_unit || ' 核'} · ${form.resources.limits_memory}${form.resources.limits_memory_unit}` },
    { id: 'network', label: '服务网络', summary: `${form.service.type}${form.dns_ndots === '' ? '' : ` · ndots=${form.dns_ndots}`}` },
  ]
})
const templateEditorSummary = computed(() => `${templateForm.value.replicas} 副本 · ${templateServiceSummary(templateForm.value.service)}`)
const serviceTypeHint = computed(() => ({
  ClusterIP: '仅集群内访问；HTTP 服务可通过域名入口暴露。',
  NodePort: '通过节点 IP 和端口访问；需配置外部网络策略。',
  LoadBalancer: '由集群负载均衡器分配外部地址，适用于 TCP 或 UDP 直出。',
}[templateForm.value.service.type] || ''))

function newServicePort(index, targetPort) { return { name: `port-${index}`, port: 80, target_port: targetPort, protocol: 'TCP', node_port: null } }
function servicePortsFromSpec(service, containerPort) { const ports = service?.ports?.length ? service.ports : [{ name: 'service', port: service?.port || 80, target_port: service?.target_port || containerPort, protocol: service?.protocol || 'TCP', node_port: service?.node_port || null }]; return ports.map((port, index) => ({ name: port.name || `port-${index + 1}`, port: port.port, target_port: port.target_port, protocol: port.protocol || 'TCP', node_port: port.node_port || null })) }
function newTemplateForm() { return { name: '', description: '', enabled: true, default_version: '', registry_id: 0, image: '', command_items: [], argument_items: [], container_port: 8080, replicas: 1, node_name: '', host_network: false, dns_ndots: '', volumes: [], file_mounts: [], tls_secret_name: '', tls_mount_dir: '/etc/tls', config_items: [], secret_items: [], resources: { requests_cpu: 100, requests_cpu_unit: 'm', requests_memory: 128, requests_memory_unit: 'Mi', limits_cpu: 500, limits_cpu_unit: 'm', limits_memory: 512, limits_memory_unit: 'Mi' }, health: { readiness_enabled: true, readiness_type: 'http', readiness_path: '/healthz', liveness_enabled: false, liveness_type: 'http', liveness_path: '/healthz' }, service: { type: 'ClusterIP', external_traffic_policy: '', ports: [newServicePort(1, 8080)] } } }
function newEndpointForm() { return { domain_id: 0, path: '/', service_port: 0, protocol: 'TCP', tls_enabled: false, ingress_enabled: false, access_mode: 'public' } }
function releaseBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function templateName(id) { return templates.value.find(template => template.id === id)?.name }
function quantityInput(value, unit) { const match = String(value || '').match(/^(\d+)(m|Mi|Gi)?$/); return { value: match ? Number(match[1]) : 1, unit: match?.[2] ?? unit } }
function formFromTemplate(template) { const spec = template.spec; const requestCPU = quantityInput(spec.resources?.requests_cpu, 'm'), requestMemory = quantityInput(spec.resources?.requests_memory, 'Mi'), limitCPU = quantityInput(spec.resources?.limits_cpu, 'm'), limitMemory = quantityInput(spec.resources?.limits_memory, 'Mi'); const disabledConfig = new Set(spec.config_disabled || []), disabledSecrets = new Set(spec.secrets_disabled || []), managedConfig = new Set(spec.config_managed_keys || []), managedSecrets = new Set(spec.secret_managed_keys || []); if (!spec.config_managed_keys?.length) (spec.file_mounts || []).filter(mount => mount.managed && mount.source_type === 'application_config').forEach(mount => managedConfig.add(mount.key)); if (!spec.secret_managed_keys?.length) (spec.file_mounts || []).filter(mount => mount.managed && mount.source_type === 'application_secret').forEach(mount => managedSecrets.add(mount.key)); return { ...newTemplateForm(), name: template.name, description: template.description || '', enabled: template.enabled, default_version: template.default_version || '', registry_id: spec.registry_id || 0, image: spec.image, command_items: stringItems(spec.command), argument_items: stringItems(spec.args), container_port: spec.container_port, replicas: spec.replicas, node_name: spec.node_name || '', host_network: !!spec.host_network, dns_ndots: spec.dns_ndots ?? '', volumes: (spec.volumes || []).map(volume => ({ claim_name: volume.claim_name, mount_path: volume.mount_path, read_only: !!volume.read_only })), file_mounts: (spec.file_mounts || []).map(fileMount => ({ source_type: fileMount.source_type, source_name: fileMount.source_name, key: fileMount.key, mount_path: fileMount.mount_path })), config_items: keyValueItems(spec.config).map(item => ({ ...item, enabled: !disabledConfig.has(item.key), externally_managed: managedConfig.has(item.key), file_only: (spec.config_file_keys || []).includes(item.key) })), secret_items: Object.keys(spec.secrets || {}).map(key => ({ key, value: '', enabled: !disabledSecrets.has(key), externally_managed: managedSecrets.has(key), file_only: (spec.secret_file_keys || []).includes(key), original_key: key })), resources: { requests_cpu: requestCPU.value, requests_cpu_unit: requestCPU.unit, requests_memory: requestMemory.value, requests_memory_unit: requestMemory.unit, limits_cpu: limitCPU.value, limits_cpu_unit: limitCPU.unit, limits_memory: limitMemory.value, limits_memory_unit: limitMemory.unit }, health: { ...newTemplateForm().health, ...spec.health }, service: { type: spec.service?.type || 'ClusterIP', external_traffic_policy: spec.service?.external_traffic_policy || '', ports: servicePortsFromSpec(spec.service, spec.container_port) } } }
function stringItems(values) { return (values || []).map(value => ({ value })) }
function keyValueItems(values) { return Object.entries(values || {}).map(([key, value]) => ({ key, value, enabled: true, externally_managed: false })) }
function stringValues(items) { return items.map(item => item.value.trim()).filter(Boolean) }
function keyValueMap(items) { return items.reduce((result, item) => { if (item.key.trim()) result[item.key.trim()] = item.value; return result }, {}) }
function disabledKeys(items) { return items.filter(item => item.key.trim() && item.enabled === false).map(item => item.key.trim()) }
function fileMountResources(sourceType) { return sourceType === 'secret' ? secretResources.value : configResources.value }
function addTLSCertificateMounts() { const directory = templateForm.value.tls_mount_dir.replace(/\/$/, '') || '/etc/tls'; const secretName = templateForm.value.tls_secret_name; const mounts = templateForm.value.file_mounts; for (const key of ['tls.crt', 'tls.key']) { const mountPath = `${directory}/${key}`; const existing = mounts.find(item => item.mount_path === mountPath); if (existing) { existing.source_type = 'secret'; existing.source_name = secretName; existing.key = key } else mounts.push({ source_type: 'secret', source_name: secretName, key, mount_path: mountPath }) } }
function fileOnlyKeys(items) { return items.filter(item => item.key.trim() && item.file_only).map(item => item.key.trim()) }
function validateConfiguration(form) {
  for (const items of [form.config_items, form.secret_items]) {
    const keys = new Set()
    for (const item of items) {
      const key = item.key.trim()
      if (!key) throw new Error('请填写配置名称，或移除未使用的配置项')
      if (keys.has(key)) throw new Error(`配置名称 ${key} 重复，请修改后保存`)
      if (item.file_edit && item.original_key !== key) throw new Error('逐项编辑期间不能修改文件名称，请先保存或撤销修改')
      if (item.original_key && item.original_key !== key && !item.value) throw new Error('修改已保存的敏感配置名称时，请重新填写内容')
      keys.add(key)
    }
  }
  for (const item of form.secret_items) {
    if (!item.original_key && !item.value.trim()) throw new Error(`请填写敏感配置 ${item.key} 的内容`)
  }
  for (const file of form.file_mounts) {
    const items = file.source_type === 'application_config' ? form.config_items : file.source_type === 'application_secret' ? form.secret_items : null
    if (items && !items.some(item => item.key === file.key && item.enabled !== false)) throw new Error(`配置文件 ${file.key || '未命名'} 必须引用已启用的配置内容`)
  }
}
function managedKeys(items) { return items.filter(item => item.key.trim() && item.externally_managed).map(item => item.key.trim()) }
function templatePayload() { const form = templateForm.value; validateConfiguration(form); const service = { type: form.service.type, external_traffic_policy: form.service.type === 'ClusterIP' ? '' : form.service.external_traffic_policy, ports: form.service.ports.map(port => ({ name: port.name, port: port.port, target_port: port.target_port, protocol: port.protocol, node_port: form.service.type === 'NodePort' ? port.node_port || 0 : 0 })) }; const fileMounts = form.file_mounts.map(({ source_type, source_name, key, mount_path }) => ({ source_type, source_name, key, mount_path })); return { ...(editingTemplate.value ? { revision: editingTemplate.value.revision } : {}), name: form.name, description: form.description, enabled: form.enabled, default_version: form.default_version, ...(form.secret_items.some(item => item.file_edit?.changes?.length) ? { secret_file_edits: form.secret_items.filter(item => item.file_edit?.changes?.length).map(item => item.file_edit) } : {}), spec: { image: form.image, registry_id: form.registry_id, command: stringValues(form.command_items), args: stringValues(form.argument_items), container_port: form.container_port, replicas: form.replicas, node_name: form.node_name, host_network: form.host_network, ...(form.dns_ndots === '' ? {} : { dns_ndots: Number(form.dns_ndots) }), volumes: form.volumes, file_mounts: fileMounts, config: keyValueMap(form.config_items), config_disabled: disabledKeys(form.config_items), config_file_keys: fileOnlyKeys(form.config_items), config_managed_keys: managedKeys(form.config_items), secrets: keyValueMap(form.secret_items), secrets_disabled: disabledKeys(form.secret_items), secret_file_keys: fileOnlyKeys(form.secret_items), secret_managed_keys: managedKeys(form.secret_items), resources: { requests_cpu: `${form.resources.requests_cpu}${form.resources.requests_cpu_unit}`, requests_memory: `${form.resources.requests_memory}${form.resources.requests_memory_unit}`, limits_cpu: `${form.resources.limits_cpu}${form.resources.limits_cpu_unit}`, limits_memory: `${form.resources.limits_memory}${form.resources.limits_memory_unit}` }, health: hasTCPServicePort.value ? form.health : { ...form.health, readiness_enabled: false, liveness_enabled: false }, service } } }
function templateServiceSummary(service) { const ports = servicePortsFromSpec(service, 0); return `${service?.type || 'ClusterIP'} · ${ports.map(port => `${port.protocol} ${port.port}`).join(', ')}` }

async function loadApplication() {
  closeTemplateEditor()
  releaseHistoryPolling.stop()
  releaseHistoryResource.cancel()
  deployWebhookStatusRequest += 1
  deployWebhookStatus.value = null
  generatedWebhookSecret.value = ''
  showWebhookSecretModal.value = false
  showWebhookDisableModal.value = false
  readError.value = ''
  storageResourcesLoaded.value = false
  persistentVolumeClaims.value = []
  clusterNodes.value = []
  const loaded = await applicationResource.refresh()
  if (!loaded) {
    if (applicationResource.error.value) readError.value = applicationResource.error.value.message || '加载应用详情失败'
    return
  }
  const { result, loadedTemplates, loadedEndpoints, loadedDomains, loadedRegistries, configmaps, secrets } = loaded
  application.value = result.application
  capabilityItems.value = (result.application.capabilities || []).map(value => ({ value }))
  workloadKind.value = result.application.workload_kind || 'deployment'
  releases.value = result.releases || []
  templates.value = loadedTemplates || []
  endpoints.value = loadedEndpoints || []
  domains.value = loadedDomains || []
  registries.value = loadedRegistries || []
  configResources.value = configmaps || []
  secretResources.value = secrets || []
  syncReleaseHistoryPolling()
  if (activeTab.value === 'config') void loadDeployWebhookStatus()
}
async function loadDeployWebhookStatus() {
  const applicationID = props.applicationID
  const request = ++deployWebhookStatusRequest
  deployWebhookStatus.value = null
  try { const status = await getApplicationDeployWebhook(applicationID); if (request === deployWebhookStatusRequest && applicationID === props.applicationID) deployWebhookStatus.value = status } catch (e) { if (request === deployWebhookStatusRequest && applicationID === props.applicationID) { deployWebhookStatus.value = { unavailable: true }; mutationError.value = e.message || '读取自动发布配置失败' } }
}
async function generateWebhookSecret() {
  const applicationID = props.applicationID
  deployWebhookStatusRequest += 1
  webhookBusy.value = true
  try { const result = await generateApplicationDeployWebhookSecret(applicationID); if (applicationID === props.applicationID) { generatedWebhookSecret.value = result.secret; deployWebhookStatus.value = { configured: true } } } catch (e) { if (applicationID === props.applicationID) mutationError.value = e.message || '生成自动发布密钥失败' } finally { webhookBusy.value = false }
}
async function disableWebhook() {
  const applicationID = props.applicationID
  deployWebhookStatusRequest += 1
  webhookBusy.value = true
  try { await disableApplicationDeployWebhook(applicationID); if (applicationID === props.applicationID) { deployWebhookStatus.value = { configured: false }; showWebhookDisableModal.value = false } } catch (e) { if (applicationID === props.applicationID) mutationError.value = e.message || '关闭自动发布失败' } finally { webhookBusy.value = false }
}
function closeWebhookSecretModal() { if (!webhookBusy.value) { generatedWebhookSecret.value = ''; showWebhookSecretModal.value = false } }
async function copyWebhookValue(value) { try { await navigator.clipboard.writeText(value) } catch { mutationError.value = '复制失败，请手动复制' } }
async function refreshReleaseHistory() {
  const applicationID = props.applicationID
  const result = await releaseHistoryResource.refresh()
  if (result && activeTab.value === 'releases' && applicationID === props.applicationID) {
    releases.value = result.releases || []
    readError.value = ''
  } else if (releaseHistoryResource.error.value && activeTab.value === 'releases') {
    readError.value = releaseHistoryResource.error.value.message || '更新发布记录失败'
  }
}
function syncReleaseHistoryPolling(immediate = false) {
  if (activeTab.value === 'releases' && application.value && !document.hidden) {
    releaseHistoryPolling.start({ immediate })
  } else {
    releaseHistoryPolling.stop()
    releaseHistoryResource.cancel()
  }
}
async function loadStorageResources() {
  if (storageResourcesLoaded.value || storageResourcesLoading.value || !application.value) return
  storageResourcesLoading.value = true
  try {
    const clusterID = application.value.environment?.cluster_id
    if (!clusterID) return
    const result = await storageResource.refresh(clusterID, application.value.environment?.namespace)
    if (!result) return
    const [items, clusterNodesResult, servers] = result
    persistentVolumeClaims.value = items || []
    const serverNames = new Map((servers || []).filter(server => Number(server.cluster_id) === Number(clusterID) && server.k8s_node_name).map(server => [server.k8s_node_name, server.name]))
    clusterNodes.value = (clusterNodesResult || []).map(node => ({ name: node.name, display_name: serverNames.get(node.name) || node.name }))
    storageResourcesLoaded.value = true
  } finally { storageResourcesLoading.value = false }
}
async function openRelease(release) { await router.push(`/applications/${props.applicationID}/releases/${release.id}`) }
function openTemplateEditor(template = null) { editingTemplate.value = template; templateForm.value = template ? formFromTemplate(template) : newTemplateForm(); activeTemplateSection.value = 'basic'; showTemplateEditor.value = true; loadStorageResources() }
function selectTemplateSection(section) {
  activeTemplateSection.value = section
  nextTick(() => { if (templateEditorFormElement.value) templateEditorFormElement.value.scrollTop = 0 })
}
let invalidTemplateField = null
let reportingTemplateValidity = false
function revealInvalidTemplateSection(event) {
  if (reportingTemplateValidity) return
  event.preventDefault()
  if (invalidTemplateField) return
  const field = event.target
  const section = field.closest('.editor-section')
  if (!section) return
  invalidTemplateField = field
  selectTemplateSection(section.id.replace('template-', ''))
  nextTick(() => {
    try {
      for (let parent = field.parentElement; parent && parent !== templateEditorFormElement.value; parent = parent.parentElement) {
        if (parent.tagName === 'DETAILS') parent.open = true
      }
      // SelectMenu uses a hidden native select for validation; focus its trigger.
      const focusTarget = field.classList.contains('select-menu-native') ? field.parentElement.querySelector('button') : field
      focusTarget?.focus()
      if (focusTarget === field) {
        reportingTemplateValidity = true
        field.reportValidity()
      } else { mutationError.value = field.validationMessage || '请检查当前分区中的必填选项' }
    } finally { invalidTemplateField = null; reportingTemplateValidity = false }
  })
}
function closeTemplateEditor() { showTemplateEditor.value = false; editingTemplate.value = null; templateForm.value = newTemplateForm() }
function addCommandItem() { templateForm.value.command_items.push({ value: '' }) }
function removeCommandItem(index) { templateForm.value.command_items.splice(index, 1) }
function addArgumentItem() { templateForm.value.argument_items.push({ value: '' }) }
function removeArgumentItem(index) { templateForm.value.argument_items.splice(index, 1) }
function openApplicationSettings() { workloadKind.value = application.value.workload_kind || 'deployment'; capabilityItems.value = (application.value.capabilities || []).map(value => ({ value })); showApplicationSettings.value = true }
function closeApplicationSettings() { if (!savingCapabilities.value) showApplicationSettings.value = false }
function closeWorkloadEditor() { if (!saving.value) showWorkloadEditor.value = false }
function addCapability() { capabilityItems.value.push({ value: '' }) }
function removeCapability(index) { capabilityItems.value.splice(index, 1) }
async function saveCapabilities() { savingCapabilities.value = true; mutationError.value = ''; try { const capabilities = capabilityItems.value.map(item => item.value.trim()).filter(Boolean); const updated = await saveApplicationCapabilities(props.applicationID, capabilities); application.value = updated; capabilityItems.value = (updated.capabilities || []).map(value => ({ value })); showApplicationSettings.value = false } catch (e) { mutationError.value = e.message || '保存能力标签失败' } finally { savingCapabilities.value = false } }
function isProtectedConsole(endpoint) { return endpoint.access_mode === 'protected_console' }
function endpointAccessLabel(endpoint) { return isProtectedConsole(endpoint) ? '受保护控制台' : '普通访问' }
function endpointURL(endpoint) { return `${endpoint.tls_enabled ? 'https' : 'http'}://${endpoint.domain}${endpoint.path || '/'}` }
function localDebugStorageKey(endpoint) { return `cylism.protected-console.local-url:${endpoint.id}` }
async function openApplicationEndpoint(endpoint) { mutationError.value = ''; try { if (!isProtectedConsole(endpoint)) { window.open(endpointURL(endpoint), '_blank', 'noopener,noreferrer'); return } await openProtectedConsole(endpoint) } catch (e) { mutationError.value = e.message || '打开应用入口失败' } }
async function openProtectedConsole(endpoint, redirectURL = '') { const body = { endpoint_id: endpoint.id }; if (redirectURL) body.redirect_url = redirectURL; const result = await createIntegrationHandoff(props.applicationID, body); if (!result.handoff_url) throw new Error('应用入口跳转创建失败'); window.open(result.handoff_url, '_blank', 'noopener,noreferrer') }
function openLocalDebugEditor(endpoint) { localDebugEndpoint.value = endpoint; localDebugURL.value = window.localStorage.getItem(localDebugStorageKey(endpoint)) || 'http://localhost:5178'; showLocalDebugEditor.value = true }
function closeLocalDebugEditor() { showLocalDebugEditor.value = false; localDebugEndpoint.value = null; localDebugURL.value = '' }
async function openLocalDebugEndpoint() { mutationError.value = ''; try { const endpoint = localDebugEndpoint.value; if (!endpoint) return; window.localStorage.setItem(localDebugStorageKey(endpoint), localDebugURL.value); await openProtectedConsole(endpoint, localDebugURL.value); closeLocalDebugEditor() } catch (e) { mutationError.value = e.message || '打开本地联调失败' } }
function addVolume() { const claim = mountableClaims.value[0]; if (claim) templateForm.value.volumes.push({ claim_name: claim.name, mount_path: '/data', read_only: false }) }
function removeVolume(index) { templateForm.value.volumes.splice(index, 1) }
async function openResourceManagement() { await router.push(`/cluster/${application.value.environment.cluster_id}/resources?tab=configs`) }
async function openCertificateManagement() {
  if (!application.value) return
  await router.push({ path: '/applications/domains', query: {
    project_id: application.value.project_id,
    environment_id: application.value.environment_id,
    application_id: props.applicationID,
    return_tab: activeTab.value,
  } })
}
function addServicePort() { const ports = templateForm.value.service.ports; ports.push(newServicePort(ports.length + 1, templateForm.value.container_port)) }
function removeServicePort(index) { if (templateForm.value.service.ports.length > 1) templateForm.value.service.ports.splice(index, 1) }
async function saveTemplate() { saving.value = true; mutationError.value = ''; try { const payload = templatePayload(); if (editingTemplate.value) await updateDeploymentTemplate(props.applicationID, editingTemplate.value.id, payload); else await createDeploymentTemplate(props.applicationID, payload); closeTemplateEditor(); await loadApplication() } catch (e) { mutationError.value = e.message || '保存上线模板失败' } finally { saving.value = false } }
function openRestartConfirmation() { showRestartConfirmation.value = true }
function closeRestartConfirmation() { if (!restarting.value) showRestartConfirmation.value = false }
async function restartApplication() { restarting.value = true; mutationError.value = ''; try { const release = await restartApplicationRequest(props.applicationID); releases.value = [release, ...releases.value]; showRestartConfirmation.value = false } catch (e) { mutationError.value = e.message || '重启应用失败' } finally { restarting.value = false } }
async function setDefaultTemplate(template) { mutationError.value = ''; try { await setDefaultDeploymentTemplate(props.applicationID, template.id); await loadApplication() } catch (e) { mutationError.value = e.message || '设置默认模板失败' } }
async function deleteTemplate(template) { mutationError.value = ''; try { await deleteDeploymentTemplate(props.applicationID, template.id); await loadApplication() } catch (e) { mutationError.value = e.message || '删除上线模板失败' } }
function openEndpointEditor(endpoint = null) { editingEndpoint.value = endpoint; if (endpoint) { const selected = endpointServicePorts.value.find(port => port.port === endpoint.service_port); endpointForm.value = { domain_id: endpoint.domain_id, path: endpoint.path || '/', service_port: endpoint.service_port || selected?.port || 0, protocol: endpoint.protocol || selected?.protocol || 'TCP', tls_enabled: endpoint.tls_enabled, ingress_enabled: endpoint.ingress_enabled !== false && (endpoint.protocol || selected?.protocol || 'TCP') !== 'UDP', access_mode: endpoint.access_mode || 'public' } } else { const form = newEndpointForm(); const first = endpointServicePorts.value[0]; if (first) { form.service_port = first.port; form.protocol = first.protocol } endpointForm.value = form } showEndpointEditor.value = true }
function syncEndpointProtocol() { const selected = endpointServicePorts.value.find(port => port.port === endpointForm.value.service_port); if (!selected) return; endpointForm.value.protocol = selected.protocol; if (selected.protocol === 'UDP') endpointForm.value.ingress_enabled = false }
function closeEndpointEditor() { showEndpointEditor.value = false; editingEndpoint.value = null; endpointForm.value = newEndpointForm() }
async function saveEndpoint() { saving.value = true; mutationError.value = ''; try { const current = editingEndpoint.value; const payload = { ...endpointForm.value }; if (!payload.service_port) { delete payload.service_port; delete payload.protocol } const saved = current ? await updateEndpoint(props.applicationID, current.id, payload) : await createEndpoint(props.applicationID, payload); if (current) endpoints.value = endpoints.value.map(endpoint => endpoint.id === saved.id ? saved : endpoint); else endpoints.value = [...endpoints.value, saved]; closeEndpointEditor() } catch (e) { mutationError.value = e.message || '保存域名绑定失败' } finally { saving.value = false } }
async function removeEndpoint(endpoint) { mutationError.value = ''; try { await deleteEndpoint(props.applicationID, endpoint.id); endpoints.value = endpoints.value.filter(item => item.id !== endpoint.id) } catch (e) { mutationError.value = e.message || '解绑域名失败' } }
async function updateWorkloadKind() { saving.value = true; mutationError.value = ''; try { application.value = await updateWorkloadKindRequest(props.applicationID, workloadKind.value); showWorkloadEditor.value = false } catch (e) { mutationError.value = e.message || '迁移工作负载失败'; } finally { saving.value = false } }
watch(() => props.applicationID, loadApplication, { immediate: true })
watch(activeTab, () => syncReleaseHistoryPolling(true))
watch(activeTab, tab => { if (tab === 'config' && application.value) void loadDeployWebhookStatus() })
onMounted(() => document.addEventListener('visibilitychange', onVisibilityChange))
onBeforeUnmount(() => document.removeEventListener('visibilitychange', onVisibilityChange))
function onVisibilityChange() { syncReleaseHistoryPolling(true) }
watch(() => endpointForm.value.protocol, protocol => { if (protocol === 'UDP') endpointForm.value.ingress_enabled = false })
watch(hasTCPServicePort, hasTCP => { if (!hasTCP) { templateForm.value.health.readiness_enabled = false; templateForm.value.health.liveness_enabled = false } })
watch(() => templateForm.value.volumes.length, count => { if (count) templateForm.value.replicas = 1 })
watch(() => templateForm.value.volumes.map(volume => volume.claim_name).join(','), () => { const bound = templateForm.value.volumes.map(volume => persistentVolumeClaims.value.find(claim => claim.name === volume.claim_name)).find(claim => claim?.bound_node); if (bound?.bound_node) templateForm.value.node_name = bound.bound_node })
</script>

<style scoped>
.back-link{display:inline-flex;align-items:center;gap:6px;margin-bottom:var(--space-16);color:var(--text-secondary);font-size:13px;font-weight:600;text-decoration:none}.back-link:hover{color:var(--action-primary)}.page-header,.section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-16)}.page-header{min-height:42px;align-items:center;margin-bottom:0}.page-header .page-title{min-width:0;overflow-wrap:anywhere}.page-subtitle,.section-heading p,.cell-primary small,.endpoint-row small,.dependency-row small,.empty-inline{margin:4px 0 0;color:var(--text-secondary);font-size:13px}.page-subtitle{margin-bottom:var(--space-24)}.detail-section{padding:var(--space-20) 0;border-top:1px solid var(--border-muted)}.section-heading{margin-bottom:var(--space-12)}.section-heading h2{margin:0;font-size:16px}.section-heading p{font-size:12px}.cell-primary small,.endpoint-row small,.dependency-row small{display:block;font-size:11px}.endpoint-list,.dependency-list{display:grid;gap:8px}.endpoint-row,.dependency-row{display:flex;align-items:center;justify-content:space-between;gap:var(--space-12);padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.endpoint-url{color:var(--action-primary);font-weight:700;text-decoration:none}.endpoint-url:hover{text-decoration:underline}.workload-kind-row{display:flex;align-items:center;gap:8px}.workload-kind-row .form-select{width:180px}.empty-inline{padding:12px 0}.editor-modal{width:min(900px,calc(100vw - 32px))}.check-row{display:flex;align-items:center;gap:8px;margin:var(--space-12) 0;color:var(--text-secondary);font-size:13px}.unit-input{display:grid;grid-template-columns:minmax(0,1fr) 76px;gap:6px}.volume-heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-12)}.volume-list,.service-port-list{display:grid;gap:8px}.volume-heading .form-label{margin:0}.volume-row{display:grid;grid-template-columns:minmax(160px,1fr) minmax(120px,1fr) auto 30px;align-items:center;gap:8px;padding:8px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.service-port-row{display:grid;grid-template-columns:minmax(100px,.8fr) 90px minmax(100px,.8fr) minmax(110px,.8fr) minmax(140px,1fr) 30px;align-items:center;gap:8px;padding:8px;border:1px solid var(--border-muted);border-radius:var(--control);background:var(--surface-subtle)}.volume-row .check-row{margin:0;white-space:nowrap}@media(max-width:640px){.page-header,.section-heading,.endpoint-row,.dependency-row{align-items:stretch;flex-direction:column}.page-header{align-items:flex-start}.page-header>.btn{width:100%}.volume-row,.service-port-row{grid-template-columns:1fr 1fr auto}.workload-kind-row{align-items:stretch;flex-direction:column}.workload-kind-row .form-select{width:100%}}
.editor-modal{width:min(1180px,calc(100vw - 32px));padding:0;overflow:hidden}.editor-modal{display:flex;flex-direction:column;max-height:calc(100dvh - 32px)}
.editor-header{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-16);padding:var(--space-20) var(--space-24);border-bottom:1px solid var(--border-muted)}
.editor-header .modal-title{margin:0}.editor-header p{margin:5px 0 0;color:var(--text-secondary);font-size:12px}.editor-header .check-row{margin:0;white-space:nowrap}
.template-editor-layout{display:grid;grid-template-columns:180px minmax(0,1fr);height:min(560px,calc(100dvh - 210px));min-height:0}
.editor-nav{display:flex;flex-direction:column;gap:4px;overflow-y:auto;padding:var(--space-12);border-right:1px solid var(--border-muted);background:var(--surface-subtle)}
.editor-nav button{display:flex;flex-direction:column;gap:5px;min-height:58px;padding:10px;border:1px solid transparent;border-radius:var(--radius-control);background:transparent;color:var(--text-secondary);font-size:12px;font-weight:600;text-align:left;cursor:pointer}
.editor-nav button small{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text-secondary);font-size:10px;font-weight:400}
.editor-nav button:hover{background:var(--surface-hover);color:var(--text-primary)}
.editor-nav button.active{border-color:var(--border);background:var(--surface-raised);color:var(--action-primary)}
.editor-nav button:focus-visible{outline:2px solid var(--focus);outline-offset:-2px}
.template-editor-form{min-width:0;min-height:0;overflow-y:auto;overscroll-behavior:contain}
.editor-section{padding:var(--space-24)}
.editor-section-heading{margin-bottom:var(--space-20)}
.editor-section-heading h3{margin:0;color:var(--text-primary);font-size:15px}.editor-section-heading p{margin:4px 0 0;color:var(--text-secondary);font-size:12px}
.editor-section .form-group:last-child{margin-bottom:0}
.editor-actions{flex:none;margin:0;padding:var(--space-16) var(--space-24);border-top:1px solid var(--border-muted);background:var(--surface-raised)}
.editor-save-hint{margin-right:auto;color:var(--text-secondary);font-size:11px}
.editor-disclosure{margin-bottom:var(--space-12);border-bottom:1px solid var(--border-muted)}
.editor-disclosure>summary{padding:var(--space-12) 0;color:var(--text-primary);font-size:12px;font-weight:600;cursor:pointer}
.editor-disclosure>summary small{margin-left:8px;color:var(--text-secondary);font-size:11px;font-weight:400}
.editor-disclosure>.form-group,.editor-disclosure>.form-row{padding:0 0 var(--space-16)}
@media(max-width:760px){
.editor-modal{width:calc(100vw - 16px);max-height:calc(100dvh - 16px)}.editor-header{padding:var(--space-16)}
.template-editor-layout{grid-template-columns:minmax(0,1fr);grid-template-rows:auto minmax(0,1fr);height:calc(100dvh - 190px)}
.editor-nav{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:4px;overflow:visible;padding:8px;border-right:0;border-bottom:1px solid var(--border-muted)}
.editor-nav button{flex:none;min-height:36px;padding:8px 10px;white-space:nowrap;align-items:center;justify-content:center}.editor-nav button small{display:none}
.editor-section{padding:var(--space-16)}.editor-actions{padding:var(--space-12) var(--space-16)}.editor-save-hint{display:none}
}
.volume-heading{margin-bottom:var(--space-12)}.tls-certificate-field{max-width:760px}.tls-helper{display:grid;grid-template-columns:minmax(220px,1fr) minmax(150px,.45fr) auto;gap:8px;align-items:center}.tls-helper .btn{white-space:nowrap}@media(max-width:640px){.tls-helper{grid-template-columns:1fr}.tls-helper .btn{width:100%}}
.value-list{display:grid;gap:8px}.value-row{display:grid;align-items:center;gap:8px;padding:8px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.value-row{grid-template-columns:minmax(0,1fr) 30px}.editor-section :deep(.unit-input){display:grid;grid-template-columns:minmax(0,1fr) 80px;gap:8px}.editor-section :deep(.unit-input .form-input),.editor-section :deep(.unit-input .form-select){min-width:0}@media(max-width:640px){.editor-section :deep(.unit-input){grid-template-columns:minmax(0,1fr) 72px}}
.capability-list{display:grid;gap:8px;max-width:520px}.capability-row{display:grid;grid-template-columns:minmax(0,1fr) 30px;gap:8px;align-items:center}.section-actions{margin-top:var(--space-12)}
.config-toggle-columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-24);max-width:760px}.config-toggle-columns>div{display:grid;gap:4px}.config-toggle-columns strong{font-size:12px;color:var(--text-secondary)}.config-toggle-columns .check-row{margin:2px 0}@media(max-width:640px){.config-toggle-columns{grid-template-columns:1fr;gap:var(--space-12)}}
.service-port-row{border-radius:var(--radius-control)}
.restart-modal{width:min(480px,calc(100vw - 32px))}.restart-modal .form-hint{margin:10px 0 22px;line-height:1.6}
.delegation-modal{width:min(620px,100%)}.delegation-token{min-height:118px;resize:vertical;font-family:var(--font-mono);font-size:11px;line-height:1.55;word-break:break-all}.copy-notice{margin:8px 0 0;color:var(--success);font-size:12px}.form-error{margin:8px 0 0;color:var(--danger);font-size:12px}
.detail-heading{grid-column:1;grid-row:1;display:flex;min-width:0;flex-direction:column;justify-content:center;gap:8px}
.detail-context{display:flex;min-height:26px;align-items:center;gap:16px;color:var(--text-secondary);font-size:12px}
.detail-context .back-link{flex:none;margin-bottom:0}
.detail-title-placeholder{width:min(280px,65%);height:30px;border-radius:var(--radius-control);background:var(--surface-hover)}
.page-header.detail-tabs-header{display:grid;grid-template-columns:minmax(0,1fr) auto;grid-template-rows:minmax(76px,auto) 48px;column-gap:16px;row-gap:0;height:auto;min-height:124px;align-items:center}
.detail-tabs-header :deep(.page-title){min-width:0;margin:0;font-size:var(--tabbed-page-header-title-size);overflow-wrap:anywhere}
.detail-tabs-header :deep(.section-tabs-actions){grid-column:2;grid-row:1;margin:0}
.detail-tabs-header :deep(.section-tabs){grid-column:1/-1;grid-row:2;align-self:end;gap:26px;margin:0;overflow-x:auto}
.detail-tabs-header :deep(.section-tab){flex:none;padding-bottom:13px;white-space:nowrap}
.detail-tabs-header::after{bottom:0}
.detail-tab-card{margin-top:var(--space-16)}
.detail-config-card .detail-section,.detail-access-card .detail-section{padding:18px 20px}
.detail-config-card .detail-section:first-child,.detail-access-card .detail-section:first-child{border-top:0}
.detail-card-toolbar{display:flex;min-height:62px;align-items:center;justify-content:space-between;gap:12px;padding:12px 20px;color:var(--text-primary);font-size:13px;font-weight:650}
.detail-count{color:var(--text-secondary);font-size:12px;font-weight:400;white-space:nowrap}
.detail-heading-actions{display:flex;align-items:center;justify-content:flex-end;flex-wrap:wrap;gap:8px;white-space:nowrap}
.domain-management-link{margin-top:8px}
.detail-table-wrap{padding:0 20px 12px}
.detail-access-card .endpoint-row{border:0;border-bottom:1px solid var(--border-muted);border-radius:0;background:transparent;padding:12px 0}
.detail-access-card .endpoint-row:last-child{border-bottom:0}
.deploy-webhook-endpoint{display:flex;max-width:850px;align-items:center;gap:8px}.deploy-webhook-endpoint .form-input{min-width:0;font-family:var(--font-mono);font-size:12px}.deploy-webhook-endpoint .icon-button{flex:none}.deploy-webhook-example{margin-top:14px;color:var(--text-secondary);font-size:12px}.deploy-webhook-example summary{width:max-content;cursor:pointer}.deploy-webhook-example pre{max-width:850px;overflow-x:auto;margin:10px 0 0;padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);font:11px/1.6 var(--font-mono)}.deploy-webhook-secret{display:block;overflow-wrap:anywhere;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);font:12px/1.5 var(--font-mono)}
@media(max-width:640px){.detail-context{gap:10px}.page-header.detail-tabs-header{grid-template-columns:minmax(0,1fr);grid-template-rows:auto auto 46px;padding-top:10px}.detail-heading{grid-row:1}.detail-tabs-header :deep(.section-tabs-actions){grid-column:1;grid-row:2;justify-self:start}.detail-tabs-header :deep(.section-tabs){grid-row:3;width:100%;gap:22px}.detail-config-card .detail-section,.detail-access-card .detail-section{padding:16px 14px}.detail-card-toolbar{padding:12px 14px}.detail-table-wrap{padding:0 14px 10px}}

/* Keep frequent template actions prominent and application settings compact. */
.template-section .detail-table-wrap{padding:0;overflow-x:auto}
.template-section .section-heading{align-items:center}
.template-section .data-table{min-width:780px}
.template-image-cell{max-width:clamp(180px,24vw,360px)}
.template-runtime-cell{min-width:180px;line-height:1.6}
.application-setting-group{padding:var(--space-16) 0}
.application-setting-group:first-child{padding-top:0}
.application-setting-group + .application-setting-group{border-top:1px solid var(--border-muted)}
.application-setting-group h3{margin:0 0 var(--space-8);font-size:13px}
.application-setting-group .section-heading{flex-direction:row;align-items:flex-start}
.application-setting-group .section-heading>.btn{flex:none}
.application-setting-group .settings-empty{min-height:64px}
.settings-workload-row{display:flex;align-items:center;gap:8px;margin-top:var(--space-12)}
.settings-workload-row .form-select{flex:1;min-width:0}
.settings-workload-row>.btn{flex:none}
.workload-migration-summary{font-size:13px;font-weight:600}
.deploy-webhook-section .section-heading{align-items:center;margin-bottom:0}
.deploy-webhook-details{margin-top:var(--space-12);font-size:12px;color:var(--text-secondary)}
.deploy-webhook-details>summary,.config-dependencies>summary{cursor:pointer}
.deploy-webhook-details .deploy-webhook-endpoint{margin-top:var(--space-12)}
.config-dependencies{font-size:12px;color:var(--text-secondary)}
.config-dependencies>summary{display:list-item}
.config-dependencies>summary .badge{margin-left:8px}
.config-dependencies .section-heading{margin-top:var(--space-16)}
.workload-editor .form-hint{margin:0 0 var(--space-16);line-height:1.6}
@media(max-width:640px){.deploy-webhook-section .section-heading{flex-wrap:wrap}.deploy-webhook-section .detail-heading-actions{justify-content:flex-start}}
</style>
