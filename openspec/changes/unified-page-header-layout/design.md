# 设计

## 页面分类

页面级标题区只允许以下两种组件：

| 页面类型 | 组件 | 适用场景 |
| --- | --- | --- |
| 工作区页面 | `SectionTabsHeader` | 页面有一个或多个同级工作区视图；只有一个视图时使用单 Tab |
| 详情/编辑页面 | `PageHeader` | 页面没有工作区切换，标题下可有说明，右侧可放操作 |

工作区页面的单 Tab 不是新的导航层，而是统一的页面身份和视觉结构。它必须使用固定的 `activeTab`，不改变路由，也不创建额外的父级页面。

## 迁移清单

### 统一为 `SectionTabsHeader`

- `AuditLogs`：单 Tab“审计记录”
- `OperationHistory`：单 Tab“执行记录”
- `SystemSettings`：多 Tab“安全与访问 / 平台入口 / 发布与更新”
- `Servers`：现有服务器分类 Tab
- `ClusterHub`：现有集群分类 Tab
- `NetworkHub`：现有网络分类 Tab
- `ResourceHub`：现有资源分类 Tab
- `Monitoring`：现有监控分类 Tab
- `RuntimeManagement`：现有运行时分类 Tab
- `Applications`：将工作台/项目与环境等工作区状态接入统一标题区；保留其领域筛选和操作区

### 保留 `PageHeader`

- `ApplicationDetails`、`ProjectEnvironments`、`ReleaseDetails`
- `CertificateOperations`、服务器终端、Pod 终端和聊天/弹窗承载页面
- 资源详情、表单编辑和其他不具备工作区 Tab 语义的页面

### 仅收敛样式，不强行 Tab 化

`ClusterDNS`、`SystemComponents`、`PersistentVolumes`、`NodeRegistryMirrors`、`ChartRepositories`、`Services`、`Configs`、`Sites`、`Certificates`、`ImageRegistries`、`ManagedOCIRegistries` 等直接资源页面继续由页面拥有内容和操作；若其标题区属于列表工作区，则使用单 Tab，否则使用标准 `PageHeader`。判断依据是页面是否存在同级视图切换，而不是文件所在目录。

## 视觉契约

- 桌面端页面级标题区统一高度为 `64px`。
- 页面级标题统一使用 `25px`、`700` 的标题规格。
- `SectionTabsHeader` 的标题、Tab 和横线属于同一标题区；横线位置固定，不通过改变外层高度补偿内容。
- 页面主体从标题区后统一使用共享间距 token。
- 操作区位于标题区右侧，不改变标题区高度；窄屏时按组件既有规则换行。
- 不增加“记录与系统”等重复的内容区父标题，不改变现有侧栏和路由层级。

## 测试与验证

- 组件测试验证两个标题组件各只有一个语义 `h1`，标题区高度和操作区结构符合契约。
- 工作区页面测试验证单 Tab 不触发路由变化，多 Tab 保持既有 Tab 选择行为。
- 迁移页面测试验证原有数据加载、筛选、分页、提交、弹窗和错误状态不变。
- 完成后运行前端全量测试、生产构建、后端测试/构建、OpenSpec 严格校验和 diff 检查。
