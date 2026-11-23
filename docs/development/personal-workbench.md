# 研发工作台与项目中心

入口为顶部「项目」→「研发工作台 / 项目中心」。使用流程见[项目研发与发行](../application-delivery/product-lifecycle.md)。

## 数据边界

- Project 是业务与部署边界；Product 作为一对一生命周期资料，使用可空唯一的 project_id 连接，保留旧版本、事项和发行 ID。
- Environment 的 kind 保存 unclassified / development / testing / staging / production；旧环境默认待分类。
- Application、Release 继续承载部署组件和真实发布。ProductVersion 组织迭代，可关联多次部署。
- ProductDistribution、ProductDistributionEvent 保存 audience：unclassified / testing / production；正式渠道汇总只查询正式记录及正式发布快照。
- 历史 product_projects / product_applications 引用继续保留，归档不会卸载或修改部署资源。

## API

接口均位于 JWT /api 路由组：

- GET /projects/:projectID/workspace：按当前用户所有权读取项目生命周期资料。
- DELETE /projects/:projectID/workspace：事务删除无环境、无应用的自有项目及生命周期资料。有部署资源时返回冲突，可归档。
- 原 /products 列表、资料写入和 lifecycle 接口作为兼容接口保留；新页面使用 project_id 作为项目路由参数，生命周期操作继续使用稳定的 product ID。
- 原 /projects 创建接口同步建立生命周期资料；原项目删除接口也会清理相应资料，并在存储层检查部署资源。
- 发行列表接受 audience 筛选。环境写入接受 kind；省略时创建为待分类，更新时保留现有分类。

项目资料、事项和发行继续按 owner_id 隔离。关联只允许当前用户拥有的项目与组件，读取会重新检查归属，不返回资源连接凭证。项目所有权变更同步转移对应生命周期资料。原部署 API 的权限和资源边界继续适用。

## 升级

GORM 增量增加关系及分类字段，migration.project_workspaces.v1 在事务中执行数据关联和汇总清理，并记录完成标记。唯一、自有、未共用的历史项目可以被采用，歧义关系不迁移部署。旧渠道暂停状态保留，旧发行与环境等待人工分类。

公开资料已从页面移除，public_bio、public_url、public_showcase 仅兼容历史数据，无公开 API 或自动导出。服务资源展示及常驻数据库入口不再出现在总览。
