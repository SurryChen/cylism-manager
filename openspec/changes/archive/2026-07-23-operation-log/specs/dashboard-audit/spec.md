## ADDED Requirements

### Requirement: 操作日志清理任务
系统 SHALL在后台启动定时任务，按配置的保留天数自动清理过期操作日志。

#### Scenario: 启动时注册清理任务
- **WHEN** 系统启动
- **THEN** 系统启动一个后台 goroutine，每小时执行一次操作日志清理

#### Scenario: 清理过期日志
- **WHEN** 清理任务运行且配置 retention_days > 0
- **THEN** 系统删除所有 created_at 早于（当前时间 - retention_days 天）的操作日志记录
