package productapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/githubapp"
	"github.com/gin-gonic/gin"
)

const (
	GitHubManifestCallbackPath   = "/api/settings/github/manifest/callback"
	GitHubInstallationReturnPath = "/api/settings/github/install/return"
	githubSettingsPath           = "/#/settings/system?tab=github"
	githubRegistrationCookie     = "__Host-cylism_github_registration"
	githubRegistrationTTL        = time.Hour
)

var githubSlug = regexp.MustCompile(`^[a-zA-Z0-9-]{1,100}$`)
var githubOrganization = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)

func githubPublicOrigin(raw string) (string, error) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
		(origin.EscapedPath() != "" && origin.EscapedPath() != "/") || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" {
		return "", errors.New("请先将 PUBLIC_URL 配置为 Cylism Manager 的 HTTPS 访问地址")
	}
	return origin.Scheme + "://" + origin.Host, nil
}

func githubRegistrationTokens() (string, string, error) {
	var random [64]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:32]), base64.RawURLEncoding.EncodeToString(random[32:]), nil
}

func githubRegistrationHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func setGitHubRegistrationCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: githubRegistrationCookie, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) StartGitHubRegistration(c *gin.Context) {
	if !h.githubAvailable(c) {
		return
	}
	origin, err := githubPublicOrigin(h.github.publicURL)
	if err != nil {
		shared.ValidationError(c, err.Error())
		return
	}
	if current := c.GetHeader("Origin"); current != "" && !strings.EqualFold(origin, current) {
		shared.ValidationError(c, "请使用平台配置的访问地址连接 GitHub")
		return
	}
	var input struct {
		Organization string `json:"organization"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if c.ShouldBindJSON(&input) != nil {
		shared.BadRequest(c, "GitHub 连接请求格式无效")
		return
	}
	organization := strings.TrimSpace(input.Organization)
	action := "https://github.com/settings/apps/new"
	if organization != "" {
		if !githubOrganization.MatchString(organization) {
			shared.ValidationError(c, "请输入有效的 GitHub 组织名")
			return
		}
		action = "https://github.com/organizations/" + organization + "/settings/apps/new"
	}
	state, browser, err := githubRegistrationTokens()
	if err != nil {
		shared.Error(c, http.StatusInternalServerError, shared.CodeInternalError, "无法启动 GitHub 连接")
		return
	}
	registration := model.GitHubRegistration{
		OwnerID: shared.UserID(c), StateHash: githubRegistrationHash(state), BrowserHash: githubRegistrationHash(browser),
		Organization: organization, ExpiresAt: time.Now().UTC().Add(githubRegistrationTTL),
	}
	if err := h.github.repository.StartGitHubRegistration(&registration); err != nil {
		if errors.Is(err, model.ErrGitHubAlreadyConnected) {
			shared.Error(c, http.StatusConflict, shared.CodeConflict, err.Error())
		} else {
			shared.DBError(c, "无法保存 GitHub 授权请求")
		}
		return
	}
	setGitHubRegistrationCookie(c, browser, int(githubRegistrationTTL.Seconds()))
	shared.Success(c, gin.H{
		"action": action + "?state=" + state,
		"manifest": gin.H{
			"name": "Cylism Manager", "url": origin, "redirect_url": origin + GitHubManifestCallbackPath,
			"setup_url": origin + GitHubInstallationReturnPath, "setup_on_update": true, "public": false,
			"hook_attributes":     gin.H{"url": origin, "active": false},
			"default_permissions": gin.H{"contents": "write", "actions": "write", "pull_requests": "write"},
		},
	})
}

func (h *Handler) GitHubManifestCallback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	state, code := c.Query("state"), c.Query("code")
	browser, _ := c.Cookie(githubRegistrationCookie)
	if h.github == nil || code == "" || len(state) != 43 || len(browser) != 43 {
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup=expired")
		return
	}
	registration, err := h.github.repository.ClaimGitHubRegistration(githubRegistrationHash(state), githubRegistrationHash(browser), time.Now().UTC())
	if err != nil {
		status := "failed"
		if errors.Is(err, model.ErrGitHubRegistrationExpired) {
			status = "expired"
		}
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup="+status)
		return
	}
	setGitHubRegistrationCookie(c, "", -1)
	app, err := h.github.client.ConvertManifest(c.Request.Context(), code)
	if err != nil {
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup=failed")
		return
	}
	if !githubRegistrationOwnerMatches(registration.Organization, app) || !githubSlug.MatchString(app.Slug) {
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup=owner")
		return
	}
	if _, err := githubapp.AppToken(app.ID, app.PrivateKey); err != nil {
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup=failed")
		return
	}
	ciphertext, err := security.Encrypt(h.github.key, app.PrivateKey)
	if err != nil {
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup=failed")
		return
	}
	connection := model.GitHubConnection{AppID: app.ID, Slug: app.Slug, EncryptedKey: ciphertext}
	if err := h.github.repository.FinishGitHubRegistration(registration, &connection); err != nil {
		c.Redirect(http.StatusSeeOther, githubSettingsPath+"&github_setup=failed")
		return
	}
	c.Redirect(http.StatusSeeOther, "https://github.com/apps/"+app.Slug+"/installations/new")
}

func githubRegistrationOwnerMatches(organization string, app githubapp.AppRegistration) bool {
	if organization == "" {
		return app.Owner.Type == "User"
	}
	return app.Owner.Type == "Organization" && strings.EqualFold(app.Owner.Login, organization)
}

func GitHubInstallationReturn(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	// The settings page reads installations directly from GitHub. The browser
	// return parameters do not authorize access to an installation.
	c.Redirect(http.StatusSeeOther, githubSettingsPath)
}
