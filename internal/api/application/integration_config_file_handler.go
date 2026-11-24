package applicationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// File access is schema-agnostic and scoped to explicitly managed mounts.
type integrationConfigFile struct {
	Source           string `json:"source"`
	Key              string `json:"key"`
	MountPath        string `json:"mount_path"`
	Format           string `json:"format"`
	TemplateRevision uint   `json:"template_revision"`
}

func (h *ApplicationHandler) integrationFileTemplate(app *model.Application) (*model.ApplicationDeploymentTemplate, applicationservice.ReleaseSpec, error) {
	template, err := h.resources.GetDefaultApplicationDeploymentTemplate(app.ID)
	var spec applicationservice.ReleaseSpec
	if err != nil {
		return nil, spec, err
	}
	if !template.Enabled {
		return nil, spec, errors.New("应用没有可用的默认上线模板")
	}
	if json.Unmarshal([]byte(template.Spec), &spec) != nil {
		return nil, spec, errors.New("读取上线模板失败")
	}
	return template, spec, nil
}

func integrationConfigFiles(spec applicationservice.ReleaseSpec, revision uint) []integrationConfigFile {
	files := []integrationConfigFile{}
	seen := map[string]bool{}
	for _, mount := range spec.FileMounts {
		source := ""
		switch mount.SourceType {
		case applicationservice.FileMountSourceApplicationConfig:
			if configMapKeyEnabled(spec, mount.Key) && applicationservice.IsConfigKeyManaged(spec, mount.Key) {
				source = "configmap"
			}
		case applicationservice.FileMountSourceApplicationSecret:
			_, exists := spec.Secrets[mount.Key]
			if exists && !containsString(spec.SecretsDisabled, mount.Key) && applicationservice.IsSecretKeyManaged(spec, mount.Key) {
				source = "secret"
			}
		}
		if source == "" || seen[source+"/"+mount.Key] {
			continue
		}
		seen[source+"/"+mount.Key] = true
		files = append(files, integrationConfigFile{Source: source, Key: mount.Key, MountPath: mount.MountPath, Format: strings.TrimPrefix(filepath.Ext(mount.Key), "."), TemplateRevision: revision})
	}
	return files
}

func integrationFileError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "应用没有可用的默认上线模板")
	} else {
		apiShared.ValidationError(c, err.Error())
	}
}

func (h *ApplicationHandler) IntegrationListConfigFiles(c *gin.Context) {
	app, ok := h.integrationApplication(c, "application:read")
	if !ok {
		return
	}
	template, spec, err := h.integrationFileTemplate(app)
	if err != nil {
		integrationFileError(c, err)
		return
	}
	files := []integrationConfigFile{}
	for _, file := range integrationConfigFiles(spec, template.Revision) {
		if delegationClaims(c).Allows(file.Source + ":read") {
			files = append(files, file)
		}
	}
	c.Header("Cache-Control", "no-store")
	apiShared.Success(c, files)
}

func (h *ApplicationHandler) integrationFileRequest(c *gin.Context, action string) (*model.Application, bool) {
	source := c.Param("source")
	if source != "secret" && source != "configmap" {
		apiShared.BadRequest(c, "配置来源必须为 configmap 或 secret")
		return nil, false
	}
	c.Header("Cache-Control", "no-store")
	return h.integrationApplication(c, source+":"+action)
}

func findIntegrationConfigFile(spec applicationservice.ReleaseSpec, revision uint, source, key string) (integrationConfigFile, bool) {
	for _, file := range integrationConfigFiles(spec, revision) {
		if file.Source == source && file.Key == key {
			return file, true
		}
	}
	return integrationConfigFile{}, false
}

func (h *ApplicationHandler) IntegrationGetConfigFile(c *gin.Context) {
	app, ok := h.integrationFileRequest(c, "read")
	if !ok {
		return
	}
	template, spec, err := h.integrationFileTemplate(app)
	if err != nil {
		integrationFileError(c, err)
		return
	}
	file, ok := findIntegrationConfigFile(spec, template.Revision, c.Param("source"), c.Param("key"))
	if !ok {
		apiShared.NotFound(c, "配置未挂载、已禁用或未开放外部管理")
		return
	}
	content := spec.Config[file.Key]
	if file.Source == "secret" {
		values, err := h.decryptTemplateSecrets(template)
		if err != nil {
			apiShared.DBError(c, "读取模板 Secret 失败")
			return
		}
		var exists bool
		content, exists = values[file.Key]
		if !exists {
			apiShared.NotFound(c, "配置不存在")
			return
		}
	}
	if fileErr := auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: "application.config_file.read", ResourceType: "application", ResourceID: app.ID,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceDelegation, Outcome: model.AuditOutcomeSucceeded,
		Summary: "外部应用读取受管配置文件", TargetName: template.Name + "/" + file.Key, RequestID: apiShared.EnsureRequestID(c),
		Metadata: map[string]any{"template_id": template.ID, "file_key": file.Key, "source": file.Source, "template_revision": template.Revision},
	}); fileErr != nil {
		apiShared.DBError(c, "无法记录审计，未返回文件内容")
		return
	}
	apiShared.Success(c, struct {
		integrationConfigFile
		Content string `json:"content"`
	}{file, content})
}

func (h *ApplicationHandler) IntegrationReplaceConfigFile(c *gin.Context) {
	app, ok := h.integrationFileRequest(c, "write")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	var req struct {
		Content          *string `json:"content"`
		ExpectedRevision uint    `json:"expected_revision"`
		Restart          bool    `json:"restart"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Content == nil || req.ExpectedRevision == 0 {
		apiShared.BadRequest(c, "配置内容和预期模板版本必填")
		return
	}
	if len(*req.Content) > 4<<20 {
		apiShared.Error(c, http.StatusRequestEntityTooLarge, apiShared.CodeValidationFail, "配置不能超过 4 MiB")
		return
	}
	if req.Restart && !delegationClaims(c).Allows("application:restart") {
		integrationForbidden(c)
		return
	}
	managedFileMutationMu.Lock()
	defer managedFileMutationMu.Unlock()
	template, spec, err := h.integrationFileTemplate(app)
	if err != nil {
		integrationFileError(c, err)
		return
	}
	file, ok := findIntegrationConfigFile(spec, template.Revision, c.Param("source"), c.Param("key"))
	if !ok {
		apiShared.NotFound(c, "配置未挂载、已禁用或未开放外部管理")
		return
	}
	if template.Revision != req.ExpectedRevision {
		apiShared.Conflict(c, fmt.Sprintf("模板版本冲突，当前版本为 %d", template.Revision))
		return
	}
	if file.Source == "secret" {
		// The existing template editor interprets blank Secret values as preserve.
		if strings.TrimSpace(*req.Content) == "" {
			apiShared.ValidationError(c, "Secret 内容不能为空")
			return
		}
		spec.Secrets[file.Key] = *req.Content
	} else {
		spec.Config[file.Key] = *req.Content
	}
	updated, err := h.templateFromRequest(c.Request.Context(), app, &deploymentTemplateRequest{Name: template.Name, Description: template.Description, Enabled: template.Enabled, DefaultVersion: template.DefaultVersion, Spec: spec}, template)
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
			apiShared.DBError(c, "保存配置失败")
		}
		return
	}
	response := gin.H{"template_id": updated.ID, "template_revision": updated.Revision, "version": updated.Revision}
	if req.Restart {
		release, err := h.createRestartRelease(c.Request.Context(), app, apiShared.UserID(c))
		if err != nil {
			response["restart_pending"], response["restart_error"] = true, "配置已保存，重新发布创建失败"
		} else {
			response["release_id"] = release.ID
		}
	}
	apiShared.Success(c, response)
}
