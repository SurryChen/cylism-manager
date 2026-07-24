## ADDED Requirements

### Requirement: NGINX 配置生成
系统 SHALL 根据站点元数据渲染 NGINX 配置模板，通过 Agent 写入远端并执行校验和重载。

#### Scenario: 生成 HTTP 站点配置
- **WHEN** 用户对 ssl_enabled=false 的站点触发"生成配置"
- **THEN** 系统渲染 HTTP server 块模板，通过 Agent 写入 /etc/nginx/conf.d/<domain>.conf，执行 nginx -t 校验，通过后 reload

#### Scenario: 生成 HTTPS 站点配置
- **WHEN** 用户对 ssl_enabled=true 且有证书路径的站点触发"生成配置"
- **THEN** 系统渲染包含 SSL 指令的 server 块 + HTTP 重定向块

#### Scenario: 配置校验失败回报错
- **WHEN** 生成的配置未通过 nginx -t
- **THEN** 系统返回校验错误输出，不执行 reload

### Requirement: NGINX 重载
系统 SHALL 通过 Agent 对指定服务器触发 nginx -s reload。

#### Scenario: 重载成功
- **WHEN** 用户触发 NGINX 重载
- **THEN** Agent 返回 reload 结果

### Requirement: NGINX 导入（含 Docker）
系统 SHALL 通过 Agent 自动探测主机或 Docker 容器中的 NGINX，解析所有 server 块并返回候选站点列表。

#### Scenario: 主机安装 NGINX 导入
- **WHEN** 用户对主机安装 NGINX 的服务器触发导入
- **THEN** Agent 执行 nginx -T，后端解析出所有域名、端口、SSL 状态、根目录、代理目标

#### Scenario: Docker NGINX 导入
- **WHEN** 用户对 Docker 部署 NGINX 的服务器触发导入
- **THEN** Agent 执行 docker ps 找到 nginx 容器，通过 docker exec 执行 nginx -T，后端解析返回

### Requirement: 双 Tab 站点管理
系统 SHALL 在站点页面提供"受管站点"和"导入站点"两个 Tab，顶部支持按服务器筛选。

#### Scenario: 服务器筛选
- **WHEN** 用户选择某个服务器
- **THEN** 受管站点和导入候选站点均只显示该服务器的记录

### Requirement: 接管导入站点
系统 SHALL 支持将导入的候选站点转为 managed=true 的受管站点记录。

#### Scenario: 接管站点
- **WHEN** 用户点击候选站点的"接管"按钮
- **THEN** 系统创建 managed=true 的 Site 记录，站点出现在受管站点 tab
