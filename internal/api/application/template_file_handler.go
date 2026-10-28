package applicationapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/gin-gonic/gin"
)

type templateSecretFileEdit struct {
	Key     string                             `json:"key"`
	Changes []applicationservice.EnvFileChange `json:"changes"`
}

// InspectDeploymentTemplateFile deliberately lives outside the ordinary template
// DTO. Only authenticated platform managers can read it, and audit must succeed
// before any value leaves the server. Delegated integrations have no such route.
func (h *ApplicationHandler) InspectDeploymentTemplateFile(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if apiShared.UserID(c) == 0 {
		apiShared.Unauthorized(c, "请使用平台管理账号查看配置文件")
		return
	}
	if _, delegated := c.Get("delegation"); delegated {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "委托账号不能查看模板敏感文件"})
		return
	}
	applicationID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	templateID, err := apiShared.ParsePositiveID(c.Param("templateID"))
	if err != nil {
		apiShared.BadRequest(c, "模板 ID 无效")
		return
	}
	var request struct {
		Key      string `json:"key"`
		Revision uint   `json:"revision"`
		Mode     string `json:"mode"`
		Line     *int   `json:"line"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Key == "" || (request.Mode != "fields" && request.Mode != "field" && request.Mode != "content") || (request.Mode == "field" && (request.Line == nil || *request.Line < 0 || request.Name == "")) {
		apiShared.BadRequest(c, "文件读取参数无效")
		return
	}
	template, err := h.applications.GetApplicationDeploymentTemplate(applicationID, templateID)
	if err != nil {
		apiShared.NotFound(c, "上线模板不存在")
		return
	}
	if request.Revision == 0 || request.Revision != template.Revision {
		apiShared.Conflict(c, "模板版本已改变，请关闭编辑器后重新打开")
		return
	}
	var spec applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		apiShared.DBError(c, "读取模板失败")
		return
	}
	if !templateHasSensitiveFile(spec, request.Key) {
		apiShared.NotFound(c, "模板中不存在此敏感文件")
		return
	}
	values, err := h.decryptTemplateSecrets(template)
	if err != nil {
		apiShared.DBError(c, "解密配置文件失败")
		return
	}
	content, exists := values[request.Key]
	if !exists {
		apiShared.NotFound(c, "敏感文件内容不存在")
		return
	}
	metadata := map[string]any{"template_id": templateID, "file_key": request.Key, "mode": request.Mode}
	var fieldValue string
	if request.Mode == "field" {
		fieldValue, err = applicationservice.ReadEnvFileField(content, *request.Line, request.Name)
		if err != nil {
			apiShared.BadRequest(c, err.Error())
			return
		}
		metadata["field_name"] = request.Name
		metadata["field_line"] = *request.Line
	}
	if err := auditservice.NewService(h.audit).Record(auditservice.AuditEventInput{
		Action: "application.template_file.read", ResourceType: "application", ResourceID: applicationID,
		Actor: apiShared.ActorFromContext(c), Source: model.AuditSourceAPI, Outcome: model.AuditOutcomeSucceeded,
		Summary: "查看模板配置文件", TargetName: template.Name + "/" + request.Key, RequestID: apiShared.EnsureRequestID(c),
		Metadata: metadata,
	}); err != nil {
		apiShared.DBError(c, "无法记录审计，未返回文件内容")
		return
	}
	response := gin.H{"key": request.Key, "revision": template.Revision}
	if request.Mode == "content" {
		response["content"] = content
	} else if request.Mode == "field" {
		response["line"], response["name"], response["value"] = *request.Line, request.Name, fieldValue
	} else {
		fields, err := applicationservice.InspectEnvFile(content)
		response["supported"] = err == nil
		if err != nil {
			response["message"] = err.Error()
		} else {
			response["fields"] = fields
		}
	}
	apiShared.Success(c, response)
}

func templateHasSensitiveFile(spec applicationservice.ReleaseSpec, key string) bool {
	if _, exists := spec.Secrets[key]; !exists {
		return false
	}
	for _, file := range spec.FileMounts {
		if file.SourceType == "application_secret" && file.Key == key {
			return true
		}
	}
	return false
}

func (h *ApplicationHandler) applyTemplateSecretFileEdits(current *model.ApplicationDeploymentTemplate, req *deploymentTemplateRequest) error {
	if len(req.SecretFileEdits) == 0 {
		return nil
	}
	if req.Revision == 0 || req.Revision != current.Revision {
		return fmt.Errorf("逐项编辑文件需要当前模板版本，请重新读取")
	}
	var previous applicationservice.ReleaseSpec
	if err := json.Unmarshal([]byte(current.Spec), &previous); err != nil {
		return fmt.Errorf("读取旧模板失败")
	}
	values, err := h.decryptTemplateSecrets(current)
	if err != nil {
		return fmt.Errorf("解密旧模板文件失败")
	}
	seen := make(map[string]bool)
	for _, edit := range req.SecretFileEdits {
		if seen[edit.Key] || !templateHasSensitiveFile(previous, edit.Key) || !templateHasSensitiveFile(req.Spec, edit.Key) {
			return fmt.Errorf("逐项编辑只能修改当前模板保留的敏感文件")
		}
		if strings.TrimSpace(req.Spec.Secrets[edit.Key]) != "" {
			return fmt.Errorf("不能同时逐项编辑与替换同一文件")
		}
		seen[edit.Key] = true
		content, exists := values[edit.Key]
		if !exists {
			return fmt.Errorf("旧模板文件不存在")
		}
		updated, err := applicationservice.UpdateEnvFile(content, edit.Changes)
		if err != nil {
			return err
		}
		if strings.TrimSpace(updated) == "" {
			return fmt.Errorf("敏感文件不能清空，请保留内容或移除文件")
		}
		req.Spec.Secrets[edit.Key] = updated
	}
	return nil
}
