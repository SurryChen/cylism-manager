package k8s

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	AlertingStateNotInstalled = "not_installed"
	AlertingStateInstalling   = "installing"
	AlertingStateReady        = "ready"
	AlertingStateDegraded     = "degraded"

	alertmanagerName        = "cylism-alertmanager"
	vmalertName             = "cylism-vmalert"
	kubeStateMetricsName    = "cylism-kube-state-metrics"
	alertingRulesConfigName = "cylism-alerting-rules"
	alertingConfigName      = "cylism-alerting-config"
	alertmanagerConfigName  = "cylism-alertmanager-config"
	alertingSecretName      = "cylism-alerting-secret"
	alertmanagerImage       = "quay.io/prometheus/alertmanager:v0.28.1"
	vmalertImage            = "victoriametrics/vmalert:v1.115.0"
	kubeStateMetricsImage   = "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.15.0"
)

// AlertingConfig contains the installation node, optional notification channel and managed rules.
// The webhook URL is accepted on write only and is never returned from the API.
type AlertingConfig struct {
	NodeName           string                  `json:"node_name"`
	FeishuWebhookURL   string                  `json:"feishu_webhook_url,omitempty"`
	Email              EmailConfig             `json:"email,omitempty"`
	NotificationPolicy AlertNotificationPolicy `json:"notification_policy,omitempty"`
	Rules              []AlertRuleConfig       `json:"rules,omitempty"`
}

// EmailConfig contains write-only SMTP settings for alert notifications.
type EmailConfig struct {
	Enabled  bool   `json:"enabled"`
	SMTPHost string `json:"smtp_host,omitempty"`
	SMTPPort int    `json:"smtp_port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	TLSMode  string `json:"tls_mode,omitempty"`
}

type AlertRuleConfig struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Severity        string  `json:"severity"`
	Enabled         bool    `json:"enabled"`
	Threshold       float64 `json:"threshold,omitempty"`
	DurationMinutes int     `json:"duration_minutes"`
}

// AlertNotificationPolicy controls Alertmanager aggregation without allowing callers to change label grouping.
type AlertNotificationPolicy struct {
	GroupWaitSeconds      int `json:"group_wait_seconds"`
	GroupIntervalMinutes  int `json:"group_interval_minutes"`
	RepeatIntervalMinutes int `json:"repeat_interval_minutes"`
}

type AlertingStatus struct {
	State                  string                  `json:"state"`
	Message                string                  `json:"message"`
	NodeName               string                  `json:"node_name,omitempty"`
	AlertmanagerReady      int32                   `json:"alertmanager_ready"`
	VMAlertReady           int32                   `json:"vmalert_ready"`
	KubeStateMetricsReady  int32                   `json:"kube_state_metrics_ready"`
	NotificationConfigured bool                    `json:"notification_configured"`
	FeishuConfigured       bool                    `json:"feishu_configured"`
	EmailConfigured        bool                    `json:"email_configured"`
	NotificationPolicy     AlertNotificationPolicy `json:"notification_policy"`
	Rules                  []AlertRuleConfig       `json:"rules,omitempty"`
}

type alertingSettings struct {
	NodeName           string                  `json:"node_name"`
	NotificationPolicy AlertNotificationPolicy `json:"notification_policy"`
	Rules              []AlertRuleConfig       `json:"rules"`
}

func defaultAlertRules() []AlertRuleConfig {
	return []AlertRuleConfig{
		{ID: "node-down", Name: "节点不可达", Severity: "critical", Enabled: true, DurationMinutes: 5},
		{ID: "node-cpu-high", Name: "节点 CPU 过高", Severity: "warning", Enabled: true, Threshold: 85, DurationMinutes: 15},
		{ID: "node-memory-high", Name: "节点内存过高", Severity: "warning", Enabled: true, Threshold: 90, DurationMinutes: 10},
		{ID: "node-disk-high", Name: "根磁盘空间不足", Severity: "warning", Enabled: true, Threshold: 85, DurationMinutes: 10},
		{ID: "pod-restarts", Name: "Pod 频繁重启", Severity: "warning", Enabled: true, Threshold: 3, DurationMinutes: 5},
		{ID: "pod-pending", Name: "Pod 持续 Pending", Severity: "warning", Enabled: true, DurationMinutes: 10},
		{ID: "workload-replicas", Name: "工作负载副本不足", Severity: "critical", Enabled: true, DurationMinutes: 5},
		{ID: "monitoring-target-down", Name: "监控组件不可用", Severity: "critical", Enabled: true, DurationMinutes: 5},
	}
}

func defaultAlertNotificationPolicy() AlertNotificationPolicy {
	return AlertNotificationPolicy{GroupWaitSeconds: 30, GroupIntervalMinutes: 5, RepeatIntervalMinutes: 240}
}

func (c *Client) AlertingStatus() *AlertingStatus {
	status := &AlertingStatus{State: AlertingStateNotInstalled, Message: "尚未启用集群告警", NotificationPolicy: defaultAlertNotificationPolicy(), Rules: defaultAlertRules()}
	if c == nil || c.Clientset == nil {
		status.State = AlertingStateDegraded
		status.Message = "Kubernetes 客户端未初始化"
		return status
	}
	alertmanager, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.ctx, alertmanagerName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return status
	}
	if err != nil {
		status.State = AlertingStateDegraded
		status.Message = fmt.Sprintf("读取 Alertmanager 状态失败: %v", err)
		return status
	}
	status.State = AlertingStateInstalling
	status.Message = deploymentStatusMessage(alertmanager)
	status.NodeName = alertmanager.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]
	status.AlertmanagerReady = alertmanager.Status.AvailableReplicas
	if deployment, getErr := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.ctx, vmalertName, metav1.GetOptions{}); getErr == nil {
		status.VMAlertReady = deployment.Status.AvailableReplicas
	}
	if deployment, getErr := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.ctx, kubeStateMetricsName, metav1.GetOptions{}); getErr == nil {
		status.KubeStateMetricsReady = deployment.Status.AvailableReplicas
	}
	if settings, getErr := c.alertingSettings(); getErr == nil {
		status.NotificationPolicy = settings.NotificationPolicy
		status.Rules = settings.Rules
	}
	if secret, getErr := c.Clientset.CoreV1().Secrets(victoriaMetricsNamespace).Get(c.ctx, alertingSecretName, metav1.GetOptions{}); getErr == nil {
		status.FeishuConfigured = strings.TrimSpace(string(secret.Data["feishu-webhook-url"])) != ""
		status.EmailConfigured = strings.TrimSpace(string(secret.Data["email-to"])) != ""
		status.NotificationConfigured = status.FeishuConfigured || status.EmailConfigured
	}
	if status.AlertmanagerReady > 0 && status.VMAlertReady > 0 && status.KubeStateMetricsReady > 0 {
		status.State = AlertingStateReady
		status.Message = "告警规则正在评估，通知通道已就绪"
	}
	return status
}

func (c *Client) AlertingStatusContext(ctx context.Context) *AlertingStatus {
	return c.withContext(ctx).AlertingStatus()
}

func (c *Client) InstallAlertingContext(ctx context.Context, config AlertingConfig) (*AlertingStatus, error) {
	return c.withContext(ctx).installAlerting(config)
}

func (c *Client) UpdateAlertingContext(ctx context.Context, config AlertingConfig) (*AlertingStatus, error) {
	return c.withContext(ctx).updateAlerting(config)
}

func (c *Client) UninstallAlertingContext(ctx context.Context) error {
	return c.withContext(ctx).uninstallAlerting()
}

// InstallAlerting creates or updates all platform-owned alerting resources.
func (c *Client) installAlerting(config AlertingConfig) (*AlertingStatus, error) {
	if c == nil || c.Clientset == nil {
		return c.AlertingStatus(), fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if c.VictoriaMetricsStatus().ReadyReplicas == 0 {
		return c.AlertingStatus(), fmt.Errorf("VictoriaMetrics 存储实例尚未就绪")
	}
	settings, err := normalizeAlertingConfig(config)
	if err != nil {
		return c.AlertingStatus(), err
	}
	node, err := c.Clientset.CoreV1().Nodes().Get(c.ctx, settings.NodeName, metav1.GetOptions{})
	if err != nil {
		return c.AlertingStatus(), fmt.Errorf("读取告警节点失败: %w", err)
	}
	if !nodeReady(node) {
		return c.AlertingStatus(), fmt.Errorf("告警节点 %s 未就绪", settings.NodeName)
	}
	if existing, getErr := c.alertingSettings(); getErr == nil {
		if existing.NodeName != "" && existing.NodeName != settings.NodeName {
			return c.AlertingStatus(), fmt.Errorf("已安装告警的节点不能直接修改；Alertmanager PVC 仍绑定在 %s", existing.NodeName)
		}
	}
	if err := ensureMonitoringNamespace(c); err != nil {
		return c.AlertingStatus(), err
	}
	if err := ensureKubeStateMetricsAccess(c); err != nil {
		return c.AlertingStatus(), err
	}
	if err := c.upsertAlertingSecret(config.FeishuWebhookURL, config.Email); err != nil {
		return c.AlertingStatus(), err
	}
	if err := c.upsertAlertingSettings(settings); err != nil {
		return c.AlertingStatus(), err
	}
	if err := c.upsertAlertingResources(settings); err != nil {
		return c.AlertingStatus(), err
	}
	if err := refreshVictoriaMetricsScrapeConfig(c); err != nil {
		return c.AlertingStatus(), err
	}
	return c.AlertingStatus(), nil
}

func (c *Client) updateAlerting(config AlertingConfig) (*AlertingStatus, error) {
	existing, err := c.alertingSettings()
	if err != nil {
		return c.AlertingStatus(), fmt.Errorf("告警尚未安装")
	}
	config.NodeName = existing.NodeName
	return c.installAlerting(config)
}

// UninstallAlerting removes workloads and generated configs. The PVC and Secret
// are intentionally retained so silences and notification settings survive a rollback.
func (c *Client) uninstallAlerting() error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	for _, remove := range []func() error{
		func() error {
			return c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Delete(c.ctx, vmalertName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Delete(c.ctx, alertmanagerName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Delete(c.ctx, kubeStateMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Delete(c.ctx, alertmanagerName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().Services(victoriaMetricsNamespace).Delete(c.ctx, kubeStateMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Delete(c.ctx, alertingRulesConfigName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Delete(c.ctx, alertmanagerConfigName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.CoreV1().ServiceAccounts(victoriaMetricsNamespace).Delete(c.ctx, kubeStateMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.RbacV1().ClusterRoleBindings().Delete(c.ctx, kubeStateMetricsName, metav1.DeleteOptions{})
		},
		func() error {
			return c.Clientset.RbacV1().ClusterRoles().Delete(c.ctx, kubeStateMetricsName, metav1.DeleteOptions{})
		},
	} {
		if err := remove(); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("卸载告警组件失败: %w", err)
		}
	}
	if err := refreshVictoriaMetricsScrapeConfig(c); err != nil {
		return err
	}
	return nil
}

// refreshVictoriaMetricsScrapeConfig rolls the metrics pod when the optional
// kube-state-metrics target changes so the mounted configuration takes effect.
func refreshVictoriaMetricsScrapeConfig(c *Client) error {
	if err := upsertVictoriaMetricsConfig(c, VictoriaMetricsConfig{}); err != nil {
		return err
	}
	deployment, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Get(c.ctx, victoriaMetricsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 VictoriaMetrics Deployment 失败: %w", err)
	}
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["cylism.io/scrape-config-hash"] = victoriaMetricsScrapeConfigHash(kubeStateMetricsInstalled(c))
	if _, err := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace).Update(c.ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("同步 VictoriaMetrics 采集配置失败: %w", err)
	}
	return nil
}

func normalizeAlertingConfig(config AlertingConfig) (alertingSettings, error) {
	notificationPolicy, err := normalizeAlertNotificationPolicy(config.NotificationPolicy)
	if err != nil {
		return alertingSettings{}, err
	}
	settings := alertingSettings{NodeName: strings.TrimSpace(config.NodeName), NotificationPolicy: notificationPolicy, Rules: mergeAlertRules(config.Rules)}
	if settings.NodeName == "" {
		return settings, fmt.Errorf("请选择告警节点")
	}
	if raw := strings.TrimSpace(config.FeishuWebhookURL); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "open.feishu.cn" {
			return settings, fmt.Errorf("飞书机器人地址必须使用 open.feishu.cn 的 HTTPS URL")
		}
	}
	config.Email.SMTPHost = strings.TrimSpace(config.Email.SMTPHost)
	config.Email.Username = strings.TrimSpace(config.Email.Username)
	config.Email.From = strings.TrimSpace(config.Email.From)
	config.Email.To = strings.TrimSpace(config.Email.To)
	config.Email.TLSMode = strings.ToLower(strings.TrimSpace(config.Email.TLSMode))
	if config.Email.Enabled {
		if config.Email.SMTPHost == "" || strings.Contains(config.Email.SMTPHost, "://") || config.Email.SMTPPort < 1 || config.Email.SMTPPort > 65535 || config.Email.From == "" || config.Email.To == "" {
			return settings, fmt.Errorf("启用邮件通知时必须填写 SMTP 主机、端口、发件人与收件人")
		}
		if (config.Email.Username == "") != (strings.TrimSpace(config.Email.Password) == "") {
			return settings, fmt.Errorf("SMTP 用户名和密码必须同时填写")
		}
		if config.Email.TLSMode != "starttls" && config.Email.TLSMode != "tls" {
			return settings, fmt.Errorf("SMTP TLS 模式必须为 starttls 或 tls")
		}
	}
	for _, rule := range settings.Rules {
		if rule.DurationMinutes < 1 || rule.DurationMinutes > 1440 {
			return settings, fmt.Errorf("规则 %s 的持续时间应在 1 到 1440 分钟之间", rule.Name)
		}
		if alertRuleSupportsThreshold(rule.ID) && (rule.Threshold <= 0 || rule.Threshold > 100000) {
			return settings, fmt.Errorf("规则 %s 的阈值必须大于 0 且不超过 100000", rule.Name)
		}
	}
	return settings, nil
}

func normalizeAlertNotificationPolicy(input AlertNotificationPolicy) (AlertNotificationPolicy, error) {
	policy := defaultAlertNotificationPolicy()
	if input.GroupWaitSeconds != 0 {
		policy.GroupWaitSeconds = input.GroupWaitSeconds
	}
	if input.GroupIntervalMinutes != 0 {
		policy.GroupIntervalMinutes = input.GroupIntervalMinutes
	}
	if input.RepeatIntervalMinutes != 0 {
		policy.RepeatIntervalMinutes = input.RepeatIntervalMinutes
	}
	if policy.GroupWaitSeconds < 5 || policy.GroupWaitSeconds > 3600 {
		return policy, fmt.Errorf("首次通知等待时间应在 5 到 3600 秒之间")
	}
	if policy.GroupIntervalMinutes < 1 || policy.GroupIntervalMinutes > 1440 {
		return policy, fmt.Errorf("同组新增告警间隔应在 1 到 1440 分钟之间")
	}
	if policy.RepeatIntervalMinutes < 5 || policy.RepeatIntervalMinutes > 10080 {
		return policy, fmt.Errorf("重复提醒间隔应在 5 到 10080 分钟之间")
	}
	return policy, nil
}

func mergeAlertRules(input []AlertRuleConfig) []AlertRuleConfig {
	defaults := defaultAlertRules()
	provided := make(map[string]AlertRuleConfig, len(input))
	for _, rule := range input {
		provided[rule.ID] = rule
	}
	for index, rule := range defaults {
		if override, ok := provided[rule.ID]; ok {
			rule.Enabled = override.Enabled
			if alertRuleSupportsThreshold(rule.ID) {
				rule.Threshold = override.Threshold
			}
			if override.DurationMinutes > 0 {
				rule.DurationMinutes = override.DurationMinutes
			}
		}
		defaults[index] = rule
	}
	return defaults
}

func alertRuleSupportsThreshold(id string) bool {
	switch id {
	case "node-cpu-high", "node-memory-high", "node-disk-high", "pod-restarts":
		return true
	default:
		return false
	}
}

func (c *Client) alertingSettings() (alertingSettings, error) {
	configMap, err := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace).Get(c.ctx, alertingConfigName, metav1.GetOptions{})
	if err != nil {
		return alertingSettings{}, err
	}
	settings := alertingSettings{}
	if err := json.Unmarshal([]byte(configMap.Data["settings.json"]), &settings); err != nil {
		return settings, fmt.Errorf("解析告警配置失败: %w", err)
	}
	notificationPolicy, err := normalizeAlertNotificationPolicy(settings.NotificationPolicy)
	if err != nil {
		return settings, err
	}
	settings.NotificationPolicy = notificationPolicy
	settings.Rules = mergeAlertRules(settings.Rules)
	return settings, nil
}

func (c *Client) upsertAlertingSecret(webhookURL string, email EmailConfig) error {
	secretClient := c.Clientset.CoreV1().Secrets(victoriaMetricsNamespace)
	current, err := secretClient.Get(c.ctx, alertingSecretName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("读取告警通知 Secret 失败: %w", err)
	}
	exists := err == nil
	data := map[string][]byte{}
	if exists {
		for key, value := range current.Data {
			data[key] = value
		}
	}
	if strings.TrimSpace(webhookURL) != "" {
		data["feishu-webhook-url"] = []byte(strings.TrimSpace(webhookURL))
	}
	if email.Enabled {
		for _, key := range []string{"email-smtp-host", "email-smtp-port", "email-username", "email-password", "email-from", "email-to", "email-tls-mode"} {
			delete(data, key)
		}
		data["email-smtp-host"] = []byte(strings.TrimSpace(email.SMTPHost))
		data["email-smtp-port"] = []byte(strconv.Itoa(email.SMTPPort))
		data["email-username"] = []byte(strings.TrimSpace(email.Username))
		data["email-password"] = []byte(email.Password)
		data["email-from"] = []byte(strings.TrimSpace(email.From))
		data["email-to"] = []byte(strings.TrimSpace(email.To))
		data["email-tls-mode"] = []byte(strings.ToLower(strings.TrimSpace(email.TLSMode)))
	}
	if len(data["relay-token"]) < 24 {
		token := make([]byte, 32)
		if _, err := rand.Read(token); err != nil {
			return fmt.Errorf("生成告警回调令牌失败: %w", err)
		}
		data["relay-token"] = []byte(base64.RawURLEncoding.EncodeToString(token))
	}
	resource := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: alertingSecretName, Namespace: victoriaMetricsNamespace, Labels: alertingLabels("alerting")}, Type: corev1.SecretTypeOpaque, Data: data}
	if exists {
		resource.ResourceVersion = current.ResourceVersion
		_, err = secretClient.Update(c.ctx, resource, metav1.UpdateOptions{})
	} else {
		_, err = secretClient.Create(c.ctx, resource, metav1.CreateOptions{})
	}
	if err != nil {
		return fmt.Errorf("保存告警通知 Secret 失败: %w", err)
	}
	return nil
}

func (c *Client) upsertAlertingSettings(settings alertingSettings) error {
	payload, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("编码告警设置失败: %w", err)
	}
	return upsertNamedConfigMap(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: alertingConfigName, Namespace: victoriaMetricsNamespace, Labels: alertingLabels("alerting")}, Data: map[string]string{"settings.json": string(payload)}})
}

func (c *Client) upsertAlertingResources(settings alertingSettings) error {
	if err := upsertNamedConfigMap(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: alertingRulesConfigName, Namespace: victoriaMetricsNamespace, Labels: alertingLabels("vmalert")}, Data: map[string]string{"alerts.yml": renderAlertRules(settings.Rules)}}); err != nil {
		return err
	}
	if err := upsertNamedConfigMap(c, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: alertmanagerConfigName, Namespace: victoriaMetricsNamespace, Labels: alertingLabels("alertmanager")}, Data: map[string]string{"alertmanager.yml": renderAlertmanagerConfig(settings.NotificationPolicy)}}); err != nil {
		return err
	}
	if err := upsertAlertmanagerPVC(c); err != nil {
		return err
	}
	if err := upsertAlertingService(c, alertmanagerName, 9093, alertmanagerLabels()); err != nil {
		return err
	}
	if err := upsertAlertingService(c, kubeStateMetricsName, 8080, kubeStateMetricsLabels()); err != nil {
		return err
	}
	if err := upsertKubeStateMetricsDeployment(c); err != nil {
		return err
	}
	if err := upsertAlertmanagerDeployment(c, settings.NodeName); err != nil {
		return err
	}
	return upsertVMAlertDeployment(c, settings.Rules)
}

func ensureKubeStateMetricsAccess(c *Client) error {
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: kubeStateMetricsName, Namespace: victoriaMetricsNamespace, Labels: kubeStateMetricsLabels()}}
	if _, err := c.Clientset.CoreV1().ServiceAccounts(victoriaMetricsNamespace).Create(c.ctx, serviceAccount, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 kube-state-metrics 服务账号失败: %w", err)
	}
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: kubeStateMetricsName, Labels: kubeStateMetricsLabels()}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"nodes", "pods"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets", "replicasets"}, Verbs: []string{"get", "list", "watch"}},
	}}
	if err := createOrUpdateClusterRole(c, role); err != nil {
		return err
	}
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: kubeStateMetricsName, Labels: kubeStateMetricsLabels()}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: kubeStateMetricsName, Namespace: victoriaMetricsNamespace}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: kubeStateMetricsName}}
	return createOrUpdateClusterRoleBinding(c, binding)
}

func upsertAlertmanagerPVC(c *Client) error {
	claims := c.Clientset.CoreV1().PersistentVolumeClaims(victoriaMetricsNamespace)
	existing, err := claims.Get(c.ctx, alertmanagerName+"-data", metav1.GetOptions{})
	if err == nil {
		labels := existing.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		changed := labels[ManagedByLabel] != ManagedByValue || labels[InfrastructureLabel] != InfrastructureAlertmanager
		labels[ManagedByLabel] = ManagedByValue
		labels[InfrastructureLabel] = InfrastructureAlertmanager
		if !changed {
			return nil
		}
		existing.Labels = labels
		if _, err := claims.Update(c.ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("更新 Alertmanager 存储卷归属失败: %w", err)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("读取 Alertmanager 存储卷失败: %w", err)
	}
	storage := resource.MustParse("1Gi")
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: alertmanagerName + "-data", Namespace: victoriaMetricsNamespace, Labels: infrastructurePVCLabels(InfrastructureAlertmanager)}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: storage}}}}
	if _, err := claims.Create(c.ctx, claim, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("创建 Alertmanager 存储卷失败: %w", err)
	}
	return nil
}

func upsertAlertingService(c *Client, name string, port int32, selector map[string]string) error {
	services := c.Clientset.CoreV1().Services(victoriaMetricsNamespace)
	current, err := services.Get(c.ctx, name, metav1.GetOptions{})
	resource := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: victoriaMetricsNamespace, Labels: selector}, Spec: corev1.ServiceSpec{Selector: selector, Ports: []corev1.ServicePort{{Name: "http", Port: port, TargetPort: intstr.FromInt32(port)}}}}
	if apierrors.IsNotFound(err) {
		_, err = services.Create(c.ctx, resource, metav1.CreateOptions{})
	} else if err == nil {
		resource.ResourceVersion = current.ResourceVersion
		resource.Spec.ClusterIP = current.Spec.ClusterIP
		resource.Spec.ClusterIPs = current.Spec.ClusterIPs
		resource.Spec.IPFamilies = current.Spec.IPFamilies
		resource.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		_, err = services.Update(c.ctx, resource, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建告警 Service %s 失败: %w", name, err)
	}
	return nil
}

func upsertKubeStateMetricsDeployment(c *Client) error {
	replicas := int32(1)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: kubeStateMetricsName, Namespace: victoriaMetricsNamespace, Labels: kubeStateMetricsLabels()}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: kubeStateMetricsLabels()}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: kubeStateMetricsLabels()}, Spec: corev1.PodSpec{ServiceAccountName: kubeStateMetricsName, Containers: []corev1.Container{{Name: "kube-state-metrics", Image: kubeStateMetricsImage, Args: []string{"--resources=nodes,pods,deployments,statefulsets,daemonsets"}, Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}, Resources: alertingResources("50m", "64Mi"), ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(8080)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}}}}}}}
	return upsertAlertingDeployment(c, deployment)
}

func upsertAlertmanagerDeployment(c *Client, nodeName string) error {
	replicas := int32(1)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: alertmanagerName, Namespace: victoriaMetricsNamespace, Labels: alertmanagerLabels()}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: alertmanagerLabels()}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: alertmanagerLabels()}, Spec: corev1.PodSpec{NodeSelector: map[string]string{corev1.LabelHostname: nodeName}, Volumes: []corev1.Volume{
		{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: alertmanagerName + "-data"}}},
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: alertmanagerConfigName}}}},
		{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: alertingSecretName}}},
	}, Containers: []corev1.Container{{Name: "alertmanager", Image: alertmanagerImage, Args: []string{"--config.file=/etc/alertmanager/alertmanager.yml", "--storage.path=/alertmanager", "--web.listen-address=:9093"}, Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 9093}}, Resources: alertingResources("100m", "128Mi"), VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/alertmanager"}, {Name: "config", MountPath: "/etc/alertmanager", ReadOnly: true}, {Name: "secret", MountPath: "/etc/alertmanager/secrets", ReadOnly: true}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/-/ready", Port: intstr.FromInt(9093)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}, LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/-/healthy", Port: intstr.FromInt(9093)}}, InitialDelaySeconds: 15, PeriodSeconds: 15}}}}}}}
	return upsertAlertingDeployment(c, deployment)
}

func upsertVMAlertDeployment(c *Client, rules []AlertRuleConfig) error {
	replicas := int32(1)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: vmalertName, Namespace: victoriaMetricsNamespace, Labels: vmalertLabels()}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: vmalertLabels()}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: vmalertLabels(), Annotations: map[string]string{"cylism.io/rules-config-hash": alertRulesHash(rules)}}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "rules", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: alertingRulesConfigName}}}}}, Containers: []corev1.Container{{Name: "vmalert", Image: vmalertImage, Args: []string{"-datasource.url=" + VictoriaMetricsServiceURL(), "-notifier.url=" + alertmanagerServiceURL(), "-rule=/etc/vmalert/alerts.yml", "-evaluationInterval=1m"}, Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8880}}, Resources: alertingResources("100m", "128Mi"), VolumeMounts: []corev1.VolumeMount{{Name: "rules", MountPath: "/etc/vmalert", ReadOnly: true}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8880)}}, InitialDelaySeconds: 5, PeriodSeconds: 10}}}}}}}
	return upsertAlertingDeployment(c, deployment)
}

func alertingResources(cpu, memory string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(memory)}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(memory)}}
}

func upsertAlertingDeployment(c *Client, deployment *appsv1.Deployment) error {
	deployments := c.Clientset.AppsV1().Deployments(victoriaMetricsNamespace)
	current, err := deployments.Get(c.ctx, deployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = deployments.Create(c.ctx, deployment, metav1.CreateOptions{})
	} else if err == nil {
		deployment.ResourceVersion = current.ResourceVersion
		_, err = deployments.Update(c.ctx, deployment, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("创建告警 Deployment %s 失败: %w", deployment.Name, err)
	}
	return nil
}

func upsertNamedConfigMap(c *Client, configMap *corev1.ConfigMap) error {
	configMaps := c.Clientset.CoreV1().ConfigMaps(victoriaMetricsNamespace)
	current, err := configMaps.Get(c.ctx, configMap.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = configMaps.Create(c.ctx, configMap, metav1.CreateOptions{})
	} else if err == nil {
		configMap.ResourceVersion = current.ResourceVersion
		_, err = configMaps.Update(c.ctx, configMap, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("保存告警配置 %s 失败: %w", configMap.Name, err)
	}
	return nil
}

func alertingLabels(component string) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/component": component}
}

func alertmanagerLabels() map[string]string     { return alertingLabels("alertmanager") }
func vmalertLabels() map[string]string          { return alertingLabels("vmalert") }
func kubeStateMetricsLabels() map[string]string { return alertingLabels("kube-state-metrics") }

func alertmanagerServiceURL() string {
	return "http://" + alertmanagerName + "." + victoriaMetricsNamespace + ".svc:9093"
}

// AlertmanagerServiceURL returns the in-cluster Alertmanager endpoint used by the platform API.
func AlertmanagerServiceURL() string {
	return alertmanagerServiceURL()
}

func renderAlertmanagerConfig(policy AlertNotificationPolicy) string {
	return fmt.Sprintf(`global:
  resolve_timeout: 5m
route:
  receiver: cylism-feishu
  group_by: [alertname, node, namespace, pod, deployment, statefulset]
  group_wait: %ds
  group_interval: %dm
  repeat_interval: %dm
receivers:
  - name: cylism-feishu
    webhook_configs:
      - url: http://cylism-manager.default.svc:8080/api/monitoring/alerts/notify
        send_resolved: true
        http_config:
          authorization:
            type: Bearer
            credentials_file: /etc/alertmanager/secrets/relay-token
`, policy.GroupWaitSeconds, policy.GroupIntervalMinutes, policy.RepeatIntervalMinutes)
}

func renderAlertRules(rules []AlertRuleConfig) string {
	byID := make(map[string]AlertRuleConfig, len(rules))
	for _, rule := range rules {
		byID[rule.ID] = rule
	}
	var lines []string
	appendRule := func(id, alert, expr, summary, threshold, valueUnit string) {
		rule := byID[id]
		if !rule.Enabled {
			return
		}
		annotations := fmt.Sprintf("        summary: %s\n        description: %s\n        rule_name: %s\n        duration: %s\n        current_value: %s", quoteYAML(summary), quoteYAML(summary), quoteYAML(rule.Name), quoteYAML(fmt.Sprintf("%d 分钟", rule.DurationMinutes)), quoteYAML("{{ $value }}"+valueUnit))
		if threshold != "" {
			annotations += "\n        threshold: " + quoteYAML(threshold)
		}
		lines = append(lines, fmt.Sprintf("    - alert: %s\n      expr: %s\n      for: %dm\n      labels:\n        severity: %s\n      annotations:\n%s", alert, expr, rule.DurationMinutes, rule.Severity, annotations))
	}
	appendRule("node-down", "NodeDown", `up{job="node-exporter"} == 0`, "节点 {{ $labels.node }} 的 node-exporter 不可达", "不可达", "")
	appendRule("node-cpu-high", "NodeCPUHigh", fmt.Sprintf(`100 - (avg by (node) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > %.2f`, byID["node-cpu-high"].Threshold), "节点 {{ $labels.node }} CPU 使用率过高", fmt.Sprintf("%.2f%%", byID["node-cpu-high"].Threshold), "%")
	appendRule("node-memory-high", "NodeMemoryHigh", fmt.Sprintf(`100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) > %.2f`, byID["node-memory-high"].Threshold), "节点 {{ $labels.node }} 内存使用率过高", fmt.Sprintf("%.2f%%", byID["node-memory-high"].Threshold), "%")
	appendRule("node-disk-high", "NodeDiskHigh", fmt.Sprintf(`max by (node) (100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"})) > %.2f`, byID["node-disk-high"].Threshold), "节点 {{ $labels.node }} 根磁盘空间不足", fmt.Sprintf("%.2f%%", byID["node-disk-high"].Threshold), "%")
	appendRule("pod-restarts", "PodFrequentRestarts", fmt.Sprintf(`increase(kube_pod_container_status_restarts_total[10m]) > %.2f`, byID["pod-restarts"].Threshold), "Pod {{ $labels.namespace }}/{{ $labels.pod }} 正在频繁重启", fmt.Sprintf("%.0f 次/10分钟", byID["pod-restarts"].Threshold), " 次/10分钟")
	appendRule("pod-pending", "PodPending", `kube_pod_status_phase{phase="Pending"} == 1`, "Pod {{ $labels.namespace }}/{{ $labels.pod }} 持续处于 Pending", "Pending", "")
	appendRule("workload-replicas", "WorkloadReplicasUnavailable", `(kube_deployment_spec_replicas > kube_deployment_status_replicas_available) or (kube_statefulset_replicas > kube_statefulset_status_replicas_ready)`, "工作负载 {{ $labels.namespace }} 可用副本不足", "期望副本数", "")
	appendRule("monitoring-target-down", "MonitoringTargetDown", `up{job=~"kubernetes-nodes|kubernetes-cadvisor|kube-state-metrics"} == 0`, "监控采集目标 {{ $labels.job }} 不可用", "可访问", "")
	if len(lines) == 0 {
		lines = append(lines, "    - alert: AlertingRulesDisabled\n      expr: vector(0) > 1\n      for: 1m\n      labels:\n        severity: warning\n      annotations:\n        summary: \"全部告警规则已禁用\"")
	}
	return "groups:\n  - name: cylism-managed-alerts\n    interval: 1m\n    rules:\n" + strings.Join(lines, "\n") + "\n"
}

func quoteYAML(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func alertRulesHash(rules []AlertRuleConfig) string {
	checksum := sha256.Sum256([]byte(renderAlertRules(rules)))
	return fmt.Sprintf("%x", checksum[:])
}
