package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	application "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/cylism/cylism-manager/internal/service/database"
	"github.com/cylism/cylism-manager/internal/service/database/gateway"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type databaseApplicationView struct {
	ApplicationID   uint     `json:"application_id"`
	ApplicationName string   `json:"application_name"`
	TemplateID      uint     `json:"template_id"`
	TemplateName    string   `json:"template_name"`
	DatabaseName    string   `json:"database_name"`
	Username        string   `json:"username"`
	BindingID       uint     `json:"binding_id,omitempty"`
	Sources         []string `json:"sources"`
}

// ListApplications reads configuration metadata without rewriting templates or
// creating bindings. A detected connection is not proof of a live SQL session.
func (h *DatabaseHandler) ListApplications(c *gin.Context) {
	item, ok := h.get(c)
	if !ok {
		return
	}
	var templates []model.ApplicationDeploymentTemplate
	var bindings []model.DatabaseBinding
	var apps []model.Application
	var gateways []model.DatabaseGateway
	for _, query := range []func() error{
		func() error { return h.store.DB().Order("application_id, id").Find(&templates).Error },
		func() error { return h.store.DB().Where("instance_id = ?", item.ID).Find(&bindings).Error },
		func() error { return h.store.DB().Preload("Environment").Find(&apps).Error },
		func() error { return h.store.DB().Where("instance_id = ?", item.ID).Find(&gateways).Error },
	} {
		if query() != nil {
			apiShared.DBError(c, "读取应用连接失败")
			return
		}
	}
	byApp := map[uint]model.Application{}
	byTemplate := map[uint]model.DatabaseBinding{}
	for _, app := range apps {
		byApp[app.ID] = app
	}
	for _, binding := range bindings {
		byTemplate[binding.TemplateID] = binding
	}
	items := []databaseApplicationView{}
	warnings := []string{}
	// Bound cluster reads and reuse keys referenced by multiple templates. A
	// disconnected cluster must not indefinitely block this read-only scan.
	scanCtx, cancelScan := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancelScan()
	mountCache := map[string]string{}
	readMount := func(app model.Application, mount application.FileMountSpec) string {
		cacheKey := fmt.Sprintf("%d/%s/%s/%s/%s", app.Environment.ClusterID, app.Environment.Namespace, mount.SourceType, mount.SourceName, mount.Key)
		if content, exists := mountCache[cacheKey]; exists {
			return content
		}
		content := ""
		defer func() { mountCache[cacheKey] = content }()
		if h.client == nil || app.Environment.Namespace == "" {
			return content
		}
		ctx, cancel := context.WithTimeout(scanCtx, 3*time.Second)
		defer cancel()
		client, err := h.client(ctx, app.Environment.ClusterID)
		if err != nil || client == nil || client.Clientset == nil {
			return content
		}
		if mount.SourceType == application.FileMountSourceSecret {
			value, err := client.Clientset.CoreV1().Secrets(app.Environment.Namespace).Get(ctx, mount.SourceName, metav1.GetOptions{})
			if err == nil {
				content = string(value.Data[mount.Key])
			}
		} else {
			value, err := client.Clientset.CoreV1().ConfigMaps(app.Environment.Namespace).Get(ctx, mount.SourceName, metav1.GetOptions{})
			if err == nil {
				content = value.Data[mount.Key]
			}
		}
		return content
	}
	for _, template := range templates {
		app, exists := byApp[template.ApplicationID]
		if !exists {
			continue
		}
		rows := map[string]int{}
		add := func(databaseName, username, source string, bindingID uint) {
			key := databaseName + "\x00" + username
			if index, exists := rows[key]; exists {
				for _, known := range items[index].Sources {
					if known == source {
						return
					}
				}
				items[index].Sources = append(items[index].Sources, source)
				return
			}
			rows[key] = len(items)
			items = append(items, databaseApplicationView{ApplicationID: app.ID, ApplicationName: app.Name, TemplateID: template.ID, TemplateName: template.Name, DatabaseName: databaseName, Username: username, BindingID: bindingID, Sources: []string{source}})
		}
		if binding, exists := byTemplate[template.ID]; exists {
			add(binding.DatabaseName, binding.Username, "平台绑定", binding.ID)
		}
		var spec application.ReleaseSpec
		if json.Unmarshal([]byte(template.Spec), &spec) != nil {
			warnings = append(warnings, fmt.Sprintf("%s / %s：模板格式无效，未识别", app.Name, template.Name))
			continue
		}
		configValues, secretValues := map[string]string{}, map[string]string{}
		for key, value := range spec.Config {
			if !containsKey(spec.ConfigDisabled, key) {
				configValues[key] = value
			}
		}
		if template.EncryptedSecrets != "" {
			plain, err := security.Decrypt(h.key, template.EncryptedSecrets)
			var secrets map[string]string
			if err != nil || json.Unmarshal([]byte(plain), &secrets) != nil {
				warnings = append(warnings, fmt.Sprintf("%s / %s：Secret 无法读取，识别结果可能不完整", app.Name, template.Name))
			} else {
				for key, value := range secrets {
					if _, configured := spec.Secrets[key]; configured && !containsKey(spec.SecretsDisabled, key) {
						secretValues[key] = value
					}
				}
			}
		}
		match := func(hint database.ConnectionHint) {
			if matchesDatabaseEndpoint(item, gateways, app.Environment.ClusterID, hint) {
				add(hint.Database, hint.Username, hint.Source, 0)
			}
		}
		env := map[string]string{}
		for key, value := range configValues {
			if !containsKey(spec.ConfigFileKeys, key) {
				env[key] = value
			}
		}
		for key, value := range secretValues {
			if !containsKey(spec.SecretFileKeys, key) {
				env[key] = value
			}
		}
		for _, hint := range database.DetectConnections(env, "环境变量 · ") {
			match(hint)
		}
		for _, mount := range spec.FileMounts {
			content := ""
			switch mount.SourceType {
			case application.FileMountSourceApplicationConfig:
				content = configValues[mount.Key]
			case application.FileMountSourceApplicationSecret:
				content = secretValues[mount.Key]
			case application.FileMountSourceConfigMap, application.FileMountSourceSecret:
				// Read only explicitly referenced keys in the app's namespace.
				content = readMount(app, mount)
				if content == "" {
					warnings = append(warnings, fmt.Sprintf("%s / %s：挂载文件 %s 未能读取，识别结果可能不完整", app.Name, template.Name, mount.MountPath))
				}
			}
			for _, hint := range database.DetectFileConnections(content, "文件 · "+mount.MountPath) {
				match(hint)
			}
		}
	}
	apiShared.Success(c, gin.H{"items": items, "warnings": warnings})
}

func containsKey(keys []string, key string) bool {
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}

func matchesDatabaseEndpoint(item *model.DatabaseInstance, gateways []model.DatabaseGateway, clusterID uint, hint database.ConnectionHint) bool {
	host := database.NormalizeDatabaseHost(hint.Host)
	// Service names have cluster scope. Do not conflate identically named
	// Services belonging to different clusters, or different ports on one host.
	match := func(expected string, port int, serviceCluster uint) bool {
		expected = database.NormalizeDatabaseHost(expected)
		if hint.Port != port || (serviceCluster != 0 && clusterID != serviceCluster) {
			return false
		}
		if host == expected {
			return true
		}
		if serviceCluster != 0 && strings.HasSuffix(expected, ".svc.cluster.local") {
			return host == strings.TrimSuffix(expected, ".cluster.local") || host == strings.TrimSuffix(expected, ".svc.cluster.local")
		}
		return false
	}
	scope := uint(0)
	if item.Mode == "managed" {
		scope = item.ClusterID
	}
	if match(item.Host, item.Port, scope) {
		return true
	}
	for _, gw := range gateways {
		if match(gateway.ServiceHost(gw.ID), 3306, gw.ClusterID) || match(gw.ListenIP, gw.ListenPort, 0) {
			return true
		}
	}
	return false
}
