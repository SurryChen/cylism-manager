# CrazyCoding 个人主页（本地预览）

独立静态页面，不依赖 Cylism Manager 的前后端或 API。无需安装依赖。

```bash
cd /Users/dxm/MyApp/cylism/cylism-manager/previews/crazycoding-site
python3 -m http.server 5188 --bind 127.0.0.1
```

打开 http://127.0.0.1:5188 。

- `index.html`：个人介绍、探索方向和 GitHub 主页链接。
- `style.css`：设计变量、桌面和手机布局。
- `site.js`：`siteConfig.icpNumber` 备案号配置。
- `favicon.svg`：站点图标。

页面未包含具体项目名称、应用域名、内部界面、后台入口或项目数据。后续只按用户明确选择添加公开作品；不通过 CSS 隐藏非公开内容。中文是主要展示语言。

页面包含插画浮动、轨道流动、鼠标轻微倾斜、滚动入场与悬停反馈。插画下方可暂停动画；系统开启“减少动态效果”后自动关闭动画。手机不启用鼠标倾斜，插画离开视口时停止循环动画。

鼠标在首屏移动时，方块、灯泡、完成标记以不同深度跟随，卡片倾斜与光源同步变化；离开后平滑回正。探索方向列表的柔和高光跟随鼠标位置。鼠标互动使用原生事件和 requestAnimationFrame，不依赖第三方动画库。

已根据腾讯云 `crazycoding.top` 网站备案记录填写 `粤ICP备2026170969号-1`，链接至工信部备案系统；HTML 中也保留完整号码，未执行 JavaScript 时仍可显示。后续号码变更时同步更新 `site.js` 与 `index.html`。当前联系方式使用 GitHub 主页，不编造邮箱。

未来可以将这四个静态文件直接部署到 Nginx，绑定 `www.crazycoding.top`，并把主域名跳转到 www；当前未修改线上 DNS 或服务。
