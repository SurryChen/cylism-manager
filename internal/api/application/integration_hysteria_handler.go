package applicationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

var hysteriaUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type hysteriaUserMutation struct {
	Operation        string `json:"operation"`
	Password         string `json:"password"`
	ExpectedRevision uint   `json:"expected_revision"`
	Restart          bool   `json:"restart"`
}

// hysteriaTemplateConfig reads the encrypted template source, not the rendered
// Kubernetes Secret, so a pending release cannot overwrite an integration edit.
func (h *ApplicationHandler) hysteriaTemplateConfig(app *model.Application) (*model.ApplicationDeploymentTemplate, applicationservice.ReleaseSpec, map[string]any, map[string]any, error) {
	template, err := h.resources.GetDefaultApplicationDeploymentTemplate(app.ID)
	if err != nil {
		return nil, applicationservice.ReleaseSpec{}, nil, nil, err
	}
	if !template.Enabled {
		return nil, applicationservice.ReleaseSpec{}, nil, nil, errors.New("应用没有可用的默认上线模板")
	}
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, spec, nil, nil, fmt.Errorf("读取默认模板: %w", err)
	}
	mounted := false
	for _, mount := range spec.FileMounts {
		if mount.SourceType == applicationservice.FileMountSourceApplicationSecret && mount.Key == "config.yaml" {
			mounted = true
			break
		}
	}
	if !mounted {
		return nil, spec, nil, nil, errors.New("默认模板未挂载应用 Secret 的 config.yaml")
	}
	if !applicationservice.IsSecretKeyManaged(spec, "config.yaml") {
		return nil, spec, nil, nil, errors.New("Secret config.yaml 未启用外部管理")
	}
	for _, key := range spec.SecretsDisabled {
		if key == "config.yaml" {
			return nil, spec, nil, nil, errors.New("默认模板已禁用 Secret config.yaml")
		}
	}
	secrets, err := h.decryptTemplateSecrets(template)
	if err != nil {
		return nil, spec, nil, nil, fmt.Errorf("读取模板 Secret: %w", err)
	}
	content := secrets["config.yaml"]
	if content == "" || len(content) > 4<<20 {
		return nil, spec, nil, nil, errors.New("Secret config.yaml 为空或超过 4 MiB")
	}
	var root map[string]any
	if err := yaml.Unmarshal([]byte(content), &root); err != nil || root == nil {
		return nil, spec, nil, nil, errors.New("Secret config.yaml 格式无效")
	}
	auth, ok := root["auth"].(map[string]any)
	if !ok {
		return nil, spec, nil, nil, errors.New("Hysteria auth 配置不存在")
	}
	users, ok := auth["userpass"].(map[string]any)
	if !ok {
		return nil, spec, nil, nil, errors.New("Hysteria auth.userpass 配置不存在")
	}
	return template, spec, root, users, nil
}

func (h *ApplicationHandler) hysteriaApplication(c *gin.Context, action string) (*model.Application, bool) {
	app, ok := h.integrationApplication(c, action)
	if !ok {
		return nil, false
	}
	if delegationClaims(c).Capability != "hysteria2" {
		integrationForbidden(c)
		return nil, false
	}
	c.Header("Cache-Control", "no-store")
	return app, true
}

func (h *ApplicationHandler) IntegrationHysteriaConfig(c *gin.Context) {
	app, ok := h.hysteriaApplication(c, "hysteria:read")
	if !ok {
		return
	}
	template, _, root, users, err := h.hysteriaTemplateConfig(app)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	names := make([]string, 0, len(users))
	for name := range users {
		if hysteriaUsernamePattern.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	traffic, _ := root["trafficStats"].(map[string]any)
	settings := gin.H{}
	if traffic != nil {
		settings["listen"], _ = traffic["listen"].(string)
		settings["secret"], _ = traffic["secret"].(string)
	}
	apiShared.Success(c, gin.H{"users": names, "revision": template.Revision, "traffic_stats": settings})
}

func (h *ApplicationHandler) IntegrationHysteriaCredential(c *gin.Context) {
	app, ok := h.hysteriaApplication(c, "hysteria:read")
	if !ok {
		return
	}
	username := c.Param("username")
	if !hysteriaUsernamePattern.MatchString(username) {
		apiShared.BadRequest(c, "用户名无效")
		return
	}
	_, _, _, users, err := h.hysteriaTemplateConfig(app)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	password, ok := users[username].(string)
	if !ok || password == "" {
		apiShared.NotFound(c, "用户不存在")
		return
	}
	apiShared.Success(c, gin.H{"password": password})
}

func (h *ApplicationHandler) IntegrationUpdateHysteriaUser(c *gin.Context) {
	app, ok := h.hysteriaApplication(c, "hysteria:write")
	if !ok {
		return
	}
	username := c.Param("username")
	var req hysteriaUserMutation
	if !hysteriaUsernamePattern.MatchString(username) || c.ShouldBindJSON(&req) != nil || req.ExpectedRevision == 0 {
		apiShared.BadRequest(c, "用户变更参数无效")
		return
	}
	if req.Operation != "add" && req.Operation != "replace" && req.Operation != "remove" {
		apiShared.BadRequest(c, "用户变更操作无效")
		return
	}
	if req.Operation != "remove" && (len(req.Password) < 12 || strings.ContainsAny(req.Password, "\r\n\x00")) {
		apiShared.BadRequest(c, "密码至少需要 12 个字符且不能包含换行")
		return
	}
	if req.Restart && !delegationClaims(c).Allows("application:restart") {
		integrationForbidden(c)
		return
	}
	managedFileMutationMu.Lock()
	defer managedFileMutationMu.Unlock()
	template, spec, root, users, err := h.hysteriaTemplateConfig(app)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "应用没有可用的默认上线模板")
		} else {
			apiShared.ValidationError(c, err.Error())
		}
		return
	}
	if template.Revision != req.ExpectedRevision {
		apiShared.Conflict(c, fmt.Sprintf("模板版本冲突，当前版本为 %d", template.Revision))
		return
	}
	_, exists := users[username]
	switch req.Operation {
	case "add":
		if exists {
			apiShared.Conflict(c, "用户已存在")
			return
		}
		users[username] = req.Password
	case "replace":
		if !exists {
			apiShared.NotFound(c, "用户不存在")
			return
		}
		users[username] = req.Password
	case "remove":
		if !exists {
			apiShared.NotFound(c, "用户不存在")
			return
		}
		if len(users) <= 1 {
			apiShared.ValidationError(c, "不能删除最后一个用户")
			return
		}
		delete(users, username)
	}
	content, err := yaml.Marshal(root)
	if err != nil {
		apiShared.DBError(c, "编码 Hysteria 配置失败")
		return
	}
	currentSecrets, err := h.decryptTemplateSecrets(template)
	if err != nil {
		apiShared.ValidationError(c, "读取模板 Secret 失败")
		return
	}
	if spec.Secrets == nil {
		spec.Secrets = make(map[string]string)
	}
	for key := range currentSecrets {
		if _, exists := spec.Secrets[key]; !exists {
			spec.Secrets[key] = ""
		}
	}
	spec.Secrets["config.yaml"] = string(content)
	updated, err := h.templateFromRequest(c.Request.Context(), app, &deploymentTemplateRequest{Name: template.Name, Description: template.Description, Enabled: template.Enabled, DefaultVersion: template.DefaultVersion, Spec: spec}, template.ID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	updated.UpdatedBy = apiShared.UserID(c)
	if err := h.resources.UpdateApplicationDeploymentTemplateIfRevision(updated, req.ExpectedRevision); err != nil {
		var conflict *model.TemplateRevisionConflictError
		if errors.As(err, &conflict) {
			apiShared.Conflict(c, conflict.Error())
		} else {
			apiShared.DBError(c, err.Error())
		}
		return
	}
	response := gin.H{"revision": updated.Revision}
	if req.Restart {
		release, restartErr := h.createRestartRelease(c.Request.Context(), app, apiShared.UserID(c))
		if restartErr != nil {
			response["restart_pending"] = true
			response["restart_error"] = "用户配置已保存，重新发布创建失败"
		} else {
			response["release_id"] = release.ID
		}
	}
	apiShared.Success(c, response)
}
