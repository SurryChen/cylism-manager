## ADDED Requirements

### Requirement: Operational page reads retain authoritative data

服务器和工作负载页面的清单、详情、统计及诊断读取 SHALL 使用可取消的页面资源，并且只允许最近一次仍有效的请求更新对应区域。

#### Scenario: A user changes the selected server before statistics resolve

- **WHEN** the user opens statistics for server A and then server B before A's request completes
- **THEN** the page SHALL show server B's statistics when B resolves
- **AND** a late response for server A SHALL NOT replace server B's title or values

#### Scenario: A workload detail request becomes stale

- **WHEN** the user expands workload A and then requests details for workload B before A's detail request completes
- **THEN** the page SHALL associate the resolved detail with its workload key
- **AND** a late response for workload A SHALL NOT overwrite workload B's detail region

#### Scenario: A refresh fails after data was loaded

- **WHEN** a server, workload, diagnostic, or statistics refresh fails after a successful response
- **THEN** the page SHALL retain the last successful data for that resource
- **AND** SHALL expose a retryable error in the resource's own region

### Requirement: Operational page mutations have local retry state

服务器和工作负载页面的变更操作 SHALL expose submitting state only for the active operation, SHALL release it on success or failure, and SHALL preserve the relevant dialog or form when the operation fails.

#### Scenario: A server mutation fails

- **WHEN** saving, deleting, importing, probing, or unbinding a server is rejected
- **THEN** the page SHALL show the failure in the related form, dialog, or action region
- **AND** SHALL keep the relevant user input or confirmation context available for retry

#### Scenario: A workload mutation fails

- **WHEN** scaling, changing an image, or rolling back a workload is rejected
- **THEN** the related dialog SHALL remain open with its current values
- **AND** its submitting state SHALL be cleared without falsely refreshing or reporting success

#### Scenario: A mutation succeeds

- **WHEN** an operational mutation completes successfully
- **THEN** the page SHALL close or update the initiating UI according to the existing workflow
- **AND** SHALL await the affected resource refresh before presenting refreshed values where the UI depends on them

### Requirement: Polling and requests are safe across page lifecycle

服务器资源监控及其他页面级轮询 SHALL prevent overlapping callbacks, stop when the monitored region is inactive or the component is unmounted, and SHALL not update disposed state.

#### Scenario: Monitoring section becomes inactive

- **WHEN** the user leaves the server resource-monitoring section or the document becomes hidden
- **THEN** resource polling SHALL stop
- **AND** no later interval callback SHALL start a new resource request

#### Scenario: A polling request exceeds its interval

- **WHEN** a resource poll is still pending at the next interval
- **THEN** the next interval SHALL skip starting a second request
- **AND** a later interval SHALL remain available after the first request settles

#### Scenario: A page is unmounted during a read

- **WHEN** the user navigates away while a server or workload read is pending
- **THEN** the request SHALL be aborted when possible
- **AND** a later response SHALL NOT update the unmounted page or create a visible error

### Requirement: High-frequency operational pages expose boundary-focused regression coverage

页面测试 SHALL directly mock `api/servers.js` or `api/kubernetes.js` for the corresponding page and SHALL cover authoritative reads, local mutation failures, lifecycle cleanup, and successful refresh behavior.

#### Scenario: A page test exercises a managed read

- **WHEN** a server or workload test starts a managed read
- **THEN** the test SHALL verify the named domain API function receives the expected arguments and an AbortSignal

#### Scenario: A page test exercises a failed mutation

- **WHEN** a mutation mock rejects
- **THEN** the test SHALL verify the local error is visible, submitting state is released, and the relevant dialog or form remains usable

#### Scenario: A page test exercises a stale or disposed request

- **WHEN** deferred responses resolve out of order or after unmount
- **THEN** the test SHALL verify only the current mounted context is updated
- **AND** aborted or obsolete responses SHALL not become user-facing errors
