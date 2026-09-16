## Design

主栏固定展示搜索、结果、资源和筛选入口，筛选按钮显示当前高级条件数量。点击后打开 `BaseModal`，编辑状态只在点击应用时提交，取消不会改变已生效条件。

筛选条件通过 chips 回显。日期使用本地日期输入，后端将起始日期解释为当天 00:00（含），结束日期解释为次日 00:00（不含），避免遗漏结束日数据。

动作选项使用后端实际写入的完整动作标识，例如 `application.release`、`registry.mirror.verify` 和 `workload.pod.terminal.start`；保留历史 `create/update/delete/deploy` 值用于兼容旧记录。
