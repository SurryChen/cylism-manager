## Purpose
NGINX 配置的导入、同步和生成管理，负责把站点元数据转换为可校验、可回滚的服务器配置，并跟踪配置部署与重载结果。

## Requirements

### Requirement: 从元数据生成 NGINX 配置
系统 SHALL根据站点元数据生成 NGINX server 块配置文件，并通过 Agent 部署到目标服务器。

#### Scenario: 生成纯 HTTP 配置
- **WHEN** 用户对 ssl_enabled=false 的站点触发生成配置
- **THEN** 系统渲染仅含 HTTP 的 server 块模板，通过 Agent 写入 conf.d/<domain>.conf，执行 nginx -t，校验通过后触发 nginx -s reload

#### Scenario: 生成 HTTPS 配置
- **WHEN** 用户对 ssl_enabled=true 且已签发证书的站点触发生成配置
- **THEN** 系统渲染包含 SSL 指令（ssl_certificate、ssl_certificate_key）指向证书路径的 server 块，以及 HTTP 到 HTTPS 的重定向块

#### Scenario: 配置校验失败
- **WHEN** 生成的配置未通过 nginx -t 校验
- **THEN** 系统返回校验错误输出，不执行 reload

### Requirement: NGINX 配置导入
系统 SHALL解析目标服务器上已有的 NGINX 配置，并将发现的站点导入为非受管记录。

#### Scenario: 导入已有配置
- **WHEN** 用户对某服务器触发配置导入
- **THEN** 系统通过 Agent 调用 nginx -T，解析所有 server{} 块，为数据库中不存在的域名创建 managed=false 的 Site 记录，并返回导入站点摘要

#### Scenario: 导入时跳过已有受管站点
- **WHEN** 发现的域名已作为受管站点存在
- **THEN** 系统跳过该域名，不覆盖已有受管站点记录

### Requirement: 站点接管
系统 SHALL允许用户"接管"导入的（非受管）站点，将其转为受管站点。

#### Scenario: 接管导入站点
- **WHEN** 用户对导入站点设置 managed=true
- **THEN** 系统将站点标记为受管，后续配置重新生成时将覆盖其 NGINX 配置文件

### Requirement: NGINX 重载
系统 SHALL支持在配置变更后对指定服务器触发 NGINX 重载。

#### Scenario: 重载 NGINX
- **WHEN** 用户对某服务器触发 reload
- **THEN** 系统通过 Agent 调用 nginx -s reload，返回成功或错误输出

### Requirement: 删除站点时清理配置文件
系统 SHALL在删除受管站点时移除已生成的 NGINX 配置文件。

#### Scenario: 删除受管站点
- **WHEN** 用户删除 managed=true 的站点
- **THEN** 系统通过 Agent 删除 conf.d/<domain>.conf 文件，执行 nginx -t，校验通过后 reload
