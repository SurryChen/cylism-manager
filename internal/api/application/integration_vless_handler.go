package applicationapi

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

var vlessUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Only the users of one unambiguous sing-box REALITY inbound may be edited.
// All other configuration, including the private key, stays in the platform.
func (h *ApplicationHandler) vlessTemplateConfig(app *model.Application) (*model.ApplicationDeploymentTemplate, applicationservice.ReleaseSpec, map[string]any, map[string]any, error) {
	template, err := h.resources.GetDefaultApplicationDeploymentTemplate(app.ID)
	var spec applicationservice.ReleaseSpec
	if err != nil {
		return nil, spec, nil, nil, err
	}
	if !template.Enabled {
		return nil, spec, nil, nil, errors.New("应用没有可用的默认上线模板")
	}
	if err = json.Unmarshal([]byte(template.Spec), &spec); err != nil {
		return nil, spec, nil, nil, err
	}
	mounted := false
	for _, mount := range spec.FileMounts {
		if mount.SourceType == applicationservice.FileMountSourceApplicationSecret && mount.Key == "config.json" {
			mounted = true
		}
	}
	if !mounted || !applicationservice.IsSecretKeyManaged(spec, "config.json") {
		return nil, spec, nil, nil, errors.New("请挂载应用 Secret config.json 并启用允许外部修改")
	}
	for _, key := range spec.SecretsDisabled {
		if key == "config.json" {
			return nil, spec, nil, nil, errors.New("Secret config.json 已禁用")
		}
	}
	secrets, err := h.decryptTemplateSecrets(template)
	if err != nil {
		return nil, spec, nil, nil, errors.New("读取模板 Secret 失败")
	}
	root, inbound, err := parseVlessConfig(secrets["config.json"])
	return template, spec, root, inbound, err
}

func parseVlessConfig(content string) (map[string]any, map[string]any, error) {
	var root map[string]any
	if len(content) == 0 || len(content) > 4<<20 || json.Unmarshal([]byte(content), &root) != nil {
		return nil, nil, errors.New("Secret config.json 格式无效")
	}
	inbounds, _ := root["inbounds"].([]any)
	var selected map[string]any
	for _, raw := range inbounds {
		inbound, _ := raw.(map[string]any)
		if inbound["type"] != "vless" {
			continue
		}
		if selected != nil {
			return nil, nil, errors.New("存在多个 VLESS 入站，无法确定管理目标")
		}
		selected = inbound
	}
	if selected == nil {
		return nil, nil, errors.New("未找到 sing-box VLESS 入站")
	}
	tls, _ := selected["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	if tls["enabled"] != true || reality["enabled"] != true {
		return nil, nil, errors.New("VLESS 入站必须启用 REALITY")
	}
	users, ok := selected["users"].([]any)
	if !ok {
		return nil, nil, errors.New("VLESS users 配置无效")
	}
	names := map[string]bool{}
	ids := map[string]bool{}
	for _, raw := range users {
		user, ok := raw.(map[string]any)
		name, _ := user["name"].(string)
		uuid, _ := user["uuid"].(string)
		if !ok || !hysteriaUsernamePattern.MatchString(name) || !vlessUUIDPattern.MatchString(uuid) || names[name] || ids[strings.ToLower(uuid)] || user["flow"] != "xtls-rprx-vision" {
			return nil, nil, errors.New("VLESS 用户必须具有唯一名称、UUID 和 Vision flow")
		}
		names[name], ids[strings.ToLower(uuid)] = true, true
	}
	return root, selected, nil
}

func (h *ApplicationHandler) vlessApplication(c *gin.Context, action string) (*model.Application, bool) {
	app, ok := h.integrationApplication(c, action)
	if !ok {
		return nil, false
	}
	if delegationClaims(c).Capability != "vless" {
		integrationForbidden(c)
		return nil, false
	}
	c.Header("Cache-Control", "no-store")
	return app, true
}

func (h *ApplicationHandler) IntegrationVlessConfig(c *gin.Context) {
	app, ok := h.vlessApplication(c, "vless:read")
	if !ok {
		return
	}
	template, spec, _, inbound, err := h.vlessTemplateConfig(app)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	tls := inbound["tls"].(map[string]any)
	reality := tls["reality"].(map[string]any)
	private, _ := reality["private_key"].(string)
	raw, err := base64.RawURLEncoding.DecodeString(private)
	if err != nil {
		apiShared.ValidationError(c, "REALITY 密钥格式无效")
		return
	}
	key, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		apiShared.ValidationError(c, "REALITY 密钥格式无效")
		return
	}
	serverName, _ := tls["server_name"].(string)
	if serverName == "" {
		handshake, _ := reality["handshake"].(map[string]any)
		serverName, _ = handshake["server"].(string)
	}
	shortIDs, _ := reality["short_id"].([]any)
	if serverName == "" || len(shortIDs) == 0 {
		apiShared.ValidationError(c, "REALITY 缺少握手域名或 short_id")
		return
	}
	shortID, ok := shortIDs[0].(string)
	_, decodeErr := hex.DecodeString(shortID)
	if !ok || len(shortID) > 16 || decodeErr != nil {
		apiShared.ValidationError(c, "REALITY short_id 无效")
		return
	}
	names := []string{}
	for _, raw := range inbound["users"].([]any) {
		names = append(names, raw.(map[string]any)["name"].(string))
	}
	sort.Strings(names)
	apiShared.Success(c, gin.H{"users": names, "revision": template.Revision, "server": h.vlessPublicServer(c.Request.Context(), app, spec), "server_name": serverName, "server_port": inbound["listen_port"], "public_key": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), "short_id": shortID, "flow": "xtls-rprx-vision"})
}

func (h *ApplicationHandler) IntegrationVlessCredential(c *gin.Context) {
	app, ok := h.vlessApplication(c, "vless:read")
	if !ok {
		return
	}
	if !hysteriaUsernamePattern.MatchString(c.Param("username")) {
		apiShared.BadRequest(c, "用户名无效")
		return
	}
	_, _, _, inbound, err := h.vlessTemplateConfig(app)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	for _, raw := range inbound["users"].([]any) {
		user := raw.(map[string]any)
		if user["name"] == c.Param("username") {
			apiShared.Success(c, gin.H{"uuid": user["uuid"]})
			return
		}
	}
	apiShared.NotFound(c, "用户不存在")
}

func (h *ApplicationHandler) IntegrationUpdateVlessUser(c *gin.Context) {
	app, ok := h.vlessApplication(c, "vless:write")
	if !ok {
		return
	}
	var req struct {
		Operation        string `json:"operation"`
		UUID             string `json:"uuid"`
		ExpectedRevision uint   `json:"expected_revision"`
		Restart          bool   `json:"restart"`
	}
	name := c.Param("username")
	if !hysteriaUsernamePattern.MatchString(name) || c.ShouldBindJSON(&req) != nil || req.ExpectedRevision == 0 || (req.Operation != "add" && req.Operation != "replace" && req.Operation != "remove") || (req.Operation != "remove" && !vlessUUIDPattern.MatchString(req.UUID)) {
		apiShared.BadRequest(c, "VLESS 用户变更参数无效")
		return
	}
	if req.Restart && !delegationClaims(c).Allows("application:restart") {
		integrationForbidden(c)
		return
	}
	managedFileMutationMu.Lock()
	defer managedFileMutationMu.Unlock()
	template, spec, root, inbound, err := h.vlessTemplateConfig(app)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if template.Revision != req.ExpectedRevision {
		apiShared.Conflict(c, fmt.Sprintf("模板版本冲突，当前版本为 %d", template.Revision))
		return
	}
	users := inbound["users"].([]any)
	index := -1
	for i, raw := range users {
		user := raw.(map[string]any)
		if user["name"] == name {
			index = i
		}
		if req.Operation != "remove" && strings.EqualFold(user["uuid"].(string), req.UUID) && user["name"] != name {
			apiShared.Conflict(c, "UUID 已被使用")
			return
		}
	}
	if req.Operation == "add" && index >= 0 {
		apiShared.Conflict(c, "用户已存在")
		return
	}
	if req.Operation != "add" && index < 0 {
		apiShared.NotFound(c, "用户不存在")
		return
	}
	switch req.Operation {
	case "add":
		users = append(users, map[string]any{"name": name, "uuid": req.UUID, "flow": "xtls-rprx-vision"})
	case "replace":
		users[index].(map[string]any)["uuid"] = req.UUID
	case "remove":
		if len(users) <= 1 {
			apiShared.ValidationError(c, "不能删除最后一个用户")
			return
		}
		users = append(users[:index], users[index+1:]...)
	}
	inbound["users"] = users
	content, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		apiShared.DBError(c, "编码 VLESS 配置失败")
		return
	}
	current, err := h.decryptTemplateSecrets(template)
	if err != nil {
		apiShared.ValidationError(c, "读取模板 Secret 失败")
		return
	}
	if spec.Secrets == nil {
		spec.Secrets = map[string]string{}
	}
	for k := range current {
		if _, exists := spec.Secrets[k]; !exists {
			spec.Secrets[k] = ""
		}
	}
	spec.Secrets["config.json"] = string(content)
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
		release, err := h.createRestartRelease(c.Request.Context(), app, apiShared.UserID(c))
		if err != nil {
			response["restart_pending"] = true
			response["restart_error"] = "用户配置已保存，重新发布创建失败"
		} else {
			response["release_id"] = release.ID
		}
	}
	apiShared.Success(c, response)
}

// Host-network services may have no load-balancer address or Ingress. Only a
// public IP from the explicitly scheduled node is returned; private addresses
// never masquerade as a usable public endpoint.
func (h *ApplicationHandler) vlessPublicServer(ctx context.Context, app *model.Application, spec applicationservice.ReleaseSpec) string {
	if !spec.HostNetwork || spec.NodeName == "" || h.clusterClient == nil {
		return ""
	}
	client, err := h.clusterClient(ctx, app.Environment.ClusterID)
	if err != nil || client == nil {
		return ""
	}
	nodes, err := client.ListNodesContext(ctx)
	if err != nil {
		return ""
	}
	for _, node := range nodes {
		if node.Name != spec.NodeName {
			continue
		}
		for _, kind := range []corev1.NodeAddressType{corev1.NodeExternalIP, corev1.NodeInternalIP} {
			for _, address := range node.Status.Addresses {
				ip := net.ParseIP(address.Address)
				if address.Type == kind && ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
					return address.Address
				}
			}
		}
	}
	return ""
}
