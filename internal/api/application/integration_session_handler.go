package applicationapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
)

type delegationRequest struct {
	EnvironmentIDs []uint   `json:"environment_ids"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
}

const (
	integrationHandoffTTL = 60 * time.Second
	integrationSessionTTL = 8 * time.Hour
)

func randomOpaqueValue(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func opaqueHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func (h *ApplicationHandler) CreateIntegrationHandoff(c *gin.Context) { h.createIntegrationHandoff(c) }

type integrationHandoffRequest struct {
	EndpointID  uint   `json:"endpoint_id"`
	RedirectURL string `json:"redirect_url"`
}

func (h *ApplicationHandler) createIntegrationHandoff(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	var req integrationHandoffRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.EndpointID == 0 {
		apiShared.BadRequest(c, "应用入口必填")
		return
	}
	endpoint, err := h.resources.GetApplicationEndpoint(app.ID, req.EndpointID)
	if err != nil {
		apiShared.NotFound(c, "应用入口不存在")
		return
	}
	if endpoint.AccessMode != model.ApplicationEndpointAccessProtectedConsole {
		apiShared.ValidationError(c, "该入口不是受保护控制台")
		return
	}
	if endpoint.Domain == "" {
		apiShared.ValidationError(c, "应用入口尚未绑定域名")
		return
	}
	scheme := "http"
	if endpoint.TLSEnabled {
		scheme = "https"
	}
	endpointURL := fmt.Sprintf("%s://%s%s", scheme, endpoint.Domain, endpoint.Path)
	if strings.TrimSpace(req.RedirectURL) != "" {
		localURL, err := parseLoopbackRedirectURL(req.RedirectURL)
		if err != nil {
			apiShared.ValidationError(c, "本地联调地址无效，仅支持 http(s)://localhost、127.0.0.1 或 ::1")
			return
		}
		endpointURL = localURL.String()
	}
	actions := []string{"application:read", "configmap:read", "configmap:write", "application:restart"}
	now := time.Now()
	code, err := randomOpaqueValue(32)
	if err != nil {
		apiShared.DBError(c, "创建管理会话失败")
		return
	}
	session := &model.IntegrationSession{HandoffCodeHash: opaqueHash(code), UserID: apiShared.UserID(c), ProjectID: app.ProjectID, ApplicationID: app.ID, EnvironmentID: app.EnvironmentID, ActionsData: strings.Join(actions, ","), HandoffExpiresAt: now.Add(integrationHandoffTTL), ExpiresAt: now.Add(integrationSessionTTL)}
	if err := h.sessions.CreateIntegrationSession(session); err != nil {
		apiShared.DBError(c, "创建管理会话失败")
		return
	}
	handoffURL, err := appendHandoffCode(endpointURL, code)
	if err != nil {
		apiShared.DBError(c, "生成跳转地址失败")
		return
	}
	model.Success(c, gin.H{"handoff_code": code, "handoff_url": handoffURL, "expires_in": int(integrationHandoffTTL.Seconds())})
}

func appendHandoffCode(raw, code string) (string, error) {
	u, err := parseExternalLink(raw)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("handoff_code", code)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func parseExternalLink(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("invalid external link")
	}
	return u, nil
}

func parseLoopbackRedirectURL(raw string) (*url.URL, error) {
	u, err := parseExternalLink(raw)
	if err != nil || u.User != nil {
		return nil, errors.New("invalid local redirect URL")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" {
		return u, nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return u, nil
	}
	return nil, errors.New("redirect URL must use a loopback host")
}

func bearerValue(c *gin.Context) string {
	parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
	if len(parts) == 2 && parts[0] == "Bearer" {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func (h *ApplicationHandler) ExchangeIntegrationSession(c *gin.Context) {
	code := bearerValue(c)
	if code == "" {
		apiShared.Unauthorized(c, "未提供跳转码")
		return
	}
	now := time.Now()
	token, err := randomOpaqueValue(48)
	if err != nil {
		apiShared.DBError(c, "交换管理会话失败")
		return
	}
	session, err := h.sessions.ExchangeIntegrationSession(opaqueHash(code), opaqueHash(token), now.Add(integrationSessionTTL), now)
	if err != nil {
		apiShared.Unauthorized(c, "跳转码无效或已过期")
		return
	}
	model.Success(c, gin.H{"session_token": token, "expires_at": session.ExpiresAt})
}

func (h *ApplicationHandler) CreateIntegrationDelegation(c *gin.Context) {
	h.createIntegrationDelegation(c)
}

type integrationDelegationRequest struct {
	Capability string `json:"capability"`
}

func (h *ApplicationHandler) createIntegrationDelegation(c *gin.Context) {
	token := bearerValue(c)
	if token == "" {
		apiShared.Unauthorized(c, "未提供管理会话")
		return
	}
	session, err := h.sessions.GetActiveIntegrationSession(opaqueHash(token), time.Now())
	if err != nil {
		apiShared.Unauthorized(c, "管理会话无效或已过期")
		return
	}
	actions := strings.Split(session.ActionsData, ",")
	var req integrationDelegationRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Capability) == "" {
		apiShared.ValidationError(c, "capability 必填")
		return
	}
	capability, err := model.NormalizeApplicationCapabilities([]string{req.Capability})
	if err != nil {
		apiShared.ValidationError(c, "capability 格式无效")
		return
	}
	delegation, err := auth.GenerateDelegationToken(h.delegationSecret, auth.DelegationClaims{UserID: session.UserID, ProjectID: session.ProjectID, EnvironmentIDs: []uint{session.EnvironmentID}, Capability: capability[0], Actions: actions}, auth.MaxDelegationTTL)
	if err != nil {
		apiShared.DBError(c, "签发委托失败")
		return
	}
	model.Success(c, gin.H{"token": delegation, "expires_in": int(auth.MaxDelegationTTL.Seconds())})
}
