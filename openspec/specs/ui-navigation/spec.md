# ui-navigation Specification

## Purpose
TBD - created by archiving change db-admin. Update Purpose after archive.
## Requirements
### Requirement: 数据管理导航入口
系统 SHALL 在顶部导航栏中新增"数据管理"入口，指向数据库管理页面。

#### Scenario: 导航栏显示数据管理入口
- **WHEN** 用户登录后查看顶部导航
- **THEN** 导航栏中显示"数据管理"链接，点击跳转到 /db-admin 页面

### Requirement: 导航栏入口调整
系统 SHALL 从导航栏移除"导入"入口，站点功能由"站点"页面统一承载。

#### Scenario: 导航栏不再显示导入
- **WHEN** 用户登录后查看导航栏
- **THEN** 导航栏显示：概览、服务器、站点、审计、数据管理，不显示"导入"

