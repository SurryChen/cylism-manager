## 新增需求

### Requirement: 站点增删改查
系统应当提供 API 端点以创建、查看、更新和删除 NGINX 站点配置，元数据包括域名、端口、SSL 状态、根路径、上游代理和自定义 location。

#### Scenario: 创建新站点
- **WHEN** 用户提交站点，域名为 "example.com"，端口 80，根路径 "/var/www/example"，并指定 server_id
- **THEN** 系统创建站点记录，managed=true，并返回创建的站点

#### Scenario: 同一服务器上拒绝重复域名
- **WHEN** 用户尝试在同一服务器上创建已存在的域名
- **THEN** 系统返回错误，提示域名重复

#### Scenario: 更新站点配置
- **WHEN** 用户更新已有站点的 root_path 或 upstream
- **THEN** 系统更新站点记录，并标记需要重新生成 NGINX 配置

#### Scenario: 删除站点
- **WHEN** 用户删除一个站点
- **THEN** 系统移除站点记录、关联的 NGINX 配置文件（如为 managed）以及关联的证书记录

### Requirement: 站点列表与筛选
系统应当允许用户按服务器筛选站点列表。

#### Scenario: 列出所有站点
- **WHEN** 用户请求站点列表，未指定筛选条件
- **THEN** 系统返回所有站点及其域名、服务器、SSL 状态和证书到期信息

#### Scenario: 按服务器筛选站点
- **WHEN** 用户请求按 server_id 筛选站点
- **THEN** 系统仅返回属于该服务器的站点

### Requirement: 站点详情含证书信息
系统应当返回站点详情，包含关联的证书信息（如有）。

#### Scenario: 查看含有效证书的站点
- **WHEN** 用户请求已签发证书的站点详情
- **THEN** 系统返回站点信息以及证书状态、到期时间和颁发者

#### Scenario: 查看无证书的站点
- **WHEN** 用户请求没有证书的站点详情
- **THEN** 系统返回站点信息，cert_id 为空
