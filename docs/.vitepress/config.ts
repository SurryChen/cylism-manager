import { defineConfig } from 'vitepress'

const repository = process.env.GITHUB_REPOSITORY?.split('/')[1]
const base = process.env.VITEPRESS_BASE || (process.env.GITHUB_ACTIONS && repository ? `/${repository}/` : '/')

export default defineConfig({
  lang: 'zh-CN',
  title: 'Cylism Manager',
  description: '面向 Kubernetes 兼容集群的服务器、应用与运维管理文档',
  base,
  cleanUrls: true,
  srcExclude: [
    'archive/**',
    'design/**',
    'k3s-tailscale-deployment.md',
    'assets/screenshots/README.md',
  ],
  themeConfig: {
    logo: '/assets/cylism-manager-icon.svg',
    siteTitle: 'Cylism Manager',
    nav: [
      { text: '首页', link: '/' },
      { text: '应用交付', link: '/application-delivery/projects-and-environments' },
      { text: '制品与供应链', link: '/supply-chain/managed-registry' },
      { text: '资源与平台', link: '/platform/servers-and-terminal' },
      { text: '可观测与运维', link: '/operations/metrics' },
      { text: '治理与系统', link: '/governance/audit-logs' },
      { text: '自动化助手', link: '/automation/agent-assistant' },
      { text: '开始使用', items: [
        { text: '快速开始', link: '/getting-started' },
        { text: '安装部署', link: '/installation/prerequisites' },
        { text: '配置参考', link: '/operations/configuration' },
        { text: '安全说明', link: '/security' },
        { text: '开发与贡献', link: '/development/contributing' },
      ] },
    ],
    sidebar: {
      '/application-delivery/': [{ text: '应用交付', items: [
        { text: '项目与环境', link: '/application-delivery/projects-and-environments' },
        { text: '应用工作台', link: '/application-delivery/application-workspace' },
        { text: '发布与版本记录', link: '/application-delivery/releases' },
        { text: '域名与应用访问', link: '/application-delivery/domains-and-access' },
      ] }],
      '/supply-chain/': [{ text: '制品与供应链', items: [
        { text: '自托管制品库', link: '/supply-chain/managed-registry' },
        { text: 'Registry Proxy', link: '/supply-chain/registry-proxy' },
        { text: '节点镜像源', link: '/supply-chain/node-registry-mirrors' },
        { text: 'Helm Chart 仓库', link: '/supply-chain/helm-chart-repositories' },
      ] }],
      '/platform/': [{ text: '资源与平台', items: [
        { text: '服务器', link: '/platform/servers-and-terminal' },
        { text: '集群与系统组件', link: '/platform/cluster-and-system-components' },
        { text: 'Kubernetes 资源', link: '/platform/kubernetes-resources' },
        { text: '网络与证书', link: '/platform/network-and-certificates' },
        { text: '存储', link: '/platform/storage' },
      ] }],
      '/operations/': [
        { text: '可观测与运维', items: [
          { text: '指标监控', link: '/operations/metrics' },
          { text: '日志', link: '/operations/logs' },
          { text: '告警', link: '/operations/alerts' },
          { text: '磁盘增长', link: '/operations/disk-growth' },
          { text: '日常维护与故障处理', link: '/operations/maintenance-and-troubleshooting' },
        ] },
        { text: '辅助资料', items: [
          { text: '配置参考', link: '/operations/configuration' },
          { text: '既有日常运维', link: '/operations/operations' },
          { text: '常见问题', link: '/operations/troubleshooting' },
        ] },
      ],
      '/governance/': [{ text: '治理与系统', items: [
        { text: '审计日志', link: '/governance/audit-logs' },
        { text: '操作历史', link: '/governance/operation-history' },
        { text: '系统设置', link: '/governance/system-settings' },
        { text: '数据库管理', link: '/governance/database-management' },
        { text: '身份、权限与安全', link: '/governance/identity-permissions-security' },
      ] }],
      '/automation/': [{ text: '自动化助手', items: [
        { text: 'Agent 助手', link: '/automation/agent-assistant' },
        { text: '自动化操作与执行记录', link: '/automation/automated-operations' },
      ] }],
      '/': [
        {
          text: '产品文档',
          items: [
            { text: '首页', link: '/' },
            { text: '应用交付', link: '/application-delivery/projects-and-environments' },
            { text: '制品与供应链', link: '/supply-chain/managed-registry' },
            { text: '资源与平台', link: '/platform/servers-and-terminal' },
            { text: '可观测与运维', link: '/operations/metrics' },
            { text: '治理与系统', link: '/governance/audit-logs' },
            { text: '自动化助手', link: '/automation/agent-assistant' },
          ],
        },
      ],
    },
    outline: {
      level: [2, 3],
      label: '目录',
    },
    search: {
      provider: 'local',
    },
    darkModeSwitchLabel: '切换深色模式',
    lightModeSwitchTitle: '切换浅色模式',
    darkModeSwitchTitle: '切换深色模式',
    socialLinks: [
      { icon: 'github', link: 'https://github.com/SurryChen/cylism-manager' },
    ],
    editLink: {
      pattern: 'https://github.com/SurryChen/cylism-manager/edit/main/docs/:path',
      text: '在 GitHub 上编辑此页',
    },
    footer: {
      message: 'Cylism Manager · Apache License 2.0',
    },
  },
})
