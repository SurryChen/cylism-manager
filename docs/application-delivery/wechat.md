# 微信发行与运营数据

项目有「微信小程序」或「微信小游戏」发行渠道时，打开「发行 → 微信接入」，选择渠道并保存对应的 AppID。已有渠道填写了应用标识时，AppID 需与应用标识一致。

## 自动记录发行

1. 平台管理员在微信开放平台配置并启用第三方平台。申请版本管理权限（权限集 18），将授权事件接收 URL 配为 `https://<平台域名>/api/wechat/component/events`，授权回调域名设为平台域名。扫码回调通过 `https://<平台域名>/api/wechat/component/authorized` 返回。
2. 将平台的 `PUBLIC_URL` 设为对应的公开 HTTPS 地址。通过环境变量提供下表四项配置。Kubernetes 清单从 `cylism-secret` 读取，Helm 部署从 `secrets.existingSecret` 指定的 Secret 读取。更新 Secret 后重启平台 Pod。

   | 环境变量 | Kubernetes Secret 键 |
   | --- | --- |
   | `WECHAT_COMPONENT_APP_ID` | `wechat-component-app-id` |
   | `WECHAT_COMPONENT_APP_SECRET` | `wechat-component-app-secret` |
   | `WECHAT_EVENT_TOKEN` | `wechat-event-token` |
   | `WECHAT_ENCODING_AES_KEY` | `wechat-encoding-aes-key` |

3. 微信开始推送授权事件票据后，在项目中选择该渠道并点击「扫码授权」。用该小程序或小游戏的管理员账号扫码，授予版本管理权限。返回项目时平台会立即同步，也可以点「立即同步」。

微信后台发布成功后，平台约每 5 分钟检查一次当前线上版本。识别到带有效发布时间的线上版本后，会按渠道配置的发布端建立或复用项目版本，并在「发行」登记正式记录和微信实际发布时间。相同渠道、相同渠道版本的已有正式记录会复用。正式草稿可以转为已发布，测试发行按原状态保留。

微信版本查询只返回当前线上版本。间隔内多次发布时，平台能登记它实际读到的版本。体验版和最新审核状态可在「微信接入」查看。提审和发布继续在微信后台或现有发布流程中操作。授权前先配置好渠道归属的发布端。

## 运营数据与推广码

在同一窗口保存小程序或小游戏自身的 AppSecret。项目「总览」会显示访问、活跃用户、新增用户的 7 天和 30 天趋势。小游戏还可以查看分享、次日留存与广告分成后收入，取决于微信是否返回对应指标。AppSecret 在服务端加密保存，保存时留空会沿用已配置的值。

图表使用微信按日汇总数据，展示数据截至日期和最近查询时间。打开总览或点击刷新可以查询微信已经产出的数据，后台每 6 小时补查近 7 天。微信尚未出数的日期在图上留空。30 天历史首次查询可能分多次补齐。

「微信接入」也可按场景值生成并下载线上推广码。断开连接会删除平台本地授权凭据和运营缓存，保留已经写入的项目版本与发行记录。若要同时解除微信端的第三方授权，可在微信后台操作。

[微信开放平台授权流程](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/Before_Develop/Authorization_Process_Technical_Description.html) · [微信小游戏数据分析](https://developers.weixin.qq.com/minigame/dev/guide/open-ability/data-analysis)
